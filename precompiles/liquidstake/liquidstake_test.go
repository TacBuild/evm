package liquidstake_test

import (
	"time"

	sdkmath "cosmossdk.io/math"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	"github.com/cosmos/evm/precompiles/authorization"
	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	"github.com/cosmos/evm/x/liquidstake/types"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"

	"math/big"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	testkeyring "github.com/cosmos/evm/testutil/integration/os/keyring"

	"testing"

	chainutil "github.com/cosmos/evm/evmd/testutil"
	"github.com/cosmos/evm/testutil/integration/os/factory"
	"github.com/cosmos/evm/testutil/integration/os/grpc"
	"github.com/cosmos/evm/testutil/integration/os/network"
	"github.com/cosmos/evm/x/vm/statedb"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/suite"
)

func CheckAuthorizationWithContext(ctx sdk.Context, ak authzkeeper.Keeper, authorizationType liquidstaketypes.AuthorizationType, grantee, granter common.Address) (*liquidstaketypes.LiquidStakeAuthorization, *time.Time) {
	liquidAuthz := liquidstaketypes.LiquidStakeAuthorization{AuthorizationType: authorizationType}
	auth, expirationTime := ak.GetAuthorization(ctx, grantee.Bytes(), granter.Bytes(), liquidAuthz.MsgTypeURL())

	authorization, ok := auth.(*liquidstaketypes.LiquidStakeAuthorization)
	if !ok {
		return nil, expirationTime
	}

	return authorization, expirationTime
}

type LiquidStakePrecompileTestSuite struct {
	suite.Suite

	nw          *network.UnitTestNetwork
	factory     factory.TxFactory
	grpcHandler grpc.Handler
	keyring     testkeyring.Keyring

	bondDenom  string
	precompile *liquidstake.Precompile

	liquidValidatorAddr common.Address
	liquidValidator     stakingtypes.Validator

	ValidatorAddr common.Address
	Validator     stakingtypes.Validator

	admin testkeyring.Key
}

func TestLiquidStakePrecompileTestSuite(t *testing.T) {
	suite.Run(t, new(LiquidStakePrecompileTestSuite))
}

func (s *LiquidStakePrecompileTestSuite) SetupTest() {
	keyring := testkeyring.New(10)
	nw := network.NewUnitTestNetwork(
		network.WithPreFundedAccounts(keyring.GetAllAccAddrs()...),
	)
	grpcHandler := grpc.NewIntegrationHandler(nw)
	txFactory := factory.New(nw, grpcHandler)

	ctx := nw.GetContext()
	sk := nw.App.StakingKeeper
	bondDenom, err := sk.BondDenom(ctx)
	if err != nil {
		panic(err)
	}

	s.bondDenom = bondDenom
	s.factory = txFactory
	s.grpcHandler = grpcHandler
	s.keyring = keyring
	s.nw = nw

	validators, err := s.nw.App.StakingKeeper.GetValidators(ctx, 2)
	s.liquidValidator = validators[0]
	if err != nil {
		panic(err)
	}

	params := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
	params.ModulePaused = false
	params.LsmDisabled = false
	params.LiquidBondDenom = "agatom"

	valAddr, err := sdk.ValAddressFromBech32(validators[0].OperatorAddress)
	if err != nil {
		panic(err)
	}
	s.liquidValidatorAddr = common.BytesToAddress(valAddr.Bytes())

	s.nw.App.LiquidStakeKeeper.SetLiquidValidator(ctx, liquidstaketypes.LiquidValidator{
		OperatorAddress: validators[0].OperatorAddress,
	})

	params.WhitelistedValidators = append(params.WhitelistedValidators, liquidstaketypes.WhitelistedValidator{
		ValidatorAddress: validators[0].OperatorAddress,
		TargetWeight:     sdkmath.NewInt(10000),
	})

	params.WhitelistAdminAddress = validators[0].OperatorAddress

	// extract validator that wouldnt be liquidValidator
	s.Validator = validators[1]
	valAddr, err = sdk.ValAddressFromBech32(s.Validator.OperatorAddress)
	if err != nil {
		panic(err)
	}
	s.ValidatorAddr = common.BytesToAddress(valAddr.Bytes())

	s.admin = keyring.GetKey(9)
	params.WhitelistAdminAddress = s.admin.AccAddr.String()

	err = s.nw.App.LiquidStakeKeeper.SetParams(ctx, params)
	if err != nil {
		panic(err)
	}

	if s.precompile, err = liquidstake.NewPrecompile(
		s.nw.App.LiquidStakeKeeper,
		s.nw.App.AuthzKeeper,
	); err != nil {
		panic(err)
	}
}

func (s *LiquidStakePrecompileTestSuite) TestIsTransaction() {
	testCases := []struct {
		name   string
		method abi.Method
		isTx   bool
	}{
		{
			authorization.ApproveMethod,
			s.precompile.Methods[authorization.ApproveMethod],
			true,
		},
		{
			authorization.IncreaseAllowanceMethod,
			s.precompile.Methods[authorization.IncreaseAllowanceMethod],
			true,
		},
		{
			authorization.DecreaseAllowanceMethod,
			s.precompile.Methods[authorization.DecreaseAllowanceMethod],
			true,
		},
		{
			liquidstake.LiquidStakeMethod,
			s.precompile.Methods[liquidstake.LiquidStakeMethod],
			true,
		},
		{
			liquidstake.StakeToLPMethod,
			s.precompile.Methods[liquidstake.StakeToLPMethod],
			true,
		},
		{
			liquidstake.LiquidUnstakeMethod,
			s.precompile.Methods[liquidstake.LiquidUnstakeMethod],
			true,
		},
		{
			authorization.AllowanceMethod,
			s.precompile.Methods[authorization.AllowanceMethod],
			false,
		},
		{
			liquidstake.UpdateParams,
			s.precompile.Methods[liquidstake.StakeToLPMethod],
			true,
		},
		{
			liquidstake.UpdateWhitelistedValidators,
			s.precompile.Methods[liquidstake.LiquidUnstakeMethod],
			true,
		},
		{
			liquidstake.SetModulePaused,
			s.precompile.Methods[authorization.AllowanceMethod],
			false,
		},
		{
			"invalid",
			abi.Method{},
			false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.Require().Equal(s.precompile.IsTransaction(&tc.method), tc.isTx)
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestRequiredGas() {
	testcases := []struct {
		name     string
		malleate func() []byte
		expGas   uint64
	}{
		{
			"success - liquidStake transaction with correct gas estimation",
			func() []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidStakeMethod,
					s.keyring.GetAddr(0),
					big.NewInt(1000000),
				)
				s.Require().NoError(err)
				return input
			},
			3920,
		},
		{
			"success - stakeToLP transaction with correct gas estimation",
			func() []byte {
				input, err := s.precompile.Pack(
					liquidstake.StakeToLPMethod,
					s.keyring.GetAddr(0),
					s.keyring.GetAddr(1),
					big.NewInt(1000000),
					big.NewInt(1000000),
				)
				s.Require().NoError(err)
				return input
			},
			5840,
		},
		{
			"success - liquidUnstake transaction with correct gas estimation",
			func() []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidUnstakeMethod,
					s.keyring.GetAddr(0),
					big.NewInt(1000000),
				)
				s.Require().NoError(err)
				return input
			},
			3920,
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			s.SetupTest()

			// malleate contract input
			input := tc.malleate()
			gas := s.precompile.RequiredGas(input)

			s.Require().Equal(tc.expGas, gas)
		})
	}
}

// TestRun tests the precompile's Run method.
func (s *LiquidStakePrecompileTestSuite) TestRun() {
	var ctx sdk.Context
	testcases := []struct {
		name        string
		malleate    func(delegator, grantee testkeyring.Key) []byte
		gas         uint64
		readOnly    bool
		expPass     bool
		errContains string
	}{
		{
			"fail - contract gas limit is < gas cost to run a query / tx",
			func(delegator, grantee testkeyring.Key) []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidStakeMethod,
					delegator.Addr,
					big.NewInt(1000000),
				)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			8000,
			false,
			false,
			"out of gas",
		},
		{
			"pass - liquidStake transaction",
			func(delegator, grantee testkeyring.Key) []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidStakeMethod,
					delegator.Addr,
					big.NewInt(1000000),
				)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			1000000,
			false,
			true,
			"",
		},
		//		{
		//			"pass - stakeToLP transaction",
		//			func(delegator, grantee testkeyring.Key) []byte {
		//				delAmount := math.NewInt(1000000000000000000)
		//				_, err := s.nw.App.StakingKeeper.Delegate(ctx, sdk.AccAddress(delegator.Addr.Bytes()), delAmount, stakingtypes.Bonded, s.validator, false)
		//				if err != nil {
		//					panic(err)
		//				}
		//
		//				_, err = s.nw.App.StakingKeeper.GetDelegation(ctx, delegator.AccAddr, sdk.ValAddress(s.validatorAdr.Bytes()))
		//				if err != nil {
		//					panic(err)
		//				}
		//
		//				// Use a smaller amount that definitely exists in the delegation
		//				tokenizeAmount := big.NewInt(1000000000000000000)
		//				input, err := s.precompile.Pack(
		//					liquidstake.StakeToLPMethod,
		//					delegator.Addr,
		//					s.validatorAdr,
		//					tokenizeAmount,
		//					tokenizeAmount,
		//				)
		//				s.Require().NoError(err, "failed to pack input")
		//				return input
		//			},
		//			1000000000000000000,
		//			false,
		//			true,
		//			"",
		//		},
		{
			"pass - liquidUnstake transaction",
			func(delegator, grantee testkeyring.Key) []byte {
				_, err := s.nw.App.LiquidStakeKeeper.LiquidStake(ctx, liquidstaketypes.LiquidStakeProxyAcc, delegator.AccAddr, sdk.NewCoin(s.bondDenom, sdkmath.NewInt(1000000000000000000)))
				s.Require().NoError(err)

				input, err := s.precompile.Pack(
					liquidstake.LiquidUnstakeMethod,
					delegator.Addr,
					big.NewInt(1000000000000000000),
				)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			1000000,
			false,
			true,
			"",
		},
		{
			"fail - invalid method",
			func(_, _ testkeyring.Key) []byte {
				return []byte("invalid")
			},
			1, // use gas > 0 to avoid doing gas estimation
			false,
			false,
			"no method with id",
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			// setup basic test suite
			s.SetupTest()
			ctx = s.nw.GetContext().WithBlockTime(time.Now())

			baseFee := s.nw.App.EVMKeeper.GetBaseFee(ctx)

			delegator := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			contract := vm.NewPrecompile(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, tc.gas)
			contractAddr := contract.Address()

			// malleate testcase
			contract.Input = tc.malleate(delegator, grantee)

			// Build and sign Ethereum transaction
			txArgs := evmtypes.EvmTxArgs{
				ChainID:   evmtypes.GetEthChainConfig().ChainID,
				Nonce:     0,
				To:        &contractAddr,
				Amount:    nil,
				GasLimit:  tc.gas,
				GasPrice:  chainutil.ExampleMinGasPrices.BigInt(),
				GasFeeCap: baseFee,
				GasTipCap: big.NewInt(1),
				Accesses:  &ethtypes.AccessList{},
			}

			msg, err := s.factory.GenerateGethCoreMsg(delegator.Priv, txArgs)
			s.Require().NoError(err)

			// Instantiate config
			proposerAddress := ctx.BlockHeader().ProposerAddress
			cfg, err := s.nw.App.EVMKeeper.EVMConfig(ctx, proposerAddress)
			s.Require().NoError(err, "failed to instantiate EVM config")

			// Instantiate EVM
			headerHash := ctx.HeaderHash()
			stDB := statedb.New(
				ctx,
				s.nw.App.EVMKeeper,
				statedb.NewEmptyTxConfig(common.BytesToHash(headerHash)),
			)
			evm := s.nw.App.EVMKeeper.NewEVM(
				ctx, *msg, cfg, nil, stDB,
			)

			precompiles, found, err := s.nw.App.EVMKeeper.GetPrecompileInstance(ctx, contractAddr)
			s.Require().NoError(err, "failed to instantiate precompile")
			s.Require().True(found, "not found precompile")
			evm.WithPrecompiles(precompiles.Map, precompiles.Addresses)

			// Run precompiled contract
			bz, err := s.precompile.Run(evm, contract, tc.readOnly)

			// Check results
			if tc.expPass {
				s.Require().NoError(err, "expected no error when running the precompile")
				s.Require().NotNil(bz, "expected returned bytes not to be nil")
			} else {
				s.Require().Error(err, "expected error to be returned when running the precompile")
				s.Require().Nil(bz, "expected returned bytes to be nil")
				s.Require().ErrorContains(err, tc.errContains)
				consumed := ctx.GasMeter().GasConsumed()
				// LessThanOrEqual because the gas is consumed before the error is returned
				s.Require().LessOrEqual(tc.gas, consumed, "expected gas consumed to be equal or less to gas limit")
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestPrecompileInitialization() {
	// Test that precompile is properly initialized
	s.Require().NotNil(s.precompile)
	s.Require().NotNil(s.nw.App.LiquidStakeKeeper)
	s.Require().NotNil(s.nw.App.AuthzKeeper)
}

func (s *LiquidStakePrecompileTestSuite) TestPrecompileMethodSignatures() {
	// Test that all expected methods are registered
	methods := s.precompile.Methods

	s.Require().Contains(methods, liquidstake.LiquidStakeMethod)
	s.Require().Contains(methods, liquidstake.StakeToLPMethod)
	s.Require().Contains(methods, liquidstake.LiquidUnstakeMethod)
	s.Require().Contains(methods, authorization.ApproveMethod)
	s.Require().Contains(methods, authorization.AllowanceMethod)
	// Test query methods
	s.Require().Contains(methods, liquidstake.ParamsMethod)
	s.Require().Contains(methods, liquidstake.LiquidValidatorsMethod)
	s.Require().Contains(methods, liquidstake.StatesMethod)
}

// TestQueryMethods tests the precompile's query methods.
func (s *LiquidStakePrecompileTestSuite) TestQueryMethods() {
	var ctx sdk.Context
	testcases := []struct {
		name        string
		malleate    func() []byte
		gas         uint64
		readOnly    bool
		expPass     bool
		errContains string
	}{
		{
			"pass - params query",
			func() []byte {
				input, err := s.precompile.Pack(liquidstake.ParamsMethod)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			100000,
			true,
			true,
			"",
		},
		{
			"pass - liquidValidators query",
			func() []byte {
				input, err := s.precompile.Pack(liquidstake.LiquidValidatorsMethod)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			100000,
			true,
			true,
			"",
		},
		{
			"pass - states query",
			func() []byte {
				input, err := s.precompile.Pack(liquidstake.StatesMethod)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			100000,
			true,
			true,
			"",
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			// setup basic test suite
			s.SetupTest()
			ctx = s.nw.GetContext().WithBlockTime(time.Now())

			baseFee := s.nw.App.EVMKeeper.GetBaseFee(ctx)

			delegator := s.keyring.GetKey(0)

			contract := vm.NewPrecompile(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, tc.gas)
			contractAddr := contract.Address()

			// malleate testcase
			contract.Input = tc.malleate()

			// Build and sign Ethereum transaction
			txArgs := evmtypes.EvmTxArgs{
				ChainID:   evmtypes.GetEthChainConfig().ChainID,
				Nonce:     0,
				To:        &contractAddr,
				Amount:    nil,
				GasLimit:  tc.gas,
				GasPrice:  chainutil.ExampleMinGasPrices.BigInt(),
				GasFeeCap: baseFee,
				GasTipCap: big.NewInt(1),
				Accesses:  &ethtypes.AccessList{},
			}

			msg, err := s.factory.GenerateGethCoreMsg(delegator.Priv, txArgs)
			s.Require().NoError(err)

			// Instantiate config
			proposerAddress := ctx.BlockHeader().ProposerAddress
			cfg, err := s.nw.App.EVMKeeper.EVMConfig(ctx, proposerAddress)
			s.Require().NoError(err, "failed to instantiate EVM config")

			// Instantiate EVM
			headerHash := ctx.HeaderHash()
			stDB := statedb.New(
				ctx,
				s.nw.App.EVMKeeper,
				statedb.NewEmptyTxConfig(common.BytesToHash(headerHash)),
			)
			evm := s.nw.App.EVMKeeper.NewEVM(
				ctx, *msg, cfg, nil, stDB,
			)

			precompiles, found, err := s.nw.App.EVMKeeper.GetPrecompileInstance(ctx, contractAddr)
			s.Require().NoError(err, "failed to instantiate precompile")
			s.Require().True(found, "not found precompile")
			evm.WithPrecompiles(precompiles.Map, precompiles.Addresses)

			// Run precompiled contract
			bz, err := s.precompile.Run(evm, contract, tc.readOnly)

			// Check results
			if tc.expPass {
				s.Require().NoError(err, "expected no error when running the precompile")
				s.Require().NotNil(bz, "expected returned bytes not to be nil")
				s.Require().Greater(len(bz), 0, "expected non-empty response")
			} else {
				s.Require().Error(err, "expected error to be returned when running the precompile")
				s.Require().Nil(bz, "expected returned bytes to be nil")
				s.Require().ErrorContains(err, tc.errContains)
			}
		})
	}
}

// TestRun tests the precompile's Run method.
func (s *LiquidStakePrecompileTestSuite) TestAdminMethods() {
	var ctx sdk.Context
	testcases := []struct {
		name        string
		malleate    func() ([]byte, testkeyring.Key)
		gas         uint64
		expPass     bool
		errContains string
	}{
		{
			"UpdateParams_Basic_Positive",
			func() ([]byte, testkeyring.Key) {
				params := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
				updatableParams := types.UpdatableParams{
					UnstakeFeeRate:        params.UnstakeFeeRate,
					LsmDisabled:           true,
					MinLiquidStakeAmount:  params.MinLiquidStakeAmount,
					CwLockedPoolAddress:   params.CwLockedPoolAddress,
					FeeAccountAddress:     params.FeeAccountAddress,
					AutocompoundFeeRate:   params.AutocompoundFeeRate,
					WhitelistAdminAddress: params.WhitelistAdminAddress,
				}

				paramsAfter := liquidstake.NewLiquidStakeUpdatableParamsOutput(&updatableParams)

				input, err := s.precompile.Pack(
					liquidstake.UpdateParams,
					paramsAfter,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			true,
			"",
		},
		{
			"UpdateWhitelistedValidators_Basic_Positive",
			func() ([]byte, testkeyring.Key) {
				paramsBeforeInternal := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
				whitelisted := liquidstake.NewLiquidStakeWhitelistedValidatorsOutput(&paramsBeforeInternal)

				whitelisted[0].TargetWeight = big.NewInt(8000)

				whitelisted = append(whitelisted, liquidstake.WhitelistedValidator{
					ValidatorAddress: s.ValidatorAddr,
					TargetWeight:     big.NewInt(2000),
				})

				input, err := s.precompile.Pack(
					liquidstake.UpdateWhitelistedValidators,
					whitelisted,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			true,
			"",
		},
		{
			"SetModulePaused_Basic_Positive",
			func() ([]byte, testkeyring.Key) {
				input, err := s.precompile.Pack(
					liquidstake.SetModulePaused,
					false,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			true,
			"",
		},
		// Negative test cases
		{
			"UpdateParams_Unauthorized_Caller",
			func() ([]byte, testkeyring.Key) {
				params := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
				updatableParams := types.UpdatableParams{
					UnstakeFeeRate:        params.UnstakeFeeRate,
					LsmDisabled:           true,
					MinLiquidStakeAmount:  params.MinLiquidStakeAmount,
					CwLockedPoolAddress:   params.CwLockedPoolAddress,
					FeeAccountAddress:     params.FeeAccountAddress,
					AutocompoundFeeRate:   params.AutocompoundFeeRate,
					WhitelistAdminAddress: params.WhitelistAdminAddress,
				}

				paramsAfter := liquidstake.NewLiquidStakeUpdatableParamsOutput(&updatableParams)

				// Use non-admin key
				nonAdmin := s.keyring.GetKey(1)

				input, err := s.precompile.Pack(
					liquidstake.UpdateParams,
					paramsAfter,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, nonAdmin
			},
			800000,
			false,
			"unauthorized",
		},
		{
			"UpdateWhitelistedValidators_Unauthorized_Caller",
			func() ([]byte, testkeyring.Key) {
				paramsBeforeInternal := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
				whitelisted := liquidstake.NewLiquidStakeWhitelistedValidatorsOutput(&paramsBeforeInternal)

				// Use non-admin key
				nonAdmin := s.keyring.GetKey(2)

				input, err := s.precompile.Pack(
					liquidstake.UpdateWhitelistedValidators,
					whitelisted,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, nonAdmin
			},
			800000,
			false,
			"unauthorized",
		},
		{
			"SetModulePaused_Unauthorized_Caller",
			func() ([]byte, testkeyring.Key) {
				// Use non-admin key
				nonAdmin := s.keyring.GetKey(3)

				input, err := s.precompile.Pack(
					liquidstake.SetModulePaused,
					true,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, nonAdmin
			},
			800000,
			false,
			"unauthorized",
		},
		{
			"UpdateWhitelistedValidators_Empty_Validators_List",
			func() ([]byte, testkeyring.Key) {
				// Create empty validators list
				emptyValidators := []liquidstake.WhitelistedValidator{}

				input, err := s.precompile.Pack(
					liquidstake.UpdateWhitelistedValidators,
					emptyValidators,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			false,
			"whitelisted validators list cannot be empty", // This might need adjustment based on actual error message
		},
		{
			"UpdateParams_Out_Of_Gas",
			func() ([]byte, testkeyring.Key) {
				params := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
				updatableParams := types.UpdatableParams{
					UnstakeFeeRate:        params.UnstakeFeeRate,
					LsmDisabled:           true,
					MinLiquidStakeAmount:  params.MinLiquidStakeAmount,
					CwLockedPoolAddress:   params.CwLockedPoolAddress,
					FeeAccountAddress:     params.FeeAccountAddress,
					AutocompoundFeeRate:   params.AutocompoundFeeRate,
					WhitelistAdminAddress: params.WhitelistAdminAddress,
				}

				paramsAfter := liquidstake.NewLiquidStakeUpdatableParamsOutput(&updatableParams)

				input, err := s.precompile.Pack(
					liquidstake.UpdateParams,
					paramsAfter,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			10000, // Insufficient gas
			false,
			"out of gas",
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			// setup basic test suite
			s.SetupTest()
			ctx = s.nw.GetContext().WithBlockTime(time.Now())

			baseFee := s.nw.App.EVMKeeper.GetBaseFee(ctx)

			input, sender := tc.malleate()

			contract := vm.NewPrecompile(vm.AccountRef(sender.Addr), s.precompile, common.U2560, tc.gas)
			contractAddr := contract.Address()

			contract.Input = input

			// Build and sign Ethereum transaction
			txArgs := evmtypes.EvmTxArgs{
				ChainID:   evmtypes.GetEthChainConfig().ChainID,
				Nonce:     0,
				To:        &contractAddr,
				Amount:    nil,
				GasLimit:  tc.gas,
				GasPrice:  chainutil.ExampleMinGasPrices.BigInt(),
				GasFeeCap: baseFee,
				GasTipCap: big.NewInt(1),
				Accesses:  &ethtypes.AccessList{},
			}

			msg, err := s.factory.GenerateGethCoreMsg(sender.Priv, txArgs)
			s.Require().NoError(err)

			// Instantiate config
			proposerAddress := ctx.BlockHeader().ProposerAddress
			cfg, err := s.nw.App.EVMKeeper.EVMConfig(ctx, proposerAddress)
			s.Require().NoError(err, "failed to instantiate EVM config")

			// Instantiate EVM
			headerHash := ctx.HeaderHash()
			stDB := statedb.New(
				ctx,
				s.nw.App.EVMKeeper,
				statedb.NewEmptyTxConfig(common.BytesToHash(headerHash)),
			)
			evm := s.nw.App.EVMKeeper.NewEVM(
				ctx, *msg, cfg, nil, stDB,
			)

			precompiles, found, err := s.nw.App.EVMKeeper.GetPrecompileInstance(ctx, contractAddr)
			s.Require().NoError(err, "failed to instantiate precompile")
			s.Require().True(found, "not found precompile")
			evm.WithPrecompiles(precompiles.Map, precompiles.Addresses)

			// Run precompiled contract
			bz, err := s.precompile.Run(evm, contract, false)

			// Check results
			if tc.expPass {
				s.Require().NoError(err, "expected no error when running the precompile")
				s.Require().NotNil(bz, "expected returned bytes not to be nil")
			} else {
				s.Require().Error(err, "expected error to be returned when running the precompile")
				s.Require().Nil(bz, "expected returned bytes to be nil")
				s.Require().ErrorContains(err, tc.errContains)
			}
		})
	}
}
