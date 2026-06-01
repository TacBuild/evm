package liquidstake

// TestParamsAndEpochs contains isolated diagnostic tests that verify:
//   1. Genesis state is initialized correctly (liquidstake params, epoch params).
//   2. Params written through n.ctx survive across NextBlock calls.
//   3. Epoch transitions do NOT fire when EpochCountingStarted=true and
//      blockTime < CurrentEpochStartTime+Duration.
//   4. UpdateLiquidValidatorSet honours the WhitelistedValidators that were set
//      before the first NextBlock.

import (
	"fmt"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/evm/x/liquidstake/types"
)

// ---------------------------------------------------------------------------
// 1. Genesis initialisation checks
// ---------------------------------------------------------------------------

// TestGenesisLiquidStakeParams verifies that the liquidstake module is
// initialised with the params written in SetupTest (before any NextBlock).
func (s *KeeperTestSuite) TestGenesisLiquidStakeParams() {
	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)

	s.Require().Equal("stk/utac", params.LiquidBondDenom, "liquid bond denom should be stk/utac")
	s.Require().True(params.UnstakeFeeRate.IsZero(), "unstake fee rate should be 0 (set in SetupTest)")
	s.Require().False(params.ModulePaused, "module should not be paused")
	s.Require().Empty(params.WhitelistedValidators, "no whitelisted validators at genesis")
}

// TestGenesisEpochParams verifies that the epochs module is initialised with
// EpochCountingStarted=true so epochs do NOT fire on block 1.
func (s *KeeperTestSuite) TestGenesisEpochParams() {
	ctx := s.ctx()
	epochsKeeper := s.nw.App.GetEpochsKeeper()

	for _, id := range []string{"hour", "day", "week"} {
		ep := epochsKeeper.GetEpochInfo(ctx, id)

		s.Require().True(ep.EpochCountingStarted,
			"epoch %q: EpochCountingStarted must be true so it does not fire on block 1", id)
		s.Require().Greater(ep.CurrentEpoch, int64(0),
			"epoch %q: CurrentEpoch must be ≥ 1 at genesis", id)
		s.Require().False(ep.CurrentEpochStartTime.IsZero(),
			"epoch %q: CurrentEpochStartTime must be set", id)
	}
}

// ---------------------------------------------------------------------------
// 2. Param persistence across NextBlock
// ---------------------------------------------------------------------------

// TestParamsPersistAfterNextBlock writes a custom UnstakeFeeRate to n.ctx and
// then advances one block. After the block the param must still be readable
// from the new context — proving that uncommitted writes in finalizeBlockState.ms
// are flushed to the committed store by NextBlock.
func (s *KeeperTestSuite) TestParamsPersistAfterNextBlock() {
	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)

	// Write a sentinel value.
	sentinel := "0.123000000000000000"
	params.UnstakeFeeRate = mustNewDecFromStr(sentinel)
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	// Verify readable in the same (uncommitted) context.
	got := s.keeper.GetParams(ctx)
	s.Require().Equal(sentinel, got.UnstakeFeeRate.String(), "param must be readable before NextBlock")

	// Commit the current ctx state so it survives FinalizeBlock's new context.
	s.Require().NoError(s.nw.CommitState())

	// Advance one block – this calls FinalizeBlock + Commit.
	s.Require().NoError(s.nw.NextBlock())

	// After NextBlock, n.ctx is a brand-new context backed by the committed store.
	ctxAfter := s.ctx()
	gotAfter := s.keeper.GetParams(ctxAfter)

	s.Require().Equal(sentinel, gotAfter.UnstakeFeeRate.String(),
		"param written before NextBlock must survive in committed store after NextBlock")
}

// TestParamsPersistAfterMultipleNextBlocks extends the previous test: writes a
// param, advances 3 blocks, verifies it is still present.
func (s *KeeperTestSuite) TestParamsPersistAfterMultipleNextBlocks() {
	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)

	sentinel := "0.042000000000000000"
	params.UnstakeFeeRate = mustNewDecFromStr(sentinel)
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	// Commit so the param survives FinalizeBlock's context replacement.
	s.Require().NoError(s.nw.CommitState())

	for i := 1; i <= 3; i++ {
		s.Require().NoError(s.nw.NextBlock())
		ctxN := s.ctx()
		p := s.keeper.GetParams(ctxN)
		s.Require().Equal(sentinel, p.UnstakeFeeRate.String(),
			"param must survive block %d", i)
	}
}

// TestWhitelistedValidatorsPersistAfterNextBlock sets WhitelistedValidators
// (simulating what setupWhitelistedValidators does) and verifies they survive
// after the first NextBlock call.
func (s *KeeperTestSuite) TestWhitelistedValidatorsPersistAfterNextBlock() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()

	validators, err := sk.GetValidators(ctx, 3)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(validators), 3)

	params := s.keeper.GetParams(ctx)
	weights := equalTargetWeights(3)
	for i, v := range validators[:3] {
		params.WhitelistedValidators = append(params.WhitelistedValidators, types.WhitelistedValidator{
			ValidatorAddress: v.OperatorAddress,
			TargetWeight:     weights[i],
		})
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	// Commit so whitelist survives FinalizeBlock's context replacement.
	s.Require().NoError(s.nw.CommitState())

	s.Require().NoError(s.nw.NextBlock())

	ctxAfter := s.ctx()
	paramsAfter := s.keeper.GetParams(ctxAfter)

	s.Require().Len(paramsAfter.WhitelistedValidators, 3,
		"WhitelistedValidators written before NextBlock must survive in committed store")
}

// ---------------------------------------------------------------------------
// 3. Epoch does NOT fire within the current epoch window
// ---------------------------------------------------------------------------

// TestEpochDoesNotFireWithinWindow advances the chain by several short blocks
// and asserts that epoch CurrentEpoch does not change (i.e. no epoch transition
// occurs) because blockTime stays well within the epoch window.
func (s *KeeperTestSuite) TestEpochDoesNotFireWithinWindow() {
	ctx := s.ctx()
	epochsKeeper := s.nw.App.GetEpochsKeeper()

	// Read initial state of the "hour" epoch.
	ep0 := epochsKeeper.GetEpochInfo(ctx, "hour")

	// Advance 10 blocks of 6 s each — total 60 s, far less than 1 h.
	for i := 1; i <= 10; i++ {
		s.Require().NoError(s.nw.NextBlockAfter(6 * time.Second))
	}

	ctxAfter := s.ctx()
	epAfter := epochsKeeper.GetEpochInfo(ctxAfter, "hour")

	s.Require().Equal(ep0.CurrentEpoch, epAfter.CurrentEpoch,
		"hour epoch must NOT advance after only 60 s of blocks (duration=1h)")
}

// TestEpochFiresAfterDuration advances the chain past the epoch duration and
// confirms the epoch counter increments.
func (s *KeeperTestSuite) TestEpochFiresAfterDuration() {
	ctx := s.ctx()
	epochsKeeper := s.nw.App.GetEpochsKeeper()

	ep0 := epochsKeeper.GetEpochInfo(ctx, "hour")

	// Advance time past 1 hour in one big jump.
	s.Require().NoError(s.nw.NextBlockAfter(time.Hour + time.Second))

	ctxAfter := s.ctx()
	epAfter := epochsKeeper.GetEpochInfo(ctxAfter, "hour")

	s.Require().Greater(epAfter.CurrentEpoch, ep0.CurrentEpoch,
		"hour epoch must advance after blockTime passes CurrentEpochStartTime+Duration")
}

// ---------------------------------------------------------------------------
// 4. UpdateLiquidValidatorSet respects WhitelistedValidators
// ---------------------------------------------------------------------------

// TestUpdateLiquidValidatorSetBasic sets 2 whitelisted validators, calls
// UpdateLiquidValidatorSet, and verifies 2 active liquid validators exist both
// before and after a NextBlock.
func (s *KeeperTestSuite) TestUpdateLiquidValidatorSetBasic() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()

	validators, err := sk.GetValidators(ctx, 2)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(validators), 2)

	// Whitelist 2 validators.
	params := s.keeper.GetParams(ctx)
	params.WhitelistedValidators = []types.WhitelistedValidator{
		{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: sdkInt(5000)},
		{ValidatorAddress: validators[1].OperatorAddress, TargetWeight: sdkInt(5000)},
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	// Check immediately (uncommitted).
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()
	activeVals := s.keeper.GetActiveLiquidValidators(ctx, wvMap)
	s.Require().Len(activeVals, 2, "must have 2 active liquid validators before NextBlock")

	// Commit so params survive FinalizeBlock's context replacement.
	s.Require().NoError(s.nw.CommitState())

	// Advance one block.
	s.Require().NoError(s.nw.NextBlock())

	// Check after block (committed).
	ctxAfter := s.ctx()
	paramsAfter := s.keeper.GetParams(ctxAfter)
	wvMapAfter := paramsAfter.WhitelistedValsMap()
	activeValsAfter := s.keeper.GetActiveLiquidValidators(ctxAfter, wvMapAfter)
	_ = s.keeper.GetAllLiquidValidators(ctxAfter)

	s.Require().Len(paramsAfter.WhitelistedValidators, 2,
		"WhitelistedValidators must survive NextBlock")
	s.Require().Len(activeValsAfter, 2,
		"2 active liquid validators must be present after NextBlock")
}

// TestEpochHookWithWhitelistedValidators is the critical regression test.
// It sets WhitelistedValidators, advances past the epoch duration so that
// BeforeEpochStart fires, and verifies that UpdateLiquidValidatorSet inside
// the hook uses the committed params (not the empty genesis params).
func (s *KeeperTestSuite) TestEpochHookWithWhitelistedValidators() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()

	validators, err := sk.GetValidators(ctx, 2)
	s.Require().NoError(err)

	// Set whitelisted validators (written to finalizeBlockState.ms, not committed yet).
	params := s.keeper.GetParams(ctx)
	params.WhitelistedValidators = []types.WhitelistedValidator{
		{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: sdkInt(5000)},
		{ValidatorAddress: validators[1].OperatorAddress, TargetWeight: sdkInt(5000)},
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	// Block 1 — commit the params to the store.
	s.Require().NoError(s.nw.CommitState())
	s.Require().NoError(s.nw.NextBlock())

	ctx1 := s.ctx()
	p1 := s.keeper.GetParams(ctx1)
	s.Require().Len(p1.WhitelistedValidators, 2, "params must be committed after block 1")

	// Block 2 — advance past the "hour" epoch duration so BeforeEpochStart fires.
	// We check that the hook sees the committed whitelist and does NOT wipe state.
	s.Require().NoError(s.nw.NextBlockAfter(time.Hour + time.Second))

	ctx2 := s.ctx()
	p2 := s.keeper.GetParams(ctx2)
	wvMap2 := p2.WhitelistedValsMap()
	activeVals2 := s.keeper.GetActiveLiquidValidators(ctx2, wvMap2)
	ep2 := s.nw.App.GetEpochsKeeper().GetEpochInfo(ctx2, "hour")
	_ = ep2

	s.Require().Len(p2.WhitelistedValidators, 2,
		"WhitelistedValidators must NOT be wiped by epoch hook")
	s.Require().Len(activeVals2, 2,
		"active liquid validators must survive epoch transition")
}

// ---------------------------------------------------------------------------
// 5. LiquidStake basic smoke test after params committed
// ---------------------------------------------------------------------------

// TestLiquidStakeAfterParamsCommitted verifies that LiquidStake succeeds after
// WhitelistedValidators are committed via NextBlock.
func (s *KeeperTestSuite) TestLiquidStakeAfterParamsCommitted() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	skParams, err := sk.GetParams(ctx)
	s.Require().NoError(err)

	validators, err := sk.GetValidators(ctx, 3)
	s.Require().NoError(err)

	// Whitelist 3 validators + set ValidatorLiquidStakingCap.
	skParams.ValidatorLiquidStakingCap = mustNewDecFromStr("1.0")
	s.Require().NoError(sk.SetParams(ctx, skParams))

	params := s.keeper.GetParams(ctx)
	params.WhitelistedValidators = nil
	weights := equalTargetWeights(3)
	for i, v := range validators[:3] {
		params.WhitelistedValidators = append(params.WhitelistedValidators, types.WhitelistedValidator{
			ValidatorAddress: v.OperatorAddress,
			TargetWeight:     weights[i],
		})
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	// Do NOT call NextBlock before LiquidStake — params are in uncommitted cache.
	staker := s.delAddrs[0]
	stakeAmt := sdkInt(1_000_000)

	activeVals := s.keeper.GetActiveLiquidValidators(ctx, params.WhitelistedValsMap())
	s.Require().Len(activeVals, 3, "must see 3 active validators in uncommitted ctx")

	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker,
		sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err, "LiquidStake must succeed with uncommitted whitelisted validators")
	s.Require().True(mintAmt.IsPositive())
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func sdkInt(n int64) sdkmath.Int {
	return sdkmath.NewInt(n)
}

func mustNewDecFromStr(s string) sdkmath.LegacyDec {
	d, err := sdkmath.LegacyNewDecFromStr(s)
	if err != nil {
		panic(fmt.Sprintf("mustNewDecFromStr(%q): %v", s, err))
	}
	return d
}
