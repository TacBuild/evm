package liquidstake_test

//import (
//	liquidstake "github.com/cosmos/evm/x/liquidstake/precompile"
//	"time"
//	"github.com/cosmos/evm/precompiles/authorization"
//	cmn "github.com/cosmos/evm/precompiles/common"
//
//	"math/big"
//
//	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
//	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
//
//	sdk "github.com/cosmos/cosmos-sdk/types"
//
//	testkeyring "github.com/cosmos/evm/testutil/integration/os/keyring"
//
//	"github.com/ethereum/go-ethereum/core/vm"
//	"github.com/cosmos/evm/x/vm/statedb"
//	evmtypes "github.com/cosmos/evm/x/vm/types"
//	ethtypes "github.com/ethereum/go-ethereum/core/types"
//	"github.com/ethereum/go-ethereum/common"
//	chainutil "github.com/cosmos/evm/evmd/testutil"
//)
//
//import (
//	"testing"
//
//	"cosmossdk.io/log"
//	dbm "github.com/cosmos/cosmos-db"
//	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
//	"github.com/stretchr/testify/suite"
//
//	"github.com/cosmos/evm/testutil/integration/os/factory"
//)
//
//type LiquidStakePrecompileTestSuite struct {
//	suite.Suite
//
//	ctx sdk.Context
//	// testAccounts []*testAccount
//
//	bondDenom  string
//	precompile *liquidstake.Precompile
//
//	keyring     testkeyring.Keyring
//
//	factory     factory.TxFactory
//}
//
//func TestLiquidStakePrecompileTestSuite(t *testing.T) {
//	suite.Run(t, new(LiquidStakePrecompileTestSuite))
//}
//
//func (s *LiquidStakePrecompileTestSuite) SetupTest() {
////	keyring := testkeyring.New(2)
////	// Create full TacChain app instance similar to keeper tests
////	app := chain.NewTacChainAppWithCustomOptions(
////		s.T(),
////		false,
////		0,
////		chain.SetupOptions{
////			Logger:  log.NewNopLogger(),
////			DB:      dbm.NewMemDB(),
////			AppOpts: simtestutil.NewAppOptionsWithFlagHome(s.T().TempDir()),
////		},
////	)
////
////
////	s.keyring = keyring
////
////
////	// Create context
////	ctx := app.BaseApp.NewContext(false)
////
////	// Get bond denomination
////	bondDenom := app.LiquidStakeKeeper.GetParams(ctx).LiquidBondDenom
////
////	// Set fields
////	s.ctx = ctx
////	s.bondDenom = bondDenom
////
////	// Verify liquidstake module account exists
////	moduleAcc := app.AccountKeeper.GetModuleAccount(ctx, liquidstaketypes.ModuleName)
////	if moduleAcc == nil {
////		panic("liquidstake module account not found - this should be set up by the full app")
////	}
////
////	// Create precompile with the liquidstake keeper from the app
////	var err error
////	if s.precompile, err = liquidstake.NewPrecompile(
////		app.LiquidStakeKeeper,
////		app.AuthzKeeper,
////	); err != nil {
////		panic(err)
////	}
//}
//
//// // Simple test helper structure
//// type testAccount struct {
//// 	privKey *secp256k1.PrivKey
//// 	addr    sdk.AccAddress
//// 	ethAddr common.Address
//// }
////
//// func newTestAccount() *testAccount {
//// 	privKey := secp256k1.GenPrivKey()
//// 	addr := sdk.AccAddress(privKey.PubKey().Address())
//// 	ethAddr := common.BytesToAddress(addr.Bytes())
//// 	return &testAccount{
//// 		privKey: privKey,
//// 		addr:    addr,
//// 		ethAddr: ethAddr,
//// 	}
//// }
//
//func (s *LiquidStakePrecompileTestSuite) CreateAuthorization(ctx sdk.Context, granter, grantee sdk.AccAddress, authzType stakingtypes.AuthorizationType, coin *sdk.Coin) error {
//	// Get all available validators and filter out jailed validators
//	validators := make([]sdk.ValAddress, 0)
//	err := s.app.StakingKeeper.IterateValidators(
//		ctx, func(_ int64, validator stakingtypes.ValidatorI) (stop bool) {
//			if validator.IsJailed() {
//				return
//			}
//			validators = append(validators, sdk.ValAddress(validator.GetOperator()))
//			return
//		},
//	)
//	if err != nil {
//		return err
//	}
//
//	stakingAuthz, err := stakingtypes.NewStakeAuthorization(validators, nil, authzType, coin)
//	if err != nil {
//		return err
//	}
//
//	expiration := time.Now().Add(cmn.DefaultExpirationDuration).UTC()
//	err = s.app.AuthzKeeper.SaveGrant(ctx, grantee, granter, stakingAuthz, &expiration)
//	if err != nil {
//		return err
//	}
//
//	return nil
//}
//
//func (s *LiquidStakePrecompileTestSuite) TestRun() {
//	var ctx sdk.Context
//	testcases := []struct {
//		name        string
//		malleate    func(delegator, grantee testkeyring.Key) []byte
//		gas         uint64
//		readOnly    bool
//		expPass     bool
//		errContains string
//	}{
//		{
//			"fail - contract gas limit is < gas cost to run a query / tx",
//			func(delegator, grantee testkeyring.Key) []byte {
//				// TODO: why is this required?
//				err := s.CreateAuthorization(ctx, delegator.AccAddr, grantee.AccAddr, liquidstake.DelegateAuthz, nil)
//				s.Require().NoError(err)
//
//				input, err := s.precompile.Pack(
//					liquidstake.LiquidStakeMethod,
//					delegator.Addr,
//					big.NewInt(1000),
//				)
//				s.Require().NoError(err, "failed to pack input")
//				return input
//			},
//			8000,
//			false,
//			false,
//			"out of gas",
//		},
//	}
//
//	for _, tc := range testcases {
//		s.Run(tc.name, func() {
//			// setup basic test suite
//			s.SetupTest()
//			ctx = s.app.NewContext(false).WithBlockTime(time.Now())
//
//			baseFee := s.app.EVMKeeper.GetBaseFee(ctx)
//
//			delegator := s.keyring.GetKey(0)
//			grantee := s.keyring.GetKey(1)
//
//			contract := vm.NewPrecompile(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, tc.gas)
//			contractAddr := contract.Address()
//
//			// malleate testcase
//			contract.Input = tc.malleate(delegator, grantee)
//
//			// Build and sign Ethereum transaction
//			txArgs := evmtypes.EvmTxArgs{
//				ChainID:   evmtypes.GetEthChainConfig().ChainID,
//				Nonce:     0,
//				To:        &contractAddr,
//				Amount:    nil,
//				GasLimit:  tc.gas,
//				GasPrice:  chainutil.ExampleMinGasPrices.BigInt(),
//				GasFeeCap: baseFee,
//				GasTipCap: big.NewInt(1),
//				Accesses:  &ethtypes.AccessList{},
//			}
//
//			msg, err := s.factory.GenerateGethCoreMsg(delegator.Priv, txArgs)
//			s.Require().NoError(err)
//
//			// Instantiate config
//			proposerAddress := ctx.BlockHeader().ProposerAddress
//			cfg, err := s.app.EVMKeeper.EVMConfig(ctx, proposerAddress)
//			s.Require().NoError(err, "failed to instantiate EVM config")
//
//			// Instantiate EVM
//			headerHash := ctx.HeaderHash()
//			stDB := statedb.New(
//				ctx,
//				s.app.EVMKeeper,
//				statedb.NewEmptyTxConfig(common.BytesToHash(headerHash)),
//			)
//			evm := s.app.EVMKeeper.NewEVM(
//				ctx, *msg, cfg, nil, stDB,
//			)
//
//			precompiles, found, err := s.app.EVMKeeper.GetPrecompileInstance(ctx, contractAddr)
//			s.Require().NoError(err, "failed to instantiate precompile")
//			s.Require().True(found, "not found precompile")
//			evm.WithPrecompiles(precompiles.Map, precompiles.Addresses)
//
//			// Run precompiled contract
//			bz, err := s.precompile.Run(evm, contract, tc.readOnly)
//
//			// Check results
//			if tc.expPass {
//				s.Require().NoError(err, "expected no error when running the precompile")
//				s.Require().NotNil(bz, "expected returned bytes not to be nil")
//			} else {
//				s.Require().Error(err, "expected error to be returned when running the precompile")
//				s.Require().Nil(bz, "expected returned bytes to be nil")
//				s.Require().ErrorContains(err, tc.errContains)
//				consumed := ctx.GasMeter().GasConsumed()
//				// LessThanOrEqual because the gas is consumed before the error is returned
//				s.Require().LessOrEqual(tc.gas, consumed, "expected gas consumed to be equal to gas limit")
//
//			}
//		})
//	}
//}
//
//func (s *LiquidStakePrecompileTestSuite) TestPrecompileInitialization() {
//	// Test that precompile is properly initialized
//	s.Require().NotNil(s.precompile)
//	s.Require().NotNil(s.app.LiquidStakeKeeper)
//	s.Require().NotNil(s.app.AuthzKeeper)
//
//	// Test that liquidstake module account exists
//	moduleAcc := s.app.AccountKeeper.GetModuleAccount(s.ctx, liquidstaketypes.ModuleName)
//	s.Require().NotNil(moduleAcc)
//	s.Require().Equal(liquidstaketypes.ModuleName, moduleAcc.GetName())
//}
//
//func (s *LiquidStakePrecompileTestSuite) TestPrecompileMethodSignatures() {
//	// Test that all expected methods are registered
//	methods := s.precompile.Methods
//
//	s.Require().Contains(methods, liquidstake.LiquidStakeMethod)
//	s.Require().Contains(methods, liquidstake.StakeToLPMethod)
//	s.Require().Contains(methods, liquidstake.LiquidUnstakeMethod)
//	s.Require().Contains(methods, authorization.ApproveMethod)
//	s.Require().Contains(methods, authorization.AllowanceMethod)
//}
