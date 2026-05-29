package liquidstake

// TestStateAudit is a comprehensive end-to-end state audit for the liquidstake module.
// It covers the two major public flows in a single test scenario:
//
//  1. LiquidStake  – delegator sends tokens → gTAC minted, proxyAcc delegation appears.
//  2. LiquidUnstake – delegator burns gTAC → unbonding delegation created on liquidStaker,
//     time skipped past unbonding period → original tokens returned to delegator.
//
// At every step we verify:
//   - delegator's bondDenom balance
//   - delegator's gTAC (LiquidBondDenom) balance
//   - proxyAcc delegation (total liquid tokens held by the proxy)
//   - per-validator LiquidTokens via GetLiquidValidatorState
//   - gTAC total supply (NetAmountState)
//
//nolint:funlen
import (
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

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
	// === PHASE 2: Advance epochs, claim rewards ===
	// -----------------------------------------------------------------------
	s.T().Log("=== PHASE 2: Advance epochs, check rewards ===")

	// Advance several blocks so staking rewards accrue.
	// Each NextBlockAfter(blockDuration) ticks the chain by 6s.
	// A rebalance epoch is 1h, so we jump 2 hours = ~2 epochs.
	// CommitState first so writes survive the upcoming NextBlockAfter calls.
	s.Require().NoError(s.nw.CommitState())
	for range 3 {
		s.Require().NoError(s.nw.NextBlockAfter(time.Hour + time.Second))
	}
	ctx = s.ctx()

	// After epoch transitions the autocompound hook should have withdrawn and
	// restaked rewards. ProxyAcc liquid tokens should be >= what they were after
	// the liquid stake (rewards compounded in, never less).
	stateAfterEpochs := s.captureState(delegator)
	s.Require().True(
		stateAfterEpochs.proxyLiquidTokens.GTE(stateAfterStake.proxyLiquidTokens),
		"proxyAcc liquid tokens must not decrease after reward compounding",
	)
	// Net amount state must reflect accrued rewards.
	nas, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	s.Require().True(nas.TotalLiquidTokens.IsPositive())
	s.Require().True(nas.NetAmount.IsPositive())
	s.T().Logf("PHASE 2 OK: after epochs proxy holds %s liquid tokens, NetAmount=%s",
		stateAfterEpochs.proxyLiquidTokens, nas.NetAmount)

	// -----------------------------------------------------------------------
	// === PHASE 3: LiquidUnstake ===
	// -----------------------------------------------------------------------
	s.T().Log("=== PHASE 3: LiquidUnstake ===")

	// Commit epoch-advance writes before calling LiquidUnstake.
	s.Require().NoError(s.nw.CommitState())
	ctx = s.ctx()
	params := s.keeper.GetParams(ctx)

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
	s.T().Logf("PHASE 3 OK: burned %s gTAC, unbonding amount %s, completes at %s",
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
	// Commit LiquidUnstake writes so the unbonding entry survives NextBlockAfter.
	s.Require().NoError(s.nw.CommitState())
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
// CalcNetAmount: delegated and unbonding baseline
// ---------------------------------------------------------------------------

// TestStateAudit_CalcNetAmount_DelegatedAndUnbondingBaseline documents the
// current stable part of the formula without deciding whether proxy account
// balances or remaining rewards should be included in future accounting.
func (s *KeeperTestSuite) TestStateAudit_CalcNetAmount_DelegatedAndUnbondingBaseline() {
	nas := types.NetAmountState{
		TotalLiquidTokens:     sdkmath.NewInt(1_000_000),
		TotalUnbondingBalance: sdkmath.NewInt(200_000),
	}

	expected := sdkmath.LegacyNewDec(1_200_000)
	got := nas.CalcNetAmount()
	s.Require().Equal(expected, got)
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
// The check uses a crafted netAmount with a large fractional part (0.9), so the
// discrepancy is visible without tying the test to unresolved NetAmount
// accounting choices.
func (s *KeeperTestSuite) TestStateAudit_NativeTokenToGTAC_NoTruncatedDenominator() {
	s.T().Log("=== NativeTokenToGTAC unit check — no spurious TruncateDec ===")

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
	s.T().Logf("OK: correct=%s, buggy-would-be=%s (diff=%s)",
		got, buggyResult, buggyResult.Sub(got))
}
