package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	"github.com/cosmos/cosmos-sdk/x/mint"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	testhelpers "github.com/cosmos/evm/evmd/helpers"
	"github.com/stretchr/testify/suite"

	chain "github.com/cosmos/evm/evmd"
	"github.com/cosmos/evm/testutil/constants"
	"github.com/cosmos/evm/x/liquidstake/keeper"
	"github.com/cosmos/evm/x/liquidstake/types"
)

var BlockTime = 6 * time.Second

type KeeperTestSuite struct {
	suite.Suite

	app      *chain.EVMD
	ctx      sdk.Context
	keeper   keeper.Keeper
	querier  keeper.Querier
	addrs    []sdk.AccAddress
	delAddrs []sdk.AccAddress
	valAddrs []sdk.ValAddress
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}

func (s *KeeperTestSuite) SetupTest() {
	s.app = chain.Setup(s.T(), constants.ExampleChainID)
	s.ctx = s.app.BaseApp.NewContext(false)
	stakingParams := stakingtypes.DefaultParams()
	stakingParams.MaxEntries = 7
	stakingParams.MaxValidators = 30
	s.Require().NoError(s.app.StakingKeeper.SetParams(s.ctx, stakingParams))

	s.keeper = s.app.LiquidStakeKeeper
	s.querier = keeper.Querier{Keeper: s.keeper}
	s.addrs = testhelpers.AddTestAddrs(s.app, s.ctx, 10, math.NewInt(1_000_000_000))
	s.delAddrs = testhelpers.AddTestAddrs(s.app, s.ctx, 10, math.NewInt(1_000_000_000))
	s.valAddrs = testhelpers.ConvertAddrsToValAddrs(s.delAddrs)

	s.ctx = s.ctx.WithBlockHeight(100).WithBlockTime(testhelpers.ParseTime("2022-03-01T00:00:00Z"))
	params := s.keeper.GetParams(s.ctx)
	params.UnstakeFeeRate = sdk.ZeroDec()
	params.AutocompoundFeeRate = types.DefaultAutocompoundFeeRate
	s.Require().NoError(s.keeper.SetParams(s.ctx, params))
	s.keeper.UpdateLiquidValidatorSet(s.ctx, true)
	// call mint.BeginBlocker for init k.SetLastBlockTime(ctx, ctx.BlockTime())
	err := mint.BeginBlocker(s.ctx, s.app.MintKeeper, minttypes.DefaultInflationCalculationFn)
	s.Require().NoError(err)
}

func (s *KeeperTestSuite) TearDownTest() {
	// invariant check
	crisis.EndBlocker(s.ctx, *s.app.CrisisKeeper)
}

func (s *KeeperTestSuite) CreateValidators(powers []int64) ([]sdk.AccAddress, []sdk.ValAddress, []cryptotypes.PubKey) {
	s.app.BeginBlocker(s.ctx)
	num := len(powers)
	addrs := testhelpers.AddTestAddrsIncremental(s.app, s.ctx, num, math.NewInt(10000000000000))
	valAddrs := testhelpers.ConvertAddrsToValAddrs(addrs)
	pks := testhelpers.CreateTestPubKeys(num)
	skParams, err := s.app.StakingKeeper.GetParams(s.ctx)
	s.Require().NoError(err)
	skParams.ValidatorLiquidStakingCap = sdk.OneDec()
	_ = s.app.StakingKeeper.SetParams(s.ctx, skParams)
	for i, power := range powers {
		val, err := stakingtypes.NewValidator(valAddrs[i].String(), pks[i], stakingtypes.Description{})
		s.Require().NoError(err)
		s.app.StakingKeeper.SetValidator(s.ctx, val)
		err = s.app.StakingKeeper.SetValidatorByConsAddr(s.ctx, val)
		s.Require().NoError(err)
		s.app.StakingKeeper.SetNewValidatorByPowerIndex(s.ctx, val)
		_ = s.app.StakingKeeper.Hooks().AfterValidatorCreated(s.ctx, valAddrs[i])
		newShares, err := s.app.StakingKeeper.Delegate(s.ctx, addrs[i], math.NewInt(power), stakingtypes.Unbonded, val, true)
		s.Require().NoError(err)
		s.Require().Equal(newShares.TruncateInt(), math.NewInt(power))
		msgValidatorBond := &stakingtypes.MsgValidatorBond{
			DelegatorAddress: addrs[i].String(),
			ValidatorAddress: val.OperatorAddress,
		}
		handler := s.app.MsgServiceRouter().Handler(msgValidatorBond)
		_, err = handler(s.ctx, msgValidatorBond)
		s.Require().NoError(err)
	}

	s.app.EndBlocker(s.ctx)
	return addrs, valAddrs, pks
}

func (s *KeeperTestSuite) advanceHeight(height int, _ bool) {
	for range height {
		s.ctx = s.ctx.WithBlockHeight(s.ctx.BlockHeight() + 1).WithBlockTime(s.ctx.BlockTime().Add(BlockTime))
		s.app.BeginBlocker(s.ctx)
		s.app.EndBlocker(s.ctx)
	}
}
