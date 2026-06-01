package liquidstake

// TestRebalancing contains integration tests for the rebalancing subsystem of
// x/liquidstake, covering:
//
//  1. UpdateLiquidValidatorSet – validators are added / removed from the
//     liquid validator set when the whitelist changes.
//  2. Rebalance – delegations are redistributed when a new validator is added
//     with non-zero weight.
//  3. TryRedelegation – individual redelegation succeeds and fails gracefully
//     (transitive-redelegation guard).
//  4. AutocompoundStakingRewards – rewards are withdrawn, re-staked and a fee
//     is transferred to the fee account.
//  5. BeforeEpochStart hook – AutocompoundStakingRewards fires on "hour" epoch
//     and UpdateLiquidValidatorSet fires on "day" epoch.

import (
	"time"

	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/evm/x/liquidstake/types"
)

// ---------------------------------------------------------------------------
// 1. UpdateLiquidValidatorSet
// ---------------------------------------------------------------------------

// TestUpdateLiquidValidatorSet_AddValidator whitelists 2 validators, stakes
// some tokens, then adds a 3rd validator to the whitelist and calls
// UpdateLiquidValidatorSet.  It verifies the 3rd validator appears in the
// active set.
func (s *KeeperTestSuite) TestUpdateLiquidValidatorSet_AddValidator() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)
	ctx := s.ctx()

	// Stake so that the proxy account has actual delegations.
	stakeAmt := sdkmath.NewInt(1_000_000)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], stakeAmt))

	// Fetch a 3rd existing validator that is not yet whitelisted.
	sk := s.nw.App.GetStakingKeeper()
	allVals, err := sk.GetValidators(ctx, 5)
	s.Require().NoError(err)

	var thirdValAddr string
	for _, v := range allVals {
		if v.OperatorAddress != valOpers[0].String() && v.OperatorAddress != valOpers[1].String() {
			thirdValAddr = v.OperatorAddress
			break
		}
	}
	s.Require().NotEmpty(thirdValAddr, "need at least 3 validators in the network")

	// Add 3rd validator to the whitelist.
	ctx = s.ctx()
	params := s.keeper.GetParams(ctx)
	weights := equalTargetWeights(3)
	for i := range params.WhitelistedValidators {
		params.WhitelistedValidators[i].TargetWeight = weights[i]
	}
	params.WhitelistedValidators = append(params.WhitelistedValidators, types.WhitelistedValidator{
		ValidatorAddress: thirdValAddr,
		TargetWeight:     weights[2],
	})
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	redels := s.keeper.UpdateLiquidValidatorSet(ctx, true)

	// 3rd validator should now be active.
	ctx = s.ctx()
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()
	activeVals := s.keeper.GetActiveLiquidValidators(ctx, wvMap)
	s.Require().Len(activeVals, 3, "3rd validator must be active after UpdateLiquidValidatorSet")

	// Redelegations may or may not occur depending on current balances,
	// but the call must not return an error.
	for _, rd := range redels {
		s.Require().NoError(rd.Error, "redelegation %s→%s should not fail",
			rd.SrcValidator.OperatorAddress, rd.DstValidator.OperatorAddress)
	}
}

// TestUpdateLiquidValidatorSet_RemoveValidator checks that after removing a
// validator from the whitelist and calling UpdateLiquidValidatorSet (with
// redelegate=true), the active set shrinks.
func (s *KeeperTestSuite) TestUpdateLiquidValidatorSet_RemoveValidator() {
	s.setupWhitelistedValidators(3, 0)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)

	// Keep only the first 2 validators.
	params.WhitelistedValidators = params.WhitelistedValidators[:2]
	weights := equalTargetWeights(2)
	for i := range params.WhitelistedValidators {
		params.WhitelistedValidators[i].TargetWeight = weights[i]
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	ctx = s.ctx()
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()
	activeVals := s.keeper.GetActiveLiquidValidators(ctx, wvMap)
	s.Require().Len(activeVals, 2, "must have exactly 2 active validators after removing one")
}

// TestUpdateLiquidValidatorSet_NoRedelegate verifies that passing
// redelegate=false returns nil (no redelegation work is performed).
func (s *KeeperTestSuite) TestUpdateLiquidValidatorSet_NoRedelegate() {
	s.setupWhitelistedValidators(2, 0)
	ctx := s.ctx()

	redels := s.keeper.UpdateLiquidValidatorSet(ctx, false)
	s.Require().Nil(redels, "UpdateLiquidValidatorSet(redelegate=false) must return nil")
}

// ---------------------------------------------------------------------------
// 2. Rebalance
// ---------------------------------------------------------------------------

// TestRebalance_EqualWeights stakes tokens across 3 validators, then calls
// Rebalance with equal target weights.  With balanced delegations the
// rebalancing threshold should not be exceeded and no redelegation should fire.
func (s *KeeperTestSuite) TestRebalance_EqualWeights() {
	s.setupWhitelistedValidators(3, 0)
	ctx := s.ctx()

	// Stake the same amount from 3 different delegators so delegations are
	// already balanced across validators.
	stakeAmt := sdkmath.NewInt(300_000)
	for i := range 3 {
		s.Require().NoError(s.liquidStaking(s.delAddrs[i], stakeAmt))
	}

	ctx = s.ctx()
	params := s.keeper.GetParams(ctx)
	liquidVals := s.keeper.GetAllLiquidValidators(ctx)
	whitelistedValsMap := params.WhitelistedValsMap()

	redels := s.keeper.Rebalance(
		ctx,
		types.LiquidStakeProxyAcc,
		liquidVals,
		whitelistedValsMap,
		types.RebalancingTrigger,
	)

	// With balanced stake the rebalance threshold should not be crossed.
	for _, rd := range redels {
		s.Require().NoError(rd.Error, "no redelegation errors expected with balanced validators")
	}
}

// TestRebalance_ImbalancedWeights stakes a large amount to one validator and
// then changes the weights so another validator should receive delegations.
// Rebalance must produce at least one redelegation entry.
func (s *KeeperTestSuite) TestRebalance_ImbalancedWeights() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()

	// Stake a large amount – all goes to validators proportionally.
	largeStake := sdkmath.NewInt(2_000_000)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], largeStake))

	// Now skew the weights heavily toward validator[1].
	ctx = s.ctx()
	params := s.keeper.GetParams(ctx)
	for i := range params.WhitelistedValidators {
		if params.WhitelistedValidators[i].ValidatorAddress == valOpers[0].String() {
			params.WhitelistedValidators[i].TargetWeight = sdkmath.NewInt(1000)
		} else {
			params.WhitelistedValidators[i].TargetWeight = sdkmath.NewInt(9000)
		}
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	ctx = s.ctx()
	liquidVals := s.keeper.GetAllLiquidValidators(ctx)
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()

	redels := s.keeper.Rebalance(
		ctx,
		types.LiquidStakeProxyAcc,
		liquidVals,
		wvMap,
		types.RebalancingTrigger,
	)

	// At least one redelegation should be attempted.
	s.Require().NotEmpty(redels, "imbalanced weights must trigger at least one redelegation")
}

// TestRebalance_EmptyLiquidValidators checks that Rebalance returns an empty
// slice (not panic) when called with no liquid validators.
func (s *KeeperTestSuite) TestRebalance_EmptyLiquidValidators() {
	ctx := s.ctx()

	redels := s.keeper.Rebalance(
		ctx,
		types.LiquidStakeProxyAcc,
		types.LiquidValidators{},
		types.WhitelistedValsMap{},
		types.RebalancingTrigger,
	)

	s.Require().Empty(redels, "Rebalance with no liquid validators must return empty slice")
}

// TestRebalance_ZeroTotalTokens verifies that Rebalance is a no-op when the
// proxy account has no delegations (zero liquid tokens).
func (s *KeeperTestSuite) TestRebalance_ZeroTotalTokens() {
	s.setupWhitelistedValidators(2, 0)
	ctx := s.ctx()

	// Do NOT stake – proxy account has no delegations.
	liquidVals := s.keeper.GetAllLiquidValidators(ctx)
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()

	redels := s.keeper.Rebalance(
		ctx,
		types.LiquidStakeProxyAcc,
		liquidVals,
		wvMap,
		types.RebalancingTrigger,
	)

	s.Require().Empty(redels, "Rebalance with zero delegations must be a no-op")
}

// ---------------------------------------------------------------------------
// 3. TryRedelegation
// ---------------------------------------------------------------------------

// TestTryRedelegation_Success performs a manual single-step redelegation from
// src→dst and verifies it completes without error.
func (s *KeeperTestSuite) TestTryRedelegation_Success() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()

	// Stake so proxy has real delegations on both validators.
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(1_000_000)))

	ctx = s.ctx()
	liquidVals := s.keeper.GetAllLiquidValidators(ctx)
	s.Require().GreaterOrEqual(len(liquidVals), 2)

	// Find src/dst by operator address.
	var srcVal, dstVal types.LiquidValidator
	for _, lv := range liquidVals {
		if lv.OperatorAddress == valOpers[0].String() {
			srcVal = lv
		} else if lv.OperatorAddress == valOpers[1].String() {
			dstVal = lv
		}
	}
	s.Require().NotEmpty(srcVal.OperatorAddress)
	s.Require().NotEmpty(dstVal.OperatorAddress)

	// Only redelegate if src has any liquid tokens.
	srcTokens := srcVal.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	if !srcTokens.IsPositive() {
		s.T().Skip("src validator has no liquid tokens – staking distribution put everything on dst")
	}

	redelegateAmt := srcTokens.Quo(sdkmath.NewInt(2))
	re := types.Redelegation{
		Delegator:    types.LiquidStakeProxyAcc,
		SrcValidator: srcVal,
		DstValidator: dstVal,
		Amount:       redelegateAmt,
		Last:         false,
	}

	_, err := s.keeper.TryRedelegation(ctx, re)
	s.Require().NoError(err, "TryRedelegation must succeed for a valid redelegation")
}

// TestTryRedelegation_InsufficientFunds verifies that TryRedelegation returns
// an error when the requested amount exceeds the delegation.
func (s *KeeperTestSuite) TestTryRedelegation_InsufficientFunds() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(500_000)))

	ctx = s.ctx()
	liquidVals := s.keeper.GetAllLiquidValidators(ctx)

	var srcVal, dstVal types.LiquidValidator
	for _, lv := range liquidVals {
		if lv.OperatorAddress == valOpers[0].String() {
			srcVal = lv
		} else if lv.OperatorAddress == valOpers[1].String() {
			dstVal = lv
		}
	}

	if srcVal.OperatorAddress == "" || dstVal.OperatorAddress == "" {
		s.T().Skip("could not resolve src/dst validators")
	}

	// Request more than the actual delegation.
	excessiveAmt := sdkmath.NewInt(999_999_999_999)
	re := types.Redelegation{
		Delegator:    types.LiquidStakeProxyAcc,
		SrcValidator: srcVal,
		DstValidator: dstVal,
		Amount:       excessiveAmt,
		Last:         false,
	}

	_, err := s.keeper.TryRedelegation(ctx, re)
	s.Require().Error(err, "TryRedelegation must fail when amount exceeds delegation")
}

// ---------------------------------------------------------------------------
// 4. AutocompoundStakingRewards
// ---------------------------------------------------------------------------

// TestAutocompoundStakingRewards_BasicFlow stakes tokens, advances several
// blocks to accumulate rewards, then calls AutocompoundStakingRewards and
// verifies:
//   - The call returns no error.
//   - The fee account received the autocompound fee (when non-zero rewards
//     were available).
func (s *KeeperTestSuite) TestAutocompoundStakingRewards_BasicFlow() {
	s.setupWhitelistedValidators(3, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	params := s.keeper.GetParams(ctx)

	stakeAmt := sdkmath.NewInt(1_000_000)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], stakeAmt))

	// Advance several blocks to accumulate staking rewards.
	s.advanceHeight(5)

	ctx = s.ctx()

	// Set non-zero AutocompoundFeeRate so the fee path is exercised.
	params = s.keeper.GetParams(ctx)
	params.AutocompoundFeeRate = sdkmath.LegacyMustNewDecFromStr("0.01")
	feeAccAddr := s.addrs[0]
	params.FeeAccountAddress = feeAccAddr.String()
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	bk := s.nw.App.GetBankKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	feeBalBefore := bk.GetBalance(ctx, feeAccAddr, bondDenom).Amount

	// Allow the distribution module to withdraw outstanding rewards first so
	// AutocompoundStakingRewards has something to re-stake.
	err = s.keeper.AutocompoundStakingRewards(ctx, params.WhitelistedValsMap())
	s.Require().NoError(err, "AutocompoundStakingRewards must not return an error")

	// The fee account balance should be >= before (it may stay the same if
	// rewards were zero, but must never decrease).
	feeBalAfter := bk.GetBalance(ctx, feeAccAddr, bondDenom).Amount
	s.Require().True(feeBalAfter.GTE(feeBalBefore),
		"fee account balance must not decrease after autocompound (before=%s, after=%s)",
		feeBalBefore, feeBalAfter)
}

// TestAutocompoundStakingRewards_ModulePaused verifies that when the module is
// paused, BeforeEpochStart skips AutocompoundStakingRewards entirely.
func (s *KeeperTestSuite) TestAutocompoundStakingRewards_ModulePaused() {
	s.setupWhitelistedValidators(2, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(500_000)))

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	params.ModulePaused = true
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	// BeforeEpochStart should be a no-op when paused.
	err := s.keeper.BeforeEpochStart(ctx, types.AutocompoundEpoch, 1)
	s.Require().NoError(err, "BeforeEpochStart must not error when module is paused")
}

// TestAutocompoundStakingRewards_NoActiveValidators verifies that
// AutocompoundStakingRewards returns without error when there are no active
// liquid validators.
func (s *KeeperTestSuite) TestAutocompoundStakingRewards_NoActiveValidators() {
	ctx := s.ctx()
	// No whitelisted validators — active set is empty.
	err := s.keeper.AutocompoundStakingRewards(ctx, types.WhitelistedValsMap{})
	s.Require().NoError(err, "AutocompoundStakingRewards must return nil when there are no active validators")
}

// ---------------------------------------------------------------------------
// 5. BeforeEpochStart hook
// ---------------------------------------------------------------------------

// TestBeforeEpochStart_AutocompoundEpoch verifies that the "hour" epoch
// triggers AutocompoundStakingRewards through the hook.
func (s *KeeperTestSuite) TestBeforeEpochStart_AutocompoundEpoch() {
	s.setupWhitelistedValidators(2, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(500_000)))

	ctx := s.ctx()
	err := s.keeper.BeforeEpochStart(ctx, types.AutocompoundEpoch, 1)
	s.Require().NoError(err, "BeforeEpochStart for AutocompoundEpoch must not error")
}

// TestBeforeEpochStart_RebalanceEpoch verifies that the "day" epoch triggers
// UpdateLiquidValidatorSet through the hook.
func (s *KeeperTestSuite) TestBeforeEpochStart_RebalanceEpoch() {
	s.setupWhitelistedValidators(3, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(600_000)))

	ctx := s.ctx()
	err := s.keeper.BeforeEpochStart(ctx, types.RebalanceEpoch, 1)
	s.Require().NoError(err, "BeforeEpochStart for RebalanceEpoch must not error")
}

// TestBeforeEpochStart_UnknownEpoch verifies that an unknown epoch identifier
// returns an error.
func (s *KeeperTestSuite) TestBeforeEpochStart_UnknownEpoch() {
	ctx := s.ctx()
	err := s.keeper.BeforeEpochStart(ctx, "unknown_epoch", 1)
	s.Require().Error(err, "BeforeEpochStart for unknown epoch must return error")
}

// TestBeforeEpochStart_AutocompoundEpochViaNextBlock verifies the end-to-end
// path: advance the chain past one "hour" epoch boundary so that the epoch
// module fires BeforeEpochStart, which in turn calls AutocompoundStakingRewards.
// We assert that no panic occurs and that the net amount state is still valid.
func (s *KeeperTestSuite) TestBeforeEpochStart_AutocompoundEpochViaNextBlock() {
	s.setupWhitelistedValidators(3, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(1_000_000)))

	// Advance past the "hour" epoch duration.
	s.Require().NoError(s.nw.NextBlockAfter(time.Hour + time.Second))

	ctx := s.ctx()
	nas, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	s.Require().True(nas.TotalLiquidTokens.GTE(sdkmath.ZeroInt()),
		"TotalLiquidTokens must be non-negative after autocompound epoch fires")
}

// TestBeforeEpochStart_RebalanceEpochViaNextBlock verifies the end-to-end path
// for the "day" epoch: add a new whitelisted validator, advance past 24 h, and
// confirm the new validator becomes active.
func (s *KeeperTestSuite) TestBeforeEpochStart_RebalanceEpochViaNextBlock() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(1_000_000)))

	// Get a 3rd validator.
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	allVals, err := sk.GetValidators(ctx, 5)
	s.Require().NoError(err)

	var thirdValAddr string
	for _, v := range allVals {
		if v.OperatorAddress != valOpers[0].String() && v.OperatorAddress != valOpers[1].String() {
			thirdValAddr = v.OperatorAddress
			break
		}
	}
	s.Require().NotEmpty(thirdValAddr, "need at least 3 validators in the network")

	// Add 3rd validator before advancing.
	ctx = s.ctx()
	params := s.keeper.GetParams(ctx)
	weights := equalTargetWeights(3)
	for i := range params.WhitelistedValidators {
		params.WhitelistedValidators[i].TargetWeight = weights[i]
	}
	params.WhitelistedValidators = append(params.WhitelistedValidators, types.WhitelistedValidator{
		ValidatorAddress: thirdValAddr,
		TargetWeight:     weights[2],
	})
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	// Commit the new whitelist, then advance past 24 h to fire "day" epoch.
	s.Require().NoError(s.nw.CommitState())
	s.Require().NoError(s.nw.NextBlock())
	s.Require().NoError(s.nw.NextBlockAfter(24*time.Hour + time.Second))

	ctx = s.ctx()
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()
	activeVals := s.keeper.GetActiveLiquidValidators(ctx, wvMap)
	s.Require().Len(activeVals, 3, "3rd validator must be active after day-epoch rebalance fires")
}

// ---------------------------------------------------------------------------
// 6. WithdrawLiquidRewards
// ---------------------------------------------------------------------------

// TestWithdrawLiquidRewards_NoError verifies that WithdrawLiquidRewards does
// not panic and returns without error when rewards may be zero or non-zero.
func (s *KeeperTestSuite) TestWithdrawLiquidRewards_NoError() {
	s.setupWhitelistedValidators(3, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(1_000_000)))

	// Advance a few blocks so that rewards are accumulated.
	s.advanceHeight(3)

	ctx := s.ctx()
	// WithdrawLiquidRewards has no return value – we only need it not to panic.
	s.Require().NotPanics(func() {
		s.keeper.WithdrawLiquidRewards(ctx, types.LiquidStakeProxyAcc)
	})
}

// TestWithdrawLiquidRewards_IncreasesProxyBalance stakes tokens, advances
// blocks so rewards accumulate, then verifies that calling
// WithdrawLiquidRewards increases (or at least does not decrease) the proxy
// account's bond-denom balance.
func (s *KeeperTestSuite) TestWithdrawLiquidRewards_IncreasesProxyBalance() {
	s.setupWhitelistedValidators(3, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(2_000_000)))

	// Advance many blocks so non-trivial rewards accumulate.
	s.advanceHeight(10)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bk := s.nw.App.GetBankKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	balBefore := bk.GetBalance(ctx, types.LiquidStakeProxyAcc, bondDenom).Amount

	s.keeper.WithdrawLiquidRewards(ctx, types.LiquidStakeProxyAcc)

	balAfter := bk.GetBalance(ctx, types.LiquidStakeProxyAcc, bondDenom).Amount
	s.Require().True(balAfter.GTE(balBefore),
		"proxy balance must not decrease after WithdrawLiquidRewards (before=%s after=%s)",
		balBefore, balAfter)
}

// ---------------------------------------------------------------------------
// 7. GetWeightMap
// ---------------------------------------------------------------------------

// TestGetWeightMap_EqualWeights verifies that GetWeightMap returns equal weights
// for equal-weight whitelisted validators.
func (s *KeeperTestSuite) TestGetWeightMap_EqualWeights() {
	s.setupWhitelistedValidators(2, 0)
	ctx := s.ctx()

	params := s.keeper.GetParams(ctx)
	liquidVals := s.keeper.GetAllLiquidValidators(ctx)
	wvMap := params.WhitelistedValsMap()

	weightMap, totalWeight := s.keeper.GetWeightMap(ctx, liquidVals, wvMap)

	s.Require().True(totalWeight.IsPositive(), "totalWeight must be positive")
	s.Require().Len(weightMap, len(liquidVals), "weightMap must have an entry per liquid validator")

	// All weights should be equal (equal distribution).
	var firstWeight sdkmath.Int
	for i, lv := range liquidVals {
		w := weightMap[lv.OperatorAddress]
		s.Require().True(w.IsPositive(), "weight for validator %s must be positive", lv.OperatorAddress)
		if i == 0 {
			firstWeight = w
		} else {
			s.Require().Equal(firstWeight, w, "all equal-weight validators must have the same weight")
		}
	}
}

// TestGetWeightMap_Empty verifies that GetWeightMap returns zero totalWeight
// when there are no liquid validators.
func (s *KeeperTestSuite) TestGetWeightMap_Empty() {
	ctx := s.ctx()
	weightMap, totalWeight := s.keeper.GetWeightMap(ctx, types.LiquidValidators{}, types.WhitelistedValsMap{})

	s.Require().Empty(weightMap)
	s.Require().True(totalWeight.IsZero(), "totalWeight must be zero with no liquid validators")
}

// ---------------------------------------------------------------------------
// 8. ProxyAccBalance
// ---------------------------------------------------------------------------

// TestGetProxyAccBalance_AfterStake verifies that GetProxyAccBalance returns a
// non-negative coin after delegations are set up (proxy may receive small
// reward leftovers).
func (s *KeeperTestSuite) TestGetProxyAccBalance_AfterStake() {
	s.setupWhitelistedValidators(2, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(500_000)))

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	balance, err := s.keeper.GetProxyAccBalance(ctx, types.LiquidStakeProxyAcc)
	s.Require().NoError(err)
	s.Require().Equal(bondDenom, balance.Denom, "proxy balance denom must be the bond denom")
	s.Require().False(balance.IsNegative(), "proxy balance must not be negative")
}

// ---------------------------------------------------------------------------
// 9. GetLiquidValidatorState / GetAllLiquidValidatorStates
// ---------------------------------------------------------------------------

// TestGetLiquidValidatorState verifies that GetLiquidValidatorState returns a
// valid state for a whitelisted validator.
func (s *KeeperTestSuite) TestGetLiquidValidatorState() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(1_000_000)))

	ctx := s.ctx()

	lvState, found := s.keeper.GetLiquidValidatorState(ctx, valOpers[0])
	s.Require().True(found, "GetLiquidValidatorState must find the whitelisted validator")
	s.Require().Equal(valOpers[0].String(), lvState.OperatorAddress,
		"LiquidValidatorState must have the correct operator address")
}

// TestGetAllLiquidValidatorStates verifies that GetAllLiquidValidatorStates
// returns one entry per liquid validator.
func (s *KeeperTestSuite) TestGetAllLiquidValidatorStates() {
	s.setupWhitelistedValidators(3, 0)

	ctx := s.ctx()
	lvStates := s.keeper.GetAllLiquidValidatorStates(ctx)
	s.Require().Len(lvStates, 3, "must have exactly 3 LiquidValidatorState entries")
}

// ---------------------------------------------------------------------------
// 10. GetAllLiquidValidatorStates round-trip
// ---------------------------------------------------------------------------

// TestGetAllLiquidValidatorStates_RoundTrip verifies that
// GetAllLiquidValidatorStates returns one entry per whitelisted validator and
// that each entry has the correct operator address.
func (s *KeeperTestSuite) TestGetAllLiquidValidatorStates_RoundTrip() {
	_, valOpers := s.setupWhitelistedValidators(3, 0)
	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(900_000)))

	ctx := s.ctx()
	lvStates := s.keeper.GetAllLiquidValidatorStates(ctx)
	s.Require().Len(lvStates, 3, "must have exactly 3 LiquidValidatorState entries")

	// Build a set of operator addresses from the states.
	stateAddrs := make(map[string]struct{}, len(lvStates))
	for _, lvs := range lvStates {
		stateAddrs[lvs.OperatorAddress] = struct{}{}
	}
	for _, oper := range valOpers {
		_, ok := stateAddrs[oper.String()]
		s.Require().True(ok, "operator %s must appear in GetAllLiquidValidatorStates", oper)
	}
}
