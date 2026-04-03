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
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

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
