package liquidstake

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/evm/x/liquidstake/types"
)

func (s *KeeperTestSuite) TestMaxEntries() {
	s.setupWhitelistedValidators(3, 1_000_000)

	// Re-read ctx and params after setupWhitelistedValidators (which calls NextBlock internally).
	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	acc0, acc1 := s.delAddrs[0], s.delAddrs[1]
	stakeAmt := sdkmath.NewInt(1_000_000)
	maxEntries := int(skParams.MaxEntries)

	activeVals := s.keeper.GetActiveLiquidValidators(ctx, params.WhitelistedValsMap())
	allLvs := s.keeper.GetAllLiquidValidators(ctx)
	s.Require().NotEmpty(activeVals)
	s.Require().NotEmpty(allLvs)

	for range maxEntries {
		gTAC, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, acc0, sdk.NewCoin(skParams.BondDenom, stakeAmt))
		s.Require().NoError(err)
		s.Require().Equal(stakeAmt, gTAC)

		gTAC, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, acc1, sdk.NewCoin(skParams.BondDenom, stakeAmt))
		s.Require().NoError(err)
		s.Require().Equal(stakeAmt, gTAC)
	}

	unstakeAmt := sdkmath.NewInt(int64(maxEntries)).Mul(stakeAmt).Quo(sdkmath.NewInt(int64(maxEntries + 1)))

	for range maxEntries {
		ctx = s.ctx()
		_, _, _, _, err = s.keeper.LiquidUnstake(ctx, types.LiquidStakeProxyAcc, acc0, sdk.NewCoin(params.LiquidBondDenom, unstakeAmt))
		s.Require().NoError(err)
		s.advanceHeight(1)
	}
	ctx = s.ctx()
	_, _, _, _, err = s.keeper.LiquidUnstake(ctx, types.LiquidStakeProxyAcc, acc0, sdk.NewCoin(params.LiquidBondDenom, unstakeAmt))
	s.Require().ErrorIs(err, stakingtypes.ErrMaxUnbondingDelegationEntries)

	for range maxEntries {
		ctx = s.ctx()
		_, _, _, _, err = s.keeper.LiquidUnstake(ctx, types.LiquidStakeProxyAcc, acc1, sdk.NewCoin(params.LiquidBondDenom, unstakeAmt))
		s.Require().NoError(err)
		s.advanceHeight(1)
	}
	ctx = s.ctx()
	_, _, _, _, err = s.keeper.LiquidUnstake(ctx, types.LiquidStakeProxyAcc, acc1, sdk.NewCoin(params.LiquidBondDenom, unstakeAmt))
	s.Require().ErrorIs(err, stakingtypes.ErrMaxUnbondingDelegationEntries)
}

func (s *KeeperTestSuite) TestLiquidStakeBasic() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(500_000)

	balBefore := s.nw.App.GetBankKeeper().GetBalance(ctx, staker, params.LiquidBondDenom).Amount
	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)
	s.Require().True(mintAmt.GT(sdkmath.ZeroInt()))

	balAfter := s.nw.App.GetBankKeeper().GetBalance(ctx, staker, params.LiquidBondDenom).Amount
	s.Require().Equal(mintAmt, balAfter.Sub(balBefore))
}

func (s *KeeperTestSuite) TestLiquidStakeModulePaused() {
	s.setupWhitelistedValidators(2, 1_000_000)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	params.ModulePaused = true
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)

	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, s.delAddrs[0],
		sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(100_000)))
	s.Require().ErrorIs(err, types.ErrModulePaused)
}

func (s *KeeperTestSuite) TestLiquidStakeBelowMinAmount() {
	s.setupWhitelistedValidators(2, 1_000_000)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	params.MinLiquidStakeAmount = sdkmath.NewInt(1_000_000)
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)

	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, s.delAddrs[0],
		sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(999)))
	s.Require().ErrorIs(err, types.ErrLessThanMinLiquidStakeAmount)
}

func (s *KeeperTestSuite) TestLiquidUnstakeBasic() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(500_000)

	mintAmt, err := s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
	s.Require().NoError(err)
	s.Require().True(mintAmt.GT(sdkmath.ZeroInt()))

	ctx = s.ctx()
	unstakeAmt := mintAmt.Quo(sdkmath.NewInt(2))
	_, _, _, _, err = s.keeper.LiquidUnstake(ctx, types.LiquidStakeProxyAcc, staker,
		sdk.NewCoin(params.LiquidBondDenom, unstakeAmt))
	s.Require().NoError(err)
}

func (s *KeeperTestSuite) TestLiquidUnstakeRejectsZeroTruncatedValidatorAmounts() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	params := s.keeper.GetParams(ctx)

	staker := s.delAddrs[0]
	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker,
		sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(1_000_000)))
	s.Require().NoError(err)

	before := s.nw.App.GetBankKeeper().GetBalance(ctx, staker, params.LiquidBondDenom).Amount
	s.Require().True(before.IsPositive())

	_, _, _, _, err = s.keeper.LiquidUnstake(ctx, types.LiquidStakeProxyAcc, staker,
		sdk.NewCoin(params.LiquidBondDenom, sdkmath.NewInt(1)))
	s.Require().ErrorIs(err, types.ErrTooSmallLiquidUnstakingAmount)

	after := s.nw.App.GetBankKeeper().GetBalance(ctx, staker, params.LiquidBondDenom).Amount
	s.Require().Equal(before, after)
}

func (s *KeeperTestSuite) TestLiquidValidatorsLifecycle() {
	_, valOpers, _ := s.CreateValidators([]int64{1_000_000, 1_000_000})
	ctx := s.ctx()

	lv1 := types.LiquidValidator{OperatorAddress: valOpers[0].String()}
	lv2 := types.LiquidValidator{OperatorAddress: valOpers[1].String()}

	s.keeper.SetLiquidValidator(ctx, lv1)
	s.keeper.SetLiquidValidator(ctx, lv2)

	lvs := s.keeper.GetAllLiquidValidators(ctx)
	s.Require().GreaterOrEqual(len(lvs), 2)

	s.keeper.RemoveLiquidValidator(ctx, lv1)
	lvs = s.keeper.GetAllLiquidValidators(ctx)
	for _, lv := range lvs {
		s.Require().NotEqual(lv1.OperatorAddress, lv.OperatorAddress)
	}
}

func (s *KeeperTestSuite) TestGetNetAmountState() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)

	stakeAmt := sdkmath.NewInt(300_000)
	for _, staker := range s.delAddrs[:3] {
		_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, staker, sdk.NewCoin(skParams.BondDenom, stakeAmt))
		s.Require().NoError(err)
	}

	ctx = s.ctx()
	nas, err := s.keeper.GetNetAmountState(ctx)
	s.Require().NoError(err)
	s.Require().True(nas.TotalLiquidTokens.GT(sdkmath.ZeroInt()), "TotalLiquidTokens should be > 0")
	s.Require().True(nas.NetAmount.GT(sdkmath.LegacyZeroDec()), "NetAmount should be > 0")
}

func (s *KeeperTestSuite) TestUpdateWhitelistedValidators() {
	s.setupWhitelistedValidators(3, 1_000_000)

	ctx := s.ctx()
	wm := s.keeper.GetParams(ctx).WhitelistedValsMap()
	activeBefore := s.keeper.GetActiveLiquidValidators(ctx, wm)
	s.Require().Len(activeBefore, 3)

	params := s.keeper.GetParams(ctx)
	params.WhitelistedValidators = params.WhitelistedValidators[:2]
	weights := equalTargetWeights(2)
	for i := range params.WhitelistedValidators {
		params.WhitelistedValidators[i].TargetWeight = weights[i]
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	ctx = s.ctx()
	wm2 := s.keeper.GetParams(ctx).WhitelistedValsMap()
	activeAfter := s.keeper.GetActiveLiquidValidators(ctx, wm2)
	s.Require().Len(activeAfter, 2)
}

func (s *KeeperTestSuite) TestGetSetParams() {
	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)

	params.UnstakeFeeRate = sdkmath.LegacyMustNewDecFromStr("0.002")
	params.MinLiquidStakeAmount = sdkmath.NewInt(5_000)
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	got := s.keeper.GetParams(ctx)
	s.Require().Equal(params.UnstakeFeeRate, got.UnstakeFeeRate)
	s.Require().Equal(params.MinLiquidStakeAmount, got.MinLiquidStakeAmount)
}
