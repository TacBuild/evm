package liquidstake_test

import (
	"time"

	"cosmossdk.io/math"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"
	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
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

type LiquidStakePrecompileTestSuite struct {
	suite.Suite

	nw          *network.UnitTestNetwork
	factory     factory.TxFactory
	grpcHandler grpc.Handler
	keyring     testkeyring.Keyring

	bondDenom  string
	precompile *liquidstake.Precompile

	validatorAdr common.Address
	validator    stakingtypes.Validator
}

func TestLiquidStakePrecompileTestSuite(t *testing.T) {
	suite.Run(t, new(LiquidStakePrecompileTestSuite))
}

func (s *LiquidStakePrecompileTestSuite) SetupTest() {
	keyring := testkeyring.New(2)
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

	validators, err := s.nw.App.StakingKeeper.GetValidators(ctx, 1)
	s.validator = validators[0]
	if err != nil {
		panic(err)
	}

	params := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
	params.ModulePaused = false
	params.LsmDisabled = false // Enable LSM for testing
	params.LiquidBondDenom = "agatom"

	// Get operator address from validator and convert to common.Address
	valAddr, err := sdk.ValAddressFromBech32(validators[0].OperatorAddress)
	if err != nil {
		panic(err)
	}
	s.validatorAdr = common.BytesToAddress(valAddr.Bytes())

	s.nw.App.LiquidStakeKeeper.SetLiquidValidator(ctx, liquidstaketypes.LiquidValidator{
		OperatorAddress: validators[0].OperatorAddress,
	})

	params.WhitelistedValidators = append(params.WhitelistedValidators, liquidstaketypes.WhitelistedValidator{
		ValidatorAddress: validators[0].OperatorAddress,
		TargetWeight:     sdkmath.NewInt(10000),
	})

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
func (s *LiquidStakePrecompileTestSuite) CreateAuthorization(ctx sdk.Context, granter, grantee sdk.AccAddress, authzType stakingtypes.AuthorizationType, coin *sdk.Coin) error {
	// Get all available validators and filter out jailed validators
	validators := make([]sdk.ValAddress, 0)
	err := s.nw.App.StakingKeeper.IterateValidators(
		ctx, func(_ int64, validator stakingtypes.ValidatorI) (stop bool) {
			if validator.IsJailed() {
				return
			}
			validators = append(validators, sdk.ValAddress(validator.GetOperator()))
			return
		},
	)
	if err != nil {
		return err
	}

	stakingAuthz, err := stakingtypes.NewStakeAuthorization(validators, nil, authzType, coin)
	if err != nil {
		return err
	}

	expiration := time.Now().Add(cmn.DefaultExpirationDuration).UTC()
	err = s.nw.App.AuthzKeeper.SaveGrant(ctx, grantee, granter, stakingAuthz, &expiration)
	if err != nil {
		return err
	}

	return nil
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
			liquidstake.UpdateParamsMethod,
			s.precompile.Methods[liquidstake.UpdateParamsMethod],
			true,
		},
		{
			liquidstake.UpdateWhitelistedValidatorsMethod,
			s.precompile.Methods[liquidstake.UpdateWhitelistedValidatorsMethod],
			true,
		},
		{
			liquidstake.SetModulePausedMethod,
			s.precompile.Methods[liquidstake.SetModulePausedMethod],
			true,
		},
		{
			authorization.AllowanceMethod,
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
				err := s.CreateAuthorization(ctx, delegator.AccAddr, grantee.AccAddr, liquidstake.DelegateAuthz, nil)
				s.Require().NoError(err)

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
				err := s.CreateAuthorization(ctx, delegator.AccAddr, grantee.AccAddr, liquidstake.DelegateAuthz, nil)
				s.Require().NoError(err)

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
		//				delAmount := math.NewInt(1000000)
		//				_, err := s.nw.App.StakingKeeper.Delegate(ctx, sdk.AccAddress(delegator.Addr.Bytes()), delAmount, stakingtypes.Bonded, s.validator, false)
		//				if err != nil {
		//					panic(err)
		//				}
		//
		//				delegation, err := s.nw.App.StakingKeeper.GetDelegation(ctx, delegator.AccAddr, sdk.ValAddress(s.validatorAdr.Bytes()))
		//				if err != nil {
		//					panic(err)
		//				}
		//
		//				fmt.Printf("Delegation: %s\n", delegation.String())
		//
		//				// Use a smaller amount that definitely exists in the delegation
		//				tokenizeAmount := big.NewInt(1000000)
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
				err := s.CreateAuthorization(ctx, delegator.AccAddr, grantee.AccAddr, liquidstake.UndelegateAuthz, nil)
				s.Require().NoError(err)

				_, err = s.nw.App.LiquidStakeKeeper.LiquidStake(ctx, liquidstaketypes.LiquidStakeProxyAcc, delegator.AccAddr, sdk.NewCoin(s.bondDenom, math.NewInt(1000000000000000000)))
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

//// TestQueryMethodsWithData tests query methods with some liquid staking data.
//func (s *LiquidStakePrecompileTestSuite) TestQueryMethodsWithData() {
//	s.SetupTest()
//	ctx := s.nw.GetContext().WithBlockTime(time.Now())
//
//	// Perform a liquid stake operation to have some data
//	delegator := s.keyring.GetKey(0)
//	_, err := s.nw.App.LiquidStakeKeeper.LiquidStake(
//		ctx,
//		liquidstaketypes.LiquidStakeProxyAcc,
//		delegator.AccAddr,
//		sdk.NewCoin(s.bondDenom, math.NewInt(1000000)),
//	)
//	s.Require().NoError(err, "failed to perform liquid stake")
//
//	// Test params query
//	contract := vm.NewPrecompile(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, 100000)
//	contract.Input, err = s.precompile.Pack(liquidstake.ParamsMethod)
//	s.Require().NoError(err)
//
//	bz, err := s.precompile.Run(nil, contract, true)
//	s.Require().NoError(err, "params query should succeed")
//	s.Require().NotNil(bz, "params response should not be nil")
//	s.Require().Greater(len(bz), 0, "params response should not be empty")
//
//	// Test liquidValidators query
//	contract.Input, err = s.precompile.Pack(liquidstake.LiquidValidatorsMethod)
//	s.Require().NoError(err)
//
//	bz, err = s.precompile.Run(nil, contract, true)
//	s.Require().NoError(err, "liquidValidators query should succeed")
//	s.Require().NotNil(bz, "liquidValidators response should not be nil")
//	s.Require().Greater(len(bz), 0, "liquidValidators response should not be empty")
//
//	// Test states query
//	contract.Input, err = s.precompile.Pack(liquidstake.StatesMethod)
//	s.Require().NoError(err)
//
//	bz, err = s.precompile.Run(nil, contract, true)
//	s.Require().NoError(err, "states query should succeed")
//	s.Require().NotNil(bz, "states response should not be nil")
//	s.Require().Greater(len(bz), 0, "states response should not be empty")
//}
//
