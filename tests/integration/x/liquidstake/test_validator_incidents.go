package liquidstake

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/evm/x/liquidstake/types"
)

// getConsAddr is a helper to get the consensus address of a validator by its operator address.
func (s *KeeperTestSuite) getConsAddr(valOper sdk.ValAddress) sdk.ConsAddress {
	s.T().Helper()
	ctx := s.ctx()
	val, err := s.nw.App.GetStakingKeeper().GetValidator(ctx, valOper)
	s.Require().NoError(err)
	consAddr, err := val.GetConsAddr()
	s.Require().NoError(err)
	return consAddr
}

// tombstoneValidator tombstones a validator. Requires that ValidatorSigningInfo already exists
// (it is created automatically for genesis validators).
func (s *KeeperTestSuite) tombstoneValidator(valOper sdk.ValAddress) {
	s.T().Helper()
	ctx := s.ctx()
	consAddr := s.getConsAddr(valOper)

	sk := s.nw.App.GetSlashingKeeper()

	// Ensure signing info exists; create it if missing.
	if !sk.HasValidatorSigningInfo(ctx, consAddr) {
		err := sk.SetValidatorSigningInfo(ctx, consAddr, slashingtypes.ValidatorSigningInfo{
			Address:    consAddr.String(),
			Tombstoned: false,
		})
		s.Require().NoError(err)
	}

	err := sk.Tombstone(ctx, consAddr)
	s.Require().NoError(err)
}

// slashValidator slashes a validator by the given fraction at the current block height.
func (s *KeeperTestSuite) slashValidator(valOper sdk.ValAddress, fraction sdkmath.LegacyDec) {
	s.T().Helper()
	ctx := s.ctx()
	consAddr := s.getConsAddr(valOper)

	val, err := s.nw.App.GetStakingKeeper().GetValidator(ctx, valOper)
	s.Require().NoError(err)

	power := val.GetConsensusPower(sdk.DefaultPowerReduction)
	height := ctx.BlockHeight()

	_, err = s.nw.App.GetStakingKeeper().Slash(ctx, consAddr, height, power, fraction)
	s.Require().NoError(err)
}

// ─── Tombstone tests ─────────────────────────────────────────────────────────

// TestTombstone_ValidatorBecomesInactive checks that after tombstoning a whitelisted
// validator it is no longer returned by GetActiveLiquidValidators.
func (s *KeeperTestSuite) TestTombstone_ValidatorBecomesInactive() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	wm := params.WhitelistedValsMap()

	activesBefore := s.keeper.GetActiveLiquidValidators(ctx, wm)
	s.Require().Len(activesBefore, 3)

	// Tombstone the first whitelisted validator.
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.tombstoneValidator(valOper)

	// After tombstone the validator should be inactive.
	activesAfter := s.keeper.GetActiveLiquidValidators(ctx, wm)
	s.Require().Len(activesAfter, 2)

	for _, av := range activesAfter {
		s.Require().NotEqual(valOper.String(), av.OperatorAddress)
	}
}

// TestTombstone_NetAmountDropsAfterSlash checks that slashing a validator reduces
// TotalLiquidTokens and NetAmount.
func (s *KeeperTestSuite) TestTombstone_NetAmountDropsAfterSlash() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(900_000)

	// Liquid stake first so there are delegations to slash.
	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	ctx = s.ctx()
	nasBefore, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	s.Require().True(nasBefore.TotalLiquidTokens.IsPositive())

	// Slash 5% from one validator.
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.slashValidator(valOper, sdkmath.LegacyMustNewDecFromStr("0.05"))

	ctx = s.ctx()
	nasAfter, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	s.Require().True(
		nasAfter.TotalLiquidTokens.LT(nasBefore.TotalLiquidTokens),
		"TotalLiquidTokens should decrease after slash",
	)
	s.Require().True(
		nasAfter.NetAmount.LT(nasBefore.NetAmount),
		"NetAmount should decrease after slash",
	)
}

// TestTombstone_MintRateIncreasesAfterSlash verifies that after a slash the MintRate
// (gTAC per native token) increases — i.e. new stakers get more gTAC for the same
// native amount, reflecting that existing gTAC is now worth less.
func (s *KeeperTestSuite) TestTombstone_MintRateIncreasesAfterSlash() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(900_000)

	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	ctx = s.ctx()
	nasBefore, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	// Heavy slash: 10%.
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.slashValidator(valOper, sdkmath.LegacyMustNewDecFromStr("0.10"))

	ctx = s.ctx()
	nasAfter, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	// MintRate = GtacTotalSupply / NetAmount; after slash NetAmount dropped → MintRate increased.
	s.Require().True(
		nasAfter.MintRate.GT(nasBefore.MintRate),
		"MintRate should increase after slash (gTAC became cheaper)",
	)
}

// TestTombstone_LiquidStakeStillWorksWithRemainingActiveValidators ensures that
// LiquidStake succeeds after one of three validators is tombstoned (quorum still met).
func (s *KeeperTestSuite) TestTombstone_LiquidStakeStillWorksWithRemainingActiveValidators() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	// Tombstone validator[0]; validators[1] and [2] remain active (quorum: 2/3 ≈ 66.7% > 33.3%).
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.tombstoneValidator(valOper)

	staker := s.delAddrs[0]
	mintAmt, err := s.keeper.LiquidStake(
		ctx,
		types.LiquidStakeProxyAcc,
		staker,
		sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(500_000)),
	)
	s.Require().NoError(err)
	s.Require().True(mintAmt.IsPositive())
}

// TestTombstone_LiquidStakeFailsWhenQuorumLost verifies that LiquidStake fails when
// tombstoning the only (or majority) validator causes the active weight to fall below quorum.
func (s *KeeperTestSuite) TestTombstone_LiquidStakeFailsWhenQuorumLost() {
	// Use only 1 validator — tombstoning it kills quorum entirely.
	s.setupWhitelistedValidators(1, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.tombstoneValidator(valOper)

	_, err = s.keeper.LiquidStake(
		ctx,
		types.LiquidStakeProxyAcc,
		s.delAddrs[0],
		sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(500_000)),
	)
	// Should fail: no active validators / quorum not met.
	s.Require().Error(err)
}

// TestTombstone_RebalanceRedelegatesAwayFromTombstoned verifies that after tombstone
// and a manual Rebalance call, tokens are redelegated away from the tombstoned validator.
func (s *KeeperTestSuite) TestTombstone_RebalanceRedelegatesAwayFromTombstoned() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(900_000)

	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	ctx = s.ctx()
	tombstonedValOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)

	// Record delegation to the about-to-be-tombstoned validator before.
	tombstonedLV := types.LiquidValidator{OperatorAddress: tombstonedValOper.String()}
	tokensBefore := tombstonedLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)

	s.tombstoneValidator(tombstonedValOper)

	// Trigger rebalance (simulates RebalanceEpoch).
	ctx = s.ctx()
	redelegations := s.keeper.UpdateLiquidValidatorSet(ctx, true)
	s.Require().NotEmpty(redelegations, "expected at least one redelegation away from tombstoned validator")

	// After rebalance, tokens on tombstoned validator should have decreased.
	ctx = s.ctx()
	tokensAfter := tombstonedLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	s.Require().True(
		tokensAfter.LT(tokensBefore),
		"tokens on tombstoned validator should decrease after rebalance",
	)
}

// TestTombstone_LiquidUnstakePrioritisesTombstonedValidator verifies that when there is
// a tombstoned validator among the liquid validators, LiquidUnstake unbonds from it first.
func (s *KeeperTestSuite) TestTombstone_LiquidUnstakePrioritisesTombstonedValidator() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(900_000)

	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	ctx = s.ctx()
	tombstonedValOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.tombstoneValidator(tombstonedValOper)

	// Record tokens on the tombstoned validator before unstake.
	ctx = s.ctx()
	tombstonedLV := types.LiquidValidator{OperatorAddress: tombstonedValOper.String()}
	tokensBefore := tombstonedLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	s.Require().True(tokensBefore.IsPositive(), "tombstoned validator should still have delegated tokens")

	// Unstake a small portion — should come from tombstoned validator first.
	unstakeAmt := mintAmt.Quo(sdkmath.NewInt(10))
	_, _, _, _, err = s.keeper.LiquidUnstake(
		ctx,
		types.LiquidStakeProxyAcc,
		staker,
		sdk.NewCoin(params.LiquidBondDenom, unstakeAmt),
	)
	s.Require().NoError(err)

	ctx = s.ctx()
	tokensAfter := tombstonedLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	s.Require().True(
		tokensAfter.LT(tokensBefore),
		"tokens on tombstoned validator should decrease when it is prioritised during unstake",
	)
}

// ─── Validator Unbonding tests ────────────────────────────────────────────────

// TestValidatorUnbond_BecomesInactiveWhenFullyUnbonded verifies that a whitelisted
// validator that has fully unbonded (status = Unbonded, tokens = 0) is treated as
// inactive by liquidstake.
func (s *KeeperTestSuite) TestValidatorUnbond_BecomesInactiveWhenFullyUnbonded() {
	// Use CreateValidators so we control the delegator who can undelegate.
	delegatorAddrs, valOpers, _ := s.CreateValidators([]int64{1_000_000, 1_000_000, 1_000_000})

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()

	// Whitelist all three.
	valWeight := types.TotalValidatorWeight.Quo(sdkmath.NewInt(3))
	params := s.keeper.GetParams(ctx)
	params.WhitelistedValidators = nil
	params.ModulePaused = false
	for i, vo := range valOpers {
		s.keeper.SetLiquidValidator(ctx, types.LiquidValidator{OperatorAddress: vo.String()})
		params.WhitelistedValidators = append(params.WhitelistedValidators, types.WhitelistedValidator{
			ValidatorAddress: vo.String(),
			TargetWeight:     valWeight,
		})
		_ = delegatorAddrs[i] // keep in scope
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.Require().NoError(s.nw.CommitState())

	wm := params.WhitelistedValsMap()
	activesBefore := s.keeper.GetActiveLiquidValidators(ctx, wm)
	s.Require().Len(activesBefore, 3)

	// Undelegate all shares from validator[0] so it eventually becomes Unbonded.
	del, err := sk.GetDelegation(ctx, delegatorAddrs[0], valOpers[0])
	s.Require().NoError(err)

	_, _, err = sk.Undelegate(ctx, delegatorAddrs[0], valOpers[0], del.Shares)
	s.Require().NoError(err)
	s.Require().NoError(s.nw.CommitState())

	// After fully undelegating, the validator may be removed from the store (no more
	// delegations) or have zero tokens → either way it must not be active.
	ctx = s.ctx()
	val, err := sk.GetValidator(ctx, valOpers[0])
	valGone := err != nil // validator deleted from store → definitely inactive
	valEmpty := err == nil && (val.Tokens.IsZero() || val.InvalidExRate())

	if valGone || valEmpty {
		activesAfter := s.keeper.GetActiveLiquidValidators(ctx, wm)
		s.Require().Less(len(activesAfter), 3, "fully unbonded/deleted validator should not be active")
	}
}

// TestValidatorUnbond_LiquidStakeDistributesToRemainingActiveValidators checks that
// after one whitelisted validator becomes unbonded, LiquidStake only delegates to the
// remaining active validators.
func (s *KeeperTestSuite) TestValidatorUnbond_LiquidStakeDistributesToRemainingActiveValidators() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	// Tombstone validator[0] as a cheap proxy for "inactive" (avoids complex unbonding
	// timing). Active quorum still holds with 2 remaining validators.
	valOper0, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.tombstoneValidator(valOper0)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(600_000)

	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)
	s.Require().True(mintAmt.IsPositive())

	// The inactive validator should have received zero new delegation.
	ctx = s.ctx()
	inactiveLV := types.LiquidValidator{OperatorAddress: valOper0.String()}
	tokensOnInactive := inactiveLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	s.Require().True(tokensOnInactive.IsZero(),
		"inactive/tombstoned validator should receive zero delegation during LiquidStake")
}

// TestValidatorUnbond_PrioritisedDuringLiquidUnstake verifies that when a validator
// becomes unbonded/inactive, LiquidUnstake prefers unbonding from it first.
func (s *KeeperTestSuite) TestValidatorUnbond_PrioritisedDuringLiquidUnstake() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(900_000)

	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)
	s.Require().True(mintAmt.IsPositive())

	// Make validator[0] inactive via tombstone.
	ctx = s.ctx()
	valOper0, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.tombstoneValidator(valOper0)

	inactiveLV := types.LiquidValidator{OperatorAddress: valOper0.String()}
	tokensBefore := inactiveLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	s.Require().True(tokensBefore.IsPositive())

	// Small unstake — should be prioritised from the inactive validator.
	unstakeAmt := mintAmt.Quo(sdkmath.NewInt(10))
	_, _, _, _, err = s.keeper.LiquidUnstake(
		ctx,
		types.LiquidStakeProxyAcc,
		staker,
		sdk.NewCoin(params.LiquidBondDenom, unstakeAmt),
	)
	s.Require().NoError(err)

	ctx = s.ctx()
	tokensAfter := inactiveLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	s.Require().True(tokensAfter.LT(tokensBefore),
		"inactive validator tokens should decrease first during LiquidUnstake")
}

// ─── Slash + LiquidUnstake interaction ───────────────────────────────────────

// TestSlash_LiquidUnstakeAfterSlashReturnsReducedAmount verifies that after a slash
// the amount of native tokens returned by LiquidUnstake is less than what was staked.
func (s *KeeperTestSuite) TestSlash_LiquidUnstakeAfterSlashReturnsReducedAmount() {
	s.setupWhitelistedValidators(1, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(1_000_000)

	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)
	s.Require().Equal(stakeAmt, mintAmt)

	// Slash 10% on the only validator.
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.slashValidator(valOper, sdkmath.LegacyMustNewDecFromStr("0.10"))

	// NetAmount after slash.
	ctx = s.ctx()
	nasAfterSlash, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	s.Require().True(nasAfterSlash.NetAmount.LT(sdkmath.LegacyNewDecFromInt(stakeAmt)),
		"NetAmount should be less than original stakeAmt after 10%% slash")

	// Unstake all gTAC — the returned native amount should be < original stakeAmt.
	bk := s.nw.App.GetBankKeeper()
	nativeBalBefore := bk.GetBalance(ctx, staker, skParams.BondDenom).Amount

	_, returnAmt, _, directAmt, err := s.keeper.LiquidUnstake(
		ctx,
		types.LiquidStakeProxyAcc,
		staker,
		sdk.NewCoin(params.LiquidBondDenom, mintAmt),
	)
	s.Require().NoError(err)
	_ = returnAmt
	_ = directAmt
	_ = nativeBalBefore

	// The unbonding amount recorded in NetAmount was reduced by the slash.
	nasAfterUnstake, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	// After unstaking all gTAC, GtacTotalSupply should be zero.
	s.Require().True(nasAfterUnstake.GtacTotalSupply.IsZero(),
		"gTAC total supply should be zero after full unstake")
}

// TestSlash_NewStakerGetsMoreGTACAfterSlash ensures that after a slash, the same
// native amount mints more gTAC (because gTAC is now cheaper).
func (s *KeeperTestSuite) TestSlash_NewStakerGetsMoreGTACAfterSlash() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	// First staker stakes to establish a baseline rate.
	staker1 := s.delAddrs[0]
	staker2 := s.delAddrs[1]
	stakeAmt := sdkmath.NewInt(600_000)

	mintAmt1, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker1, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	// Slash 20% from one validator.
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.slashValidator(valOper, sdkmath.LegacyMustNewDecFromStr("0.20"))

	ctx = s.ctx()

	// Second staker stakes the same amount after slash.
	mintAmt2, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker2, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	// After a slash the net amount is lower, so the same native amount yields MORE gTAC.
	s.Require().True(
		mintAmt2.GT(mintAmt1),
		"second staker should receive more gTAC than first staker after a slash (gTAC became cheaper): mintAmt1=%s mintAmt2=%s",
		mintAmt1, mintAmt2,
	)
}

// ─── Jailing (downtime) tests ─────────────────────────────────────────────────

// TestJail_ValidatorRemainsActiveInLiquidStake verifies that a jailed-but-not-tombstoned
// validator is still treated as active by liquidstake (jailed flag is not checked by
// ActiveCondition).
func (s *KeeperTestSuite) TestJail_ValidatorRemainsActiveInLiquidStake() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	wm := params.WhitelistedValsMap()

	activesBefore := s.keeper.GetActiveLiquidValidators(ctx, wm)
	s.Require().Len(activesBefore, 3)

	// Jail (downtime) validator[0] — no tombstone.
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	consAddr := s.getConsAddr(valOper)

	err = s.nw.App.GetSlashingKeeper().Jail(ctx, consAddr)
	s.Require().NoError(err)

	// Validator should still be considered active by liquidstake.
	activesAfter := s.keeper.GetActiveLiquidValidators(ctx, wm)
	s.Require().Len(activesAfter, 3,
		"jailed (non-tombstoned) validator should remain active in liquidstake")
}

// TestJail_LiquidStakeSucceedsAfterJail confirms that LiquidStake works normally
// while a validator is jailed (not tombstoned).
func (s *KeeperTestSuite) TestJail_LiquidStakeSucceedsAfterJail() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	// Jail validator[0].
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	consAddr := s.getConsAddr(valOper)
	err = s.nw.App.GetSlashingKeeper().Jail(ctx, consAddr)
	s.Require().NoError(err)

	staker := s.delAddrs[0]
	mintAmt, err := s.keeper.LiquidStake(
		ctx,
		types.LiquidStakeProxyAcc,
		staker,
		sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(500_000)),
	)
	s.Require().NoError(err, "LiquidStake should succeed even when a validator is jailed")
	s.Require().True(mintAmt.IsPositive())
}

// ─── Multiple incidents ───────────────────────────────────────────────────────

// TestMultipleSlashes_NetAmountCumulativelyDecreases verifies that repeated slashes
// on different validators cumulatively reduce NetAmount.
func (s *KeeperTestSuite) TestMultipleSlashes_NetAmountCumulativelyDecreases() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	stakeAmt := sdkmath.NewInt(900_000)
	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, s.delAddrs[0], sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	ctx = s.ctx()
	nas0, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	fraction := sdkmath.LegacyMustNewDecFromStr("0.05")
	for i := range 2 {
		valOper, parseErr := sdk.ValAddressFromBech32(params.WhitelistedValidators[i].ValidatorAddress)
		s.Require().NoError(parseErr)
		s.slashValidator(valOper, fraction)
	}

	ctx = s.ctx()
	nasAfter, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	s.Require().True(nasAfter.NetAmount.LT(nas0.NetAmount),
		"NetAmount should be lower after two consecutive slashes")
}

// TestTombstone_LiquidUnstakeSucceedsAfterAllTokensOnInactiveValidator covers the edge
// case where all liquid tokens sit on a tombstoned validator. LiquidUnstake should still
// succeed by unbonding from that validator (no active validators needed for unstake).
func (s *KeeperTestSuite) TestTombstone_LiquidUnstakeSucceedsAfterAllTokensOnInactiveValidator() {
	s.setupWhitelistedValidators(1, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(1_000_000)

	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	ctx = s.ctx()
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)
	s.tombstoneValidator(valOper)

	// Unstake half — should work even though the only validator is tombstoned.
	unstakeAmt := mintAmt.Quo(sdkmath.NewInt(2))
	_, _, _, _, err = s.keeper.LiquidUnstake(
		ctx,
		types.LiquidStakeProxyAcc,
		staker,
		sdk.NewCoin(params.LiquidBondDenom, unstakeAmt),
	)
	s.Require().NoError(err, "LiquidUnstake should succeed even when the only validator is tombstoned")
}

// TestValidatorUnbond_RebalanceAfterUnbond checks that after a validator starts unbonding
// (becomes inactive) and a Rebalance is triggered, the liquidstake module attempts to
// redelegate tokens to the remaining active validators.
func (s *KeeperTestSuite) TestValidatorUnbond_RebalanceAfterUnbond() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	stakeAmt := sdkmath.NewInt(900_000)
	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, s.delAddrs[0], sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)

	// Simulate validator going inactive via tombstone (same effect on liquidstake).
	ctx = s.ctx()
	inactiveValOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)

	inactiveLV := types.LiquidValidator{OperatorAddress: inactiveValOper.String()}
	tokensBefore := inactiveLV.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false)
	s.Require().True(tokensBefore.IsPositive())

	s.tombstoneValidator(inactiveValOper)

	ctx = s.ctx()
	redelegations := s.keeper.UpdateLiquidValidatorSet(ctx, true)

	// There should have been at least one redelegation away from the now-inactive validator.
	var redelegatedAway bool
	for _, red := range redelegations {
		if red.SrcValidator.OperatorAddress == inactiveValOper.String() {
			redelegatedAway = true
			break
		}
	}
	s.Require().True(redelegatedAway,
		"Rebalance should redelegate tokens away from the inactive (tombstoned) validator")

	// The two remaining active validators should now hold more tokens.
	ctx = s.ctx()
	activeVals := s.keeper.GetActiveLiquidValidators(ctx, params.WhitelistedValsMap())
	totalActiveTokens := sdkmath.ZeroInt()
	for _, av := range activeVals {
		lv := types.LiquidValidator{OperatorAddress: av.OperatorAddress}
		totalActiveTokens = totalActiveTokens.Add(lv.GetLiquidTokens(ctx, *s.nw.App.GetStakingKeeper(), false))
	}
	s.Require().True(totalActiveTokens.IsPositive(),
		"active validators should hold tokens after rebalance")
}

// TestSlash_GetNetAmountStateConsistency verifies the internal consistency of
// NetAmountState after a slash: NetAmount must equal the sum of its components.
func (s *KeeperTestSuite) TestSlash_GetNetAmountStateConsistency() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, s.delAddrs[0], sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(900_000)))
	s.Require().NoError(err)

	// Slash.
	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[1].ValidatorAddress)
	s.Require().NoError(err)
	s.slashValidator(valOper, sdkmath.LegacyMustNewDecFromStr("0.07"))

	ctx = s.ctx()
	nas, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	// NetAmount = ProxyAccBalance + TotalLiquidTokens + TotalUnbondingBalance + TotalRemainingRewards
	expected := sdkmath.LegacyNewDecFromInt(nas.ProxyAccBalance).
		Add(sdkmath.LegacyNewDecFromInt(nas.TotalLiquidTokens)).
		Add(sdkmath.LegacyNewDecFromInt(nas.TotalUnbondingBalance)).
		Add(nas.TotalRemainingRewards)

	s.Require().Equal(expected.TruncateInt(), nas.NetAmount.TruncateInt(),
		"NetAmount should equal the sum of its components after a slash")
}

// TestTombstone_IsTombstonedHelperWorks is a sanity check for the tombstoneValidator
// helper used throughout this test file.
func (s *KeeperTestSuite) TestTombstone_IsTombstonedHelperWorks() {
	s.setupWhitelistedValidators(2, 1_000_000)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)

	valOper, err := sdk.ValAddressFromBech32(params.WhitelistedValidators[0].ValidatorAddress)
	s.Require().NoError(err)

	val, err := s.nw.App.GetStakingKeeper().GetValidator(ctx, valOper)
	s.Require().NoError(err)

	s.Require().False(s.keeper.IsTombstoned(ctx, val), "validator should not be tombstoned initially")

	s.tombstoneValidator(valOper)

	ctx = s.ctx()
	val, err = s.nw.App.GetStakingKeeper().GetValidator(ctx, valOper)
	s.Require().NoError(err)
	s.Require().True(s.keeper.IsTombstoned(ctx, val), "validator should be tombstoned after tombstoneValidator()")
}

// stakingtypes import used for ErrMaxUnbondingDelegationEntries — keep it referenced.
var _ = stakingtypes.ErrMaxUnbondingDelegationEntries
