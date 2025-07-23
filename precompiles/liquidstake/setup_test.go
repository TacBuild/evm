package liquidstake_test

//import (
//	"testing"
//
//	"cosmossdk.io/log"
//	chain "github.com/cosmos/evm/app"
//	liquidstake "github.com/cosmos/evm/x/liquidstake/precompile"
//	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
//	dbm "github.com/cosmos/cosmos-db"
//	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
//	sdk "github.com/cosmos/cosmos-sdk/types"
//	"github.com/stretchr/testify/suite"
//
//	testkeyring "github.com/cosmos/evm/testutil/integration/os/keyring"
//	"github.com/cosmos/evm/testutil/integration/os/network"
//
//	"github.com/cosmos/evm/testutil/integration/os/factory"
//)
//
//type LiquidStakePrecompileTestSuite struct {
//	suite.Suite
//
//	app *chain.TacChainApp
//	ctx sdk.Context
//	// testAccounts []*testAccount
//
//	bondDenom  string
//	precompile *liquidstake.Precompile
//
//	network     *network.UnitTestNetwork
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
//	keyring := testkeyring.New(2)
//	// Create full TacChain app instance similar to keeper tests
//	app := chain.NewTacChainAppWithCustomOptions(
//		s.T(),
//		false,
//		0,
//		chain.SetupOptions{
//			Logger:  log.NewNopLogger(),
//			DB:      dbm.NewMemDB(),
//			AppOpts: simtestutil.NewAppOptionsWithFlagHome(s.T().TempDir()),
//		},
//	)
//
//	nw := network.NewUnitTestNetwork(
//		network.WithPreFundedAccounts(keyring.GetAllAccAddrs()...),
//	)
//
//	s.network = nw
//	s.keyring = keyring
//
//
//	// Create context
//	ctx := app.BaseApp.NewContext(false)
//
//	// Get bond denomination
//	bondDenom := app.LiquidStakeKeeper.GetParams(ctx).LiquidBondDenom
//
//	// Set fields
//	s.app = app
//	s.ctx = ctx
//	s.bondDenom = bondDenom
//
//	// Verify liquidstake module account exists
//	moduleAcc := app.AccountKeeper.GetModuleAccount(ctx, liquidstaketypes.ModuleName)
//	if moduleAcc == nil {
//		panic("liquidstake module account not found - this should be set up by the full app")
//	}
//
//	// Create precompile with the liquidstake keeper from the app
//	var err error
//	if s.precompile, err = liquidstake.NewPrecompile(
//		app.LiquidStakeKeeper,
//		app.AuthzKeeper,
//	); err != nil {
//		panic(err)
//	}
//}
