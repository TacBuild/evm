package keeper_test

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/evm/x/liquidstake/types"
)

func (s *KeeperTestSuite) TestMaxEntries() {
	stakingParams, err := s.app.StakingKeeper.GetParams(s.ctx)
	s.Require().NoError(err)

	// Create validators
	numValidators := 3

	power := make([]int64, numValidators)
	for i := range numValidators {
		power[i] = 1000000
	}
	_, valOpers, _ := s.CreateValidators(power)

	liquidstakeParams := s.keeper.GetParams(s.ctx)
	liquidstakeParams.WhitelistedValidators = []types.WhitelistedValidator{}

	valWeight := types.TotalValidatorWeight.Quo(math.NewInt(int64(numValidators)))

	for i := range numValidators {
		liquidstakeParams.WhitelistedValidators = append(liquidstakeParams.WhitelistedValidators, types.WhitelistedValidator{
			ValidatorAddress: valOpers[i].String(),
			TargetWeight:     valWeight,
		})
	}
	liquidstakeParams.ModulePaused = false
	s.Require().NoError(s.keeper.SetParams(s.ctx, liquidstakeParams))
	s.keeper.UpdateLiquidValidatorSet(s.ctx, true)

	// Create test accounts
	acc0, acc1 := s.delAddrs[0], s.delAddrs[1]

	// Liquidstake
	stakeAmount := math.NewInt(1_000)
	numStakes := stakingParams.MaxEntries
	for range numStakes {
		gTACAmount, err := s.keeper.LiquidStake(s.ctx, types.LiquidStakeProxyAcc, acc0, sdk.NewCoin(stakingParams.BondDenom, stakeAmount))
		s.Require().NoError(err)
		s.Require().Equal(gTACAmount, stakeAmount)

		gTACAmount, err = s.keeper.LiquidStake(s.ctx, types.LiquidStakeProxyAcc, acc1, sdk.NewCoin(stakingParams.BondDenom, stakeAmount))
		s.Require().NoError(err)
		s.Require().Equal(gTACAmount, stakeAmount)
	}

	// LiquidUnstake
	unstakeAmount := math.NewInt(int64(numStakes)).Mul(stakeAmount).Quo(math.NewInt(int64(stakingParams.MaxEntries + 1)))
	for range stakingParams.MaxEntries {
		_, _, _, _, err = s.keeper.LiquidUnstake(s.ctx, types.LiquidStakeProxyAcc, acc0, sdk.NewCoin(liquidstakeParams.LiquidBondDenom, unstakeAmount))
		s.advanceHeight(1, false)
		s.Require().NoError(err)
	}

	_, _, _, _, err = s.keeper.LiquidUnstake(s.ctx, types.LiquidStakeProxyAcc, acc0, sdk.NewCoin(liquidstakeParams.LiquidBondDenom, unstakeAmount))
	s.Require().ErrorIs(err, stakingtypes.ErrMaxUnbondingDelegationEntries)

	for range stakingParams.MaxEntries {
		_, _, _, _, err = s.keeper.LiquidUnstake(s.ctx, types.LiquidStakeProxyAcc, acc1, sdk.NewCoin(liquidstakeParams.LiquidBondDenom, unstakeAmount))
		s.advanceHeight(1, false)
		s.Require().NoError(err)
	}

	_, _, _, _, err = s.keeper.LiquidUnstake(s.ctx, types.LiquidStakeProxyAcc, acc1, sdk.NewCoin(liquidstakeParams.LiquidBondDenom, unstakeAmount))
	s.Require().ErrorIs(err, stakingtypes.ErrMaxUnbondingDelegationEntries)
}
