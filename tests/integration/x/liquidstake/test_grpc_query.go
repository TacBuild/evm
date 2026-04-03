package liquidstake

// test_grpc_query.go covers keeper.Querier — the gRPC query server for x/liquidstake.
// All three endpoints are tested: Params, LiquidValidators, States.

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/evm/x/liquidstake/types"
)

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

// TestQuerier_Params_Default verifies that Params returns the current module params.
func (s *KeeperTestSuite) TestQuerier_Params_Default() {
	ctx := s.ctx()
	resp, err := s.querier.Params(ctx, &types.QueryParamsRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	got := resp.Params
	expected := s.keeper.GetParams(ctx)
	s.Require().Equal(expected.LiquidBondDenom, got.LiquidBondDenom)
	s.Require().Equal(expected.UnstakeFeeRate, got.UnstakeFeeRate)
	s.Require().Equal(expected.MinLiquidStakeAmount, got.MinLiquidStakeAmount)
	s.Require().Equal(expected.LsmDisabled, got.LsmDisabled)
}

// TestQuerier_Params_AfterUpdate verifies that Params reflects a params update immediately.
func (s *KeeperTestSuite) TestQuerier_Params_AfterUpdate() {
	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	params.UnstakeFeeRate = sdkmath.LegacyMustNewDecFromStr("0.003")
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	resp, err := s.querier.Params(ctx, &types.QueryParamsRequest{})
	s.Require().NoError(err)
	s.Require().Equal(sdkmath.LegacyMustNewDecFromStr("0.003"), resp.Params.UnstakeFeeRate)
}

// TestQuerier_Params_NilRequest verifies that Params accepts a nil request (no pagination).
func (s *KeeperTestSuite) TestQuerier_Params_NilRequest() {
	ctx := s.ctx()
	// Params ignores the request object entirely.
	resp, err := s.querier.Params(ctx, nil)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
}

// ---------------------------------------------------------------------------
// LiquidValidators
// ---------------------------------------------------------------------------

// TestQuerier_LiquidValidators_Empty verifies that LiquidValidators returns an empty
// list when no liquid validators have been registered.
func (s *KeeperTestSuite) TestQuerier_LiquidValidators_Empty() {
	ctx := s.ctx()
	resp, err := s.querier.LiquidValidators(ctx, &types.QueryLiquidValidatorsRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	// Default setup has no whitelisted validators → empty list.
	s.Require().Empty(resp.LiquidValidators)
}

// TestQuerier_LiquidValidators_WithValidators verifies that LiquidValidators returns
// an entry for each whitelisted-and-active validator.
func (s *KeeperTestSuite) TestQuerier_LiquidValidators_WithValidators() {
	s.setupWhitelistedValidators(2, 0)
	ctx := s.ctx()

	resp, err := s.querier.LiquidValidators(ctx, &types.QueryLiquidValidatorsRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.LiquidValidators, 2, "must return 2 liquid validators")

	for _, lv := range resp.LiquidValidators {
		s.Require().NotEmpty(lv.OperatorAddress)
		// Active validators must have a non-zero weight.
		s.Require().True(lv.Weight.IsPositive(), "active validator weight must be positive")
	}
}

// TestQuerier_LiquidValidators_AfterStake verifies that LiquidTokens are populated
// after a liquid stake operation.
func (s *KeeperTestSuite) TestQuerier_LiquidValidators_AfterStake() {
	s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	staker := s.delAddrs[0]
	_, err = s.keeper.LiquidStake(
		ctx,
		types.LiquidStakeProxyAcc,
		staker,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(400_000)),
	)
	s.Require().NoError(err)

	resp, err := s.querier.LiquidValidators(ctx, &types.QueryLiquidValidatorsRequest{})
	s.Require().NoError(err)

	totalLiquidTokens := sdkmath.ZeroInt()
	for _, lv := range resp.LiquidValidators {
		totalLiquidTokens = totalLiquidTokens.Add(lv.LiquidTokens)
	}
	s.Require().True(totalLiquidTokens.IsPositive(), "liquid validators must hold tokens after stake")
}

// TestQuerier_LiquidValidators_NilRequest verifies that a nil request returns an error.
func (s *KeeperTestSuite) TestQuerier_LiquidValidators_NilRequest() {
	ctx := s.ctx()
	_, err := s.querier.LiquidValidators(ctx, nil)
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

// ---------------------------------------------------------------------------
// States
// ---------------------------------------------------------------------------

// TestQuerier_States_Empty verifies that States returns a zeroed NetAmountState
// when no liquid stake has been performed.
func (s *KeeperTestSuite) TestQuerier_States_Empty() {
	ctx := s.ctx()
	resp, err := s.querier.States(ctx, &types.QueryStatesRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	nas := resp.NetAmountState
	s.Require().True(nas.GtacTotalSupply.IsZero(), "gTAC supply must be zero before any stake")
	s.Require().True(nas.TotalLiquidTokens.IsZero(), "liquid tokens must be zero before any stake")
}

// TestQuerier_States_AfterStake verifies that States reflects the staked amount.
func (s *KeeperTestSuite) TestQuerier_States_AfterStake() {
	s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	stakeAmt := sdkmath.NewInt(600_000)
	_, err = s.keeper.LiquidStake(
		ctx,
		types.LiquidStakeProxyAcc,
		s.delAddrs[0],
		sdk.NewCoin(bondDenom, stakeAmt),
	)
	s.Require().NoError(err)

	resp, err := s.querier.States(ctx, &types.QueryStatesRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	nas := resp.NetAmountState
	s.Require().True(nas.GtacTotalSupply.IsPositive(), "gTAC supply must be positive after stake")
	s.Require().True(nas.TotalLiquidTokens.IsPositive(), "liquid tokens must be positive after stake")
	s.Require().True(nas.NetAmount.IsPositive(), "NetAmount must be positive after stake")
	// MintRate should be set (1:1 on first stake).
	s.Require().True(nas.MintRate.IsPositive(), "MintRate must be positive after stake")
}

// TestQuerier_States_NilRequest verifies that a nil request returns an error.
func (s *KeeperTestSuite) TestQuerier_States_NilRequest() {
	ctx := s.ctx()
	_, err := s.querier.States(ctx, nil)
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}
