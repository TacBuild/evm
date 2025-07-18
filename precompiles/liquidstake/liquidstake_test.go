package liquidstake_test

import (
	"time"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"
	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
	sdkmath "cosmossdk.io/math"

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
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/suite"
)

type LiquidStakePrecompileTestSuite struct {
	suite.Suite

	nw     *network.UnitTestNetwork
	factory     factory.TxFactory
	grpcHandler grpc.Handler
	keyring     testkeyring.Keyring

	bondDenom  string
	precompile *liquidstake.Precompile
}

func TestPrecompileUnitTestSuite(t *testing.T) {
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

	validatorAddrs, err := s.nw.App.StakingKeeper.GetValidators(ctx, 1)
	if err != nil {
		panic(err)
	}

	params := s.nw.App.LiquidStakeKeeper.GetParams(ctx)
	params.ModulePaused = false
	params.LiquidBondDenom = "aatom"

	s.nw.App.LiquidStakeKeeper.SetLiquidValidator(ctx,liquidstaketypes.LiquidValidator {
		OperatorAddress: validatorAddrs[0].OperatorAddress,
	})

	params.WhitelistedValidators = append(params.WhitelistedValidators,	liquidstaketypes.WhitelistedValidator {
		ValidatorAddress: validatorAddrs[0].OperatorAddress,
		TargetWeight: sdkmath.NewInt(10000),
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
				// TODO: why is this required?
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
			8000000,
			false,
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
				s.Require().LessOrEqual(tc.gas, consumed, "expected gas consumed to be equal to gas limit")

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
}
