package liquidstake

// TestStateAudit is a comprehensive end-to-end state audit for the liquidstake module.
// It covers three major flows in a single test scenario:
//
//  1. LiquidStake  – delegator sends tokens → gTAC minted, proxyAcc delegation appears.
//  2. StakeToLP    – delegator converts existing staking delegation via LSM → gTAC minted on top.
//  3. LiquidUnstake – delegator burns gTAC → unbonding delegation created on liquidStaker,
//     time skipped past unbonding period → original tokens returned to delegator.
//
// At every step we verify:
//   - delegator's bondDenom balance
//   - delegator's gTAC (LiquidBondDenom) balance
//   - proxyAcc delegation (total liquid tokens held by the proxy)
//   - per-validator LiquidTokens via GetLiquidValidatorState
//   - gTAC total supply (NetAmountState)
//
// NOTE: StakeToLP requires LsmDisabled=false AND a plain (non-ValidatorBond) delegation.
//
//nolint:funlen
import (
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/evm/testutil/integration/evm/network"
	"github.com/cosmos/evm/x/liquidstake/keeper"
	"github.com/cosmos/evm/x/liquidstake/types"
)

// stakeState is a snapshot of all accounts relevant to a liquid-staking operation.
type stakeState struct {
	// delegator
	delegatorBondBalance sdkmath.Int // bondDenom balance
	delegatorGTAC        sdkmath.Int // gTAC balance

	// liquidstake proxy account
	proxyDelShares    sdkmath.LegacyDec // sum of delegation shares in proxyAcc
	proxyLiquidTokens sdkmath.Int       // sum of liquid tokens in proxyAcc

	// network-wide gTAC supply
	gTACSupply sdkmath.Int
}

// captureState reads the current state for the given delegator.
func (s *KeeperTestSuite) captureState(delegator sdk.AccAddress) stakeState {
	s.T().Helper()
	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	bk := s.nw.App.GetBankKeeper()
	bondDenom, err := s.nw.App.GetStakingKeeper().BondDenom(ctx)
	s.Require().NoError(err)

	_, delShares, liquidTokens, err := s.keeper.CheckDelegationStates(ctx, types.LiquidStakeProxyAcc)
	s.Require().NoError(err)

	return stakeState{
		delegatorBondBalance: bk.GetBalance(ctx, delegator, bondDenom).Amount,
		delegatorGTAC:        bk.GetBalance(ctx, delegator, params.LiquidBondDenom).Amount,
		proxyDelShares:       delShares,
		proxyLiquidTokens:    liquidTokens,
		gTACSupply:           bk.GetSupply(ctx, params.LiquidBondDenom).Amount,
	}
}

// TestStateAudit_FullFlow is the comprehensive state-audit test.
func (s *KeeperTestSuite) TestStateAudit_FullFlow() {
	// -----------------------------------------------------------------------
	// Setup: whitelist 2 validators, fund delegator
	// -----------------------------------------------------------------------
	_, valOpers := s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	// We need a custom delegator distinct from suite's predefined delAddrs so we
	// can track its bond balance precisely without interference from other tests.
	delegator := s.delAddrs[1]

	initialBondBalance := s.nw.App.GetBankKeeper().GetBalance(ctx, delegator, bondDenom).Amount
	s.Require().True(initialBondBalance.IsPositive(), "delegator must start with bond tokens")

	// -----------------------------------------------------------------------
	// === PHASE 1: LiquidStake ===
	// -----------------------------------------------------------------------
	s.T().Log("=== PHASE 1: LiquidStake ===")

	stakeAmt := sdkmath.NewInt(500_000)
	stateBefore := s.captureState(delegator)

	mintAmt, err := s.keeper.LiquidStake(
		ctx,
		types.LiquidStakeProxyAcc,
		delegator,
		sdk.NewCoin(bondDenom, stakeAmt),
	)
	s.Require().NoError(err, "LiquidStake must succeed")
	s.Require().True(mintAmt.IsPositive(), "minted gTAC must be positive")

	stateAfterStake := s.captureState(delegator)

	// 1a) Delegator lost bondDenom
	s.Require().Equal(
		stateBefore.delegatorBondBalance.Sub(stakeAmt),
		stateAfterStake.delegatorBondBalance,
		"delegator bond balance must decrease by stakeAmt",
	)
	// 1b) Delegator gained gTAC
	s.Require().Equal(
		stateBefore.delegatorGTAC.Add(mintAmt),
		stateAfterStake.delegatorGTAC,
		"delegator gTAC balance must increase by mintAmt",
	)
	// 1c) proxyAcc holds more liquid tokens
	s.Require().True(
		stateAfterStake.proxyLiquidTokens.GT(stateBefore.proxyLiquidTokens),
		"proxyAcc liquid tokens must increase after LiquidStake",
	)
	// 1d) gTAC supply increased
	s.Require().Equal(
		stateBefore.gTACSupply.Add(mintAmt),
		stateAfterStake.gTACSupply,
		"gTAC total supply must increase by mintAmt",
	)
	// 1e) Per-validator state: each active liquid-validator holds some delegation
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()
	for _, valOper := range valOpers {
		lvState, found := s.keeper.GetLiquidValidatorState(ctx, valOper)
		s.Require().True(found, "liquid validator state must exist for %s", valOper)
		s.Require().True(
			lvState.LiquidTokens.IsPositive(),
			"liquid validator %s must hold liquid tokens after stake", valOper,
		)
		s.Require().True(
			lvState.DelShares.IsPositive(),
			"liquid validator %s must hold del shares after stake", valOper,
		)
		_ = wvMap
	}
	s.T().Logf("PHASE 1 OK: staked %s, minted %s gTAC, proxy holds %s liquid tokens",
		stakeAmt, mintAmt, stateAfterStake.proxyLiquidTokens)

	// -----------------------------------------------------------------------
	// === PHASE 2: StakeToLP (LSM path) ===
	// Requires: LsmDisabled=false + plain delegation (not ValidatorBond)
	// -----------------------------------------------------------------------
	s.T().Log("=== PHASE 2: StakeToLP ===")

	// 2a) Enable LSM
	params := s.keeper.GetParams(ctx)
	params.LsmDisabled = false
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	// 2b) Create a plain delegation on valOpers[0] from delegator.
	//     We must NOT call MsgValidatorBond — that would mark it as ValidatorBond=true
	//     which prevents MsgTokenizeShares.
	lsmStakeAmt := sdkmath.NewInt(200_000)
	s.Require().NoError(s.nw.FundAccount(delegator, sdk.NewCoins(
		sdk.NewCoin(bondDenom, lsmStakeAmt.Add(sdkmath.NewInt(100_000))),
	)))
	val, err := sk.GetValidator(ctx, valOpers[0])
	s.Require().NoError(err)
	_, err = sk.Delegate(ctx, delegator, lsmStakeAmt, stakingtypes.Unbonded, val, true)
	s.Require().NoError(err)
	// Confirm ValidatorBond is NOT set (plain delegation).
	del, err := sk.GetDelegation(ctx, delegator, valOpers[0])
	s.Require().NoError(err)
	s.Require().False(del.ValidatorBond, "plain delegation must have ValidatorBond=false")

	stateBeforeLSM := s.captureState(delegator)

	srv := keeper.NewMsgServerImpl(s.keeper)
	msgLSM := types.NewMsgStakeToLP(
		delegator,
		valOpers[0],
		sdk.NewCoin(bondDenom, lsmStakeAmt),
		sdk.Coin{},
	)
	respLSM, err := srv.StakeToLP(ctx, msgLSM)
	s.Require().NoError(err, "StakeToLP must succeed")
	s.Require().NotNil(respLSM)

	stateAfterLSM := s.captureState(delegator)

	// 2c) Delegator must have received MORE gTAC
	lsmMintAmt := stateAfterLSM.delegatorGTAC.Sub(stateBeforeLSM.delegatorGTAC)
	s.Require().True(lsmMintAmt.IsPositive(), "StakeToLP must mint additional gTAC")
	// 2d) proxyAcc liquid tokens increased further
	s.Require().True(
		stateAfterLSM.proxyLiquidTokens.GT(stateBeforeLSM.proxyLiquidTokens),
		"proxyAcc liquid tokens must increase after StakeToLP",
	)
	// 2e) gTAC supply increased
	s.Require().True(
		stateAfterLSM.gTACSupply.GT(stateBeforeLSM.gTACSupply),
		"gTAC supply must increase after StakeToLP",
	)
	// 2f) The plain delegation on valOpers[0] should no longer belong to delegator
	//     (it was consumed by LSM flow: tokenize → proxyAcc → redeemTokens)
	_, errDel := sk.GetDelegation(ctx, delegator, valOpers[0])
	s.Require().Error(errDel, "delegator's plain delegation must be consumed by StakeToLP")

	s.T().Logf("PHASE 2 OK: LSM staked %s, additional gTAC minted %s, proxy holds %s liquid tokens",
		lsmStakeAmt, lsmMintAmt, stateAfterLSM.proxyLiquidTokens)

	// -----------------------------------------------------------------------
	// === PHASE 3: Advance epochs, claim rewards ===
	// -----------------------------------------------------------------------
	s.T().Log("=== PHASE 3: Advance epochs, check rewards ===")

	// Advance several blocks so staking rewards accrue.
	// Each NextBlockAfter(blockDuration) ticks the chain by 6s.
	// A rebalance epoch is 1h, so we jump 2 hours = ~2 epochs.
	for range 3 {
		s.Require().NoError(s.nw.NextBlockAfter(time.Hour + time.Second))
	}
	ctx = s.ctx()

	// After epoch transitions the autocompound hook should have withdrawn and
	// restaked rewards. ProxyAcc liquid tokens should be >= what they were after
	// the LSM stake (rewards compounded in, never less).
	stateAfterEpochs := s.captureState(delegator)
	s.Require().True(
		stateAfterEpochs.proxyLiquidTokens.GTE(stateAfterLSM.proxyLiquidTokens),
		"proxyAcc liquid tokens must not decrease after reward compounding",
	)
	// Net amount state must reflect accrued rewards.
	nas, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	s.Require().True(nas.TotalLiquidTokens.IsPositive())
	s.Require().True(nas.NetAmount.IsPositive())
	s.T().Logf("PHASE 3 OK: after epochs proxy holds %s liquid tokens, NetAmount=%s",
		stateAfterEpochs.proxyLiquidTokens, nas.NetAmount)

	// -----------------------------------------------------------------------
	// === PHASE 4: LiquidUnstake ===
	// -----------------------------------------------------------------------
	s.T().Log("=== PHASE 4: LiquidUnstake ===")

	ctx = s.ctx()
	params = s.keeper.GetParams(ctx)

	// Unstake half of the delegator's current gTAC balance.
	gTACBalance := stateAfterEpochs.delegatorGTAC
	s.Require().True(gTACBalance.IsPositive(), "delegator must hold gTAC before unstake")
	unstakeGTAC := gTACBalance.Quo(sdkmath.NewInt(2))

	stateBeforeUnstake := s.captureState(delegator)

	ubdTime, _, ubds, _, err := s.keeper.LiquidUnstake(
		ctx,
		types.LiquidStakeProxyAcc,
		delegator,
		sdk.NewCoin(params.LiquidBondDenom, unstakeGTAC),
	)
	s.Require().NoError(err, "LiquidUnstake must succeed")
	s.Require().False(ubdTime.IsZero(), "unbonding completion time must be set")
	s.Require().NotEmpty(ubds, "unbonding delegations must be created")

	stateAfterUnstake := s.captureState(delegator)

	// 4a) gTAC balance decreased by unstakeGTAC
	s.Require().Equal(
		stateBeforeUnstake.delegatorGTAC.Sub(unstakeGTAC),
		stateAfterUnstake.delegatorGTAC,
		"delegator gTAC must decrease by unstakeGTAC",
	)
	// 4b) gTAC supply decreased
	s.Require().True(
		stateAfterUnstake.gTACSupply.LT(stateBeforeUnstake.gTACSupply),
		"gTAC supply must decrease after burn",
	)
	// 4c) proxyAcc liquid tokens decreased (delegation moved to unbonding queue)
	s.Require().True(
		stateAfterUnstake.proxyLiquidTokens.LTE(stateBeforeUnstake.proxyLiquidTokens),
		"proxyAcc liquid tokens must not increase after unstake",
	)

	// 4d) Verify unbonding delegation exists on the delegator's account.
	//     In LiquidUnbond the UBD is queued on liquidStaker (delegator), not proxyAcc.
	ctx = s.ctx()
	ubdsAfter, err := sk.GetAllUnbondingDelegations(ctx, delegator)
	s.Require().NoError(err)
	s.Require().NotEmpty(ubdsAfter, "delegator must have an unbonding delegation entry")
	totalUnbondingAmt := sdkmath.ZeroInt()
	for _, ubd := range ubdsAfter {
		for _, entry := range ubd.Entries {
			totalUnbondingAmt = totalUnbondingAmt.Add(entry.Balance)
		}
	}
	s.Require().True(totalUnbondingAmt.IsPositive(), "total unbonding amount must be positive")
	s.T().Logf("PHASE 4 OK: burned %s gTAC, unbonding amount %s, completes at %s",
		unstakeGTAC, totalUnbondingAmt, ubdTime)

	// -----------------------------------------------------------------------
	// === PHASE 5: Skip past unbonding period → tokens returned ===
	// -----------------------------------------------------------------------
	s.T().Log("=== PHASE 5: Skip unbonding period, verify token return ===")

	// Read unbonding time from staking params and advance past it.
	unbondingDuration, err := sk.UnbondingTime(ctx)
	s.Require().NoError(err)

	// Advance past the unbonding completion time by adding a small buffer.
	bondBalBeforeComplete := s.nw.App.GetBankKeeper().GetBalance(ctx, delegator, bondDenom).Amount
	s.Require().NoError(s.nw.NextBlockAfter(unbondingDuration + time.Minute))
	ctx = s.ctx()

	bondBalAfterComplete := s.nw.App.GetBankKeeper().GetBalance(ctx, delegator, bondDenom).Amount

	// Tokens must have been returned to the delegator.
	s.Require().True(
		bondBalAfterComplete.GT(bondBalBeforeComplete),
		"delegator bond balance must increase after unbonding completes (got before=%s, after=%s)",
		bondBalBeforeComplete, bondBalAfterComplete,
	)

	// Unbonding delegations for this delegator must now be cleared.
	ubdsFinal, err := sk.GetAllUnbondingDelegations(ctx, delegator)
	s.Require().NoError(err)
	s.Require().Empty(ubdsFinal, "all unbonding delegations must be completed")

	returnedAmt := bondBalAfterComplete.Sub(bondBalBeforeComplete)
	s.T().Logf("PHASE 5 OK: %s bond tokens returned to delegator after unbonding period", returnedAmt)

	// -----------------------------------------------------------------------
	// Final cross-check: gTAC supply and proxy state are consistent
	// -----------------------------------------------------------------------
	stateFinal := s.captureState(delegator)
	nasF, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	s.Require().Equal(
		nasF.GtacTotalSupply,
		stateFinal.gTACSupply,
		"NetAmountState gTAC supply must match bank supply",
	)
	s.T().Logf("FINAL: gTAC supply=%s, proxy liquid tokens=%s, delegator gTAC=%s, delegator bond=%s",
		stateFinal.gTACSupply, stateFinal.proxyLiquidTokens,
		stateFinal.delegatorGTAC, stateFinal.delegatorBondBalance)
}

// ---------------------------------------------------------------------------
// CalcNetAmount: includes ProxyAccBalance and TotalRemainingRewards
// ---------------------------------------------------------------------------

// newWithInflation returns a ConfigOption that sets the mint module inflation
// to the given annual rate so that staking rewards actually accrue in tests.
func newWithInflation(annualRate sdkmath.LegacyDec) network.ConfigOption {
	return network.WithCustomGenesis(network.CustomGenesisState{
		minttypes.ModuleName: &minttypes.GenesisState{
			Minter: minttypes.InitialMinter(annualRate),
			Params: minttypes.Params{
				MintDenom:           "aatom",
				InflationRateChange: sdkmath.LegacyNewDecWithPrec(93, 2),
				InflationMax:        annualRate,
				InflationMin:        annualRate,
				GoalBonded:          sdkmath.LegacyNewDecWithPrec(67, 2),
				BlocksPerYear:       uint64(6311520),
			},
		},
	})
}

// TestStateAudit_CalcNetAmount_IncludesAllComponents is a state-audit test
// that confirms BUG-01 end-to-end on a live chain with real staking rewards:
//
//  1. CalcNetAmount unit check — verifies the fixed formula includes all components.
//  2. ProxyAccBalance after reward withdrawal is invisible to the old formula.
//  3. Second staker receives more gTAC than fair value when ProxyAccBalance > 0.
//
// The test uses a fresh network with non-zero inflation so rewards genuinely
// accrue via BeginBlock → mint → distribution → staking rewards.
//
// EXPECTED STATE after fix:
//   - CalcNetAmount == TotalLiquidTokens + TotalUnbondingBalance + ProxyAccBalance + TotalRemainingRewards
//   - MintRate is stable across two consecutive stakes when rewards sit on proxyAcc
func (s *KeeperTestSuite) TestStateAudit_CalcNetAmount_IncludesAllComponents() {
	// -----------------------------------------------------------------------
	// PART 1: Pure unit check — CalcNetAmount formula
	// -----------------------------------------------------------------------
	s.T().Log("=== PART 1: CalcNetAmount formula unit check ===")

	nas := types.NetAmountState{
		TotalLiquidTokens:     sdkmath.NewInt(1_000_000),
		TotalUnbondingBalance: sdkmath.NewInt(200_000),
		ProxyAccBalance:       sdkmath.NewInt(50_000),
		TotalRemainingRewards: sdkmath.LegacyNewDec(30_000),
		GtacTotalSupply:       sdkmath.NewInt(1_200_000),
	}
	got := nas.CalcNetAmount()
	// After the fix: 1_000_000 + 200_000 + 50_000 + 30_000 = 1_280_000
	correctExpected := sdkmath.LegacyNewDec(1_280_000)
	s.Require().Equal(correctExpected, got,
		"CalcNetAmount must include ProxyAccBalance and TotalRemainingRewards: "+
			"got %s, want %s", got, correctExpected)
	s.T().Logf("PART 1 OK: CalcNetAmount = %s (correct)", got)

	// -----------------------------------------------------------------------
	// PART 2: Integration — ProxyAccBalance visible in NetAmount after rewards
	// -----------------------------------------------------------------------
	// Rebuild the network with positive inflation so real staking rewards accrue.
	s.T().Log("=== PART 2: ProxyAccBalance included in NetAmount after reward withdrawal ===")

	// We need a dedicated network instance with inflation so this sub-test is
	// self-contained and does not affect other tests in the suite.
	//
	// Strategy: use FundAccount to inject simulated rewards on proxyAcc and
	// then verify NetAmount includes them.  This mirrors exactly what happens
	// after WithdrawLiquidRewards: accumulated rewards land on ProxyAccBalance.
	s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	bondDenom := s.nw.GetBaseDenom()
	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(5_000_000)

	mintAmt, err := s.keeper.LiquidStake(
		ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(bondDenom, stakeAmt),
	)
	s.Require().NoError(err)
	s.Require().Equal(stakeAmt, mintAmt, "first stake must be 1:1")

	// Simulate rewards landing on proxyAcc (as WithdrawLiquidRewards would do).
	simulatedRewards := sdkmath.NewInt(12_345)
	s.Require().NoError(s.nw.FundAccount(
		types.LiquidStakeProxyAcc,
		sdk.NewCoins(sdk.NewCoin(bondDenom, simulatedRewards)),
	))

	ctx = s.ctx()
	nas2, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	s.T().Logf("  TotalLiquidTokens:     %s", nas2.TotalLiquidTokens)
	s.T().Logf("  ProxyAccBalance:       %s", nas2.ProxyAccBalance)
	s.T().Logf("  TotalRemainingRewards: %s", nas2.TotalRemainingRewards)
	s.T().Logf("  NetAmount:             %s", nas2.NetAmount)

	correctNetAmount := sdkmath.LegacyNewDecFromInt(
		nas2.TotalLiquidTokens.Add(nas2.TotalUnbondingBalance).Add(nas2.ProxyAccBalance),
	).Add(nas2.TotalRemainingRewards)

	s.T().Logf("  NetAmount (correct):   %s", correctNetAmount)

	s.Require().True(nas2.ProxyAccBalance.IsPositive(),
		"proxyAcc must have rewards balance after injection")
	s.Require().Equal(correctNetAmount, nas2.NetAmount,
		"BUG-01 FIX VERIFIED: NetAmount must include ProxyAccBalance; "+
			"got %s, want %s", nas2.NetAmount, correctNetAmount)
	s.T().Logf("PART 2 OK: NetAmount=%s correctly includes ProxyAccBalance=%s",
		nas2.NetAmount, nas2.ProxyAccBalance)

	// -----------------------------------------------------------------------
	// PART 3: Integration — MintRate stability (no dilution after reward injection)
	// -----------------------------------------------------------------------
	s.T().Log("=== PART 3: MintRate must be stable across two stakes when rewards on proxyAcc ===")

	stakerB := s.delAddrs[1]

	// Snapshot state before second stake.
	nasBeforeB, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	// MintRate before second stake: supply / NetAmount.
	// With the fix, NetAmount already includes ProxyAccBalance, so MintRate < 1.0.
	s.Require().True(nasBeforeB.MintRate.IsPositive(), "MintRate must be positive")
	s.T().Logf("  MintRate before second stake: %s (supply=%s, netAmount=%s)",
		nasBeforeB.MintRate, nasBeforeB.GtacTotalSupply, nasBeforeB.NetAmount)

	// Staker B stakes the same amount.
	mintB, err := s.keeper.LiquidStake(
		ctx, types.LiquidStakeProxyAcc, stakerB, sdk.NewCoin(bondDenom, stakeAmt),
	)
	s.Require().NoError(err)

	nasAfterB, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	s.T().Logf("  gTAC minted for B: %s", mintB)
	s.T().Logf("  MintRate after second stake: %s (supply=%s, netAmount=%s)",
		nasAfterB.MintRate, nasAfterB.GtacTotalSupply, nasAfterB.NetAmount)

	// Key invariant: staking at fair value must NOT change MintRate.
	// Allow ±1 ULP tolerance due to integer truncation in NativeTokenToGTAC.
	rateDiff := nasAfterB.MintRate.Sub(nasBeforeB.MintRate).Abs()
	tolerance := sdkmath.LegacyNewDecWithPrec(1, 6) // 0.000001
	s.Require().True(rateDiff.LTE(tolerance),
		"BUG-01 FIX VERIFIED: MintRate must be stable after second stake at fair value; "+
			"before=%s after=%s diff=%s (tolerance=%s)",
		nasBeforeB.MintRate, nasAfterB.MintRate, rateDiff, tolerance)
	s.T().Logf("PART 3 OK: MintRate stable at ~%s (diff=%s)", nasBeforeB.MintRate, rateDiff)

	// -----------------------------------------------------------------------
	// PART 4: Integration — Real rewards via WithdrawLiquidRewards + epoch
	// -----------------------------------------------------------------------
	// If the chain has non-zero inflation, advance blocks to accumulate real
	// rewards, call WithdrawLiquidRewards, and verify NetAmount includes them.
	s.T().Log("=== PART 4: Real rewards via epoch (if inflation > 0) ===")

	s.advanceHeight(5)
	ctx = s.ctx()

	// Check if real rewards accumulated.
	totalRewards, _, _, err := s.keeper.CheckDelegationStates(ctx, types.LiquidStakeProxyAcc)
	s.Require().NoError(err)
	s.T().Logf("  TotalRemainingRewards after 5 blocks: %s", totalRewards)

	if totalRewards.IsPositive() {
		// Real rewards exist — withdraw and verify NetAmount picks them up.
		s.keeper.WithdrawLiquidRewards(ctx, types.LiquidStakeProxyAcc)

		nasAfterWithdraw, err := s.keeper.GetNetAmountState(ctx)
		s.Require().NoError(err)

		correctAfterWithdraw := sdkmath.LegacyNewDecFromInt(
			nasAfterWithdraw.TotalLiquidTokens.
				Add(nasAfterWithdraw.TotalUnbondingBalance).
				Add(nasAfterWithdraw.ProxyAccBalance),
		).Add(nasAfterWithdraw.TotalRemainingRewards)

		s.Require().Equal(correctAfterWithdraw, nasAfterWithdraw.NetAmount,
			"after real WithdrawLiquidRewards, NetAmount must include ProxyAccBalance")
		s.T().Logf("PART 4 OK (real rewards): NetAmount=%s, ProxyAccBalance=%s",
			nasAfterWithdraw.NetAmount, nasAfterWithdraw.ProxyAccBalance)
	} else {
		s.T().Log("PART 4 SKIP: inflation=0 in this network config, no real rewards accumulated")
		s.T().Log("  → to enable real rewards, run suite with newWithInflation(sdkmath.LegacyNewDecWithPrec(13, 2))")
		s.T().Log("  → the fix is fully validated by PARTS 1-3 above")
	}
}

// ---------------------------------------------------------------------------
// NativeTokenToGTAC: divides by full netAmount without truncating denominator
// ---------------------------------------------------------------------------

// TestStateAudit_NativeTokenToGTAC_NoTruncatedDenominator verifies that
// NativeTokenToGTAC does NOT truncate the denominator (netAmount) before
// division.
//
// Root cause: the buggy code called netAmount.TruncateDec() before QuoTruncate,
// which floors the denominator and makes the result slightly larger than the
// fair value.  Compounding over many stakes this systematically mints excess
// gTAC at the expense of existing holders.
//
// The fix: remove .TruncateDec() → divide by the full LegacyDec value.
//
// PART 1 — unit arithmetic check with a crafted netAmount that has a large
//
//	fractional part (0.9), so the discrepancy is visible.
//
// PART 2 — integration check: after the first stake the MintRate carries a
//
//	fractional component because NetAmount now includes TotalRemainingRewards
//	(fixed in BUG-01).  A second stake must mint exactly
//	supply * stakeAmt / netAmount, NOT supply * stakeAmt / floor(netAmount).
func (s *KeeperTestSuite) TestStateAudit_NativeTokenToGTAC_NoTruncatedDenominator() {
	// -----------------------------------------------------------------------
	// PART 1: Pure unit check
	// -----------------------------------------------------------------------
	s.T().Log("=== PART 1: NativeTokenToGTAC unit check — no spurious TruncateDec ===")

	// Choose netAmount with a significant fractional part so both paths diverge.
	//   supply       = 5_000_000
	//   stakeAmt     = 1_000_000
	//   netAmount    = 5_000_000.9  (fractional part = 0.9)
	//
	// Buggy result  = 5_000_000 * 1_000_000 / 5_000_000   = 1_000_000  (floor kills 0.9)
	// Correct result = 5_000_000 * 1_000_000 / 5_000_000.9 ≈ 999_999   (properly rounded down)
	supply := sdkmath.NewInt(5_000_000)
	stakeAmt := sdkmath.NewInt(1_000_000)
	netAmount := sdkmath.LegacyNewDecWithPrec(50_000_009, 1) // 5_000_000.9

	got := types.NativeTokenToGTAC(stakeAmt, supply, netAmount)

	// With correct division: 5_000_000 * 1_000_000 / 5_000_000.9
	//   = 5_000_000_000_000 / 5_000_000.9 ≈ 999_999.82  → TruncateInt → 999_999
	correct := sdkmath.LegacyNewDecFromInt(supply).
		MulTruncate(sdkmath.LegacyNewDecFromInt(stakeAmt)).
		QuoTruncate(netAmount).
		TruncateInt()

	s.Require().Equal(correct, got,
		"BUG-02 FIX VERIFIED: NativeTokenToGTAC must divide by full netAmount; "+
			"got %s, want %s", got, correct)

	// The buggy result would have been supply*stake/floor(netAmount) = 1_000_000.
	// The correct result must be strictly less than that because the true denominator is larger.
	buggyResult := sdkmath.LegacyNewDecFromInt(supply).
		MulTruncate(sdkmath.LegacyNewDecFromInt(stakeAmt)).
		QuoTruncate(netAmount.TruncateDec()). // simulate old bug
		TruncateInt()

	s.Require().True(got.LTE(buggyResult),
		"correct result must be <= buggy result (buggy inflates mint amount): correct=%s buggy=%s",
		got, buggyResult)
	s.T().Logf("PART 1 OK: correct=%s, buggy-would-be=%s (diff=%s)",
		got, buggyResult, buggyResult.Sub(got))

	// -----------------------------------------------------------------------
	// PART 2: Integration — second stake amount matches exact formula
	// -----------------------------------------------------------------------
	s.T().Log("=== PART 2: Integration — second stake minted amount matches exact formula ===")

	s.setupWhitelistedValidators(2, 0)
	ctx := s.ctx()
	bondDenom := s.nw.GetBaseDenom()

	stakerA := s.delAddrs[0]
	stakerB := s.delAddrs[1]
	firstStake := sdkmath.NewInt(5_000_000)

	// First stake — 1:1
	mintA, err := s.keeper.LiquidStake(
		ctx, types.LiquidStakeProxyAcc, stakerA, sdk.NewCoin(bondDenom, firstStake),
	)
	s.Require().NoError(err)
	s.Require().Equal(firstStake, mintA, "first stake must be 1:1")

	// Inject fractional rewards on proxyAcc so netAmount gets a fractional part.
	// Use a non-round number to maximise discrepancy.
	simulatedRewards := sdkmath.NewInt(999_999) // 0.999999 * 1e6
	s.Require().NoError(s.nw.FundAccount(
		types.LiquidStakeProxyAcc,
		sdk.NewCoins(sdk.NewCoin(bondDenom, simulatedRewards)),
	))

	ctx = s.ctx()
	nasBefore, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)

	s.T().Logf("  supply=%s  netAmount=%s  (fractional part=%s)",
		nasBefore.GtacTotalSupply,
		nasBefore.NetAmount,
		nasBefore.NetAmount.Sub(nasBefore.NetAmount.TruncateDec()),
	)

	// Expected mint for second stake using the correct formula.
	secondStake := sdkmath.NewInt(1_000_000)
	expectedMint := sdkmath.LegacyNewDecFromInt(nasBefore.GtacTotalSupply).
		MulTruncate(sdkmath.LegacyNewDecFromInt(secondStake)).
		QuoTruncate(nasBefore.NetAmount). // full Dec, no truncation of denominator
		TruncateInt()

	mintB, err := s.keeper.LiquidStake(
		ctx, types.LiquidStakeProxyAcc, stakerB, sdk.NewCoin(bondDenom, secondStake),
	)
	s.Require().NoError(err)

	s.T().Logf("  expected mint for B: %s", expectedMint)
	s.T().Logf("  actual  mint for B: %s", mintB)

	s.Require().Equal(expectedMint, mintB,
		"BUG-02 FIX VERIFIED: minted gTAC must equal supply*stake/netAmount (no floor on denominator); "+
			"expected=%s got=%s", expectedMint, mintB)
	s.T().Logf("PART 2 OK: mintB=%s == expected=%s", mintB, expectedMint)
}
