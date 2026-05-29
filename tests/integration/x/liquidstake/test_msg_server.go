package liquidstake

// TestMsgServer contains integration tests for the x/liquidstake MsgServer layer.
// Each test drives the server through types.NewMsgServerImpl(keeper) to exercise
// the msg_server.go code-paths that are otherwise unreachable from keeper-level
// tests.

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"github.com/cosmos/evm/x/liquidstake/keeper"
	"github.com/cosmos/evm/x/liquidstake/types"
)

// govAuthority returns the governance module address used as the default
// authority when the liquidstake keeper is constructed.
func govAuthority() string {
	return authtypes.NewModuleAddress(govtypes.ModuleName).String()
}

// ---------------------------------------------------------------------------
// MsgLiquidStake
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestMsgServer_LiquidStake_Success() {
	s.setupWhitelistedValidators(3, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(500_000)

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgLiquidStake(staker, sdk.NewCoin(bondDenom, stakeAmt))

	resp, err := srv.LiquidStake(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	// gTAC balance must increase.
	params := s.keeper.GetParams(ctx)
	bal := s.nw.App.GetBankKeeper().GetBalance(ctx, staker, params.LiquidBondDenom)
	s.Require().True(bal.IsPositive(), "staker must hold gTAC after LiquidStake")
}

func (s *KeeperTestSuite) TestMsgServer_LiquidStake_ModulePaused() {
	s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)
	params.ModulePaused = true
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgLiquidStake(s.delAddrs[0], sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000)))

	_, err = srv.LiquidStake(ctx, msg)
	s.Require().ErrorIs(err, types.ErrModulePaused)
}

// ---------------------------------------------------------------------------
// MsgLiquidUnstake
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestMsgServer_LiquidUnstake_Success() {
	s.setupWhitelistedValidators(3, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	staker := s.delAddrs[0]
	stakeAmt := sdkmath.NewInt(1_000_000)

	srv := keeper.NewMsgServerImpl(s.keeper)

	// Stake first via MsgServer.
	_, err = srv.LiquidStake(ctx, types.NewMsgLiquidStake(staker, sdk.NewCoin(bondDenom, stakeAmt)))
	s.Require().NoError(err)

	// Read gTAC balance after staking.
	ctx = s.ctx()
	params := s.keeper.GetParams(ctx)
	gTACBal := s.nw.App.GetBankKeeper().GetBalance(ctx, staker, params.LiquidBondDenom)
	s.Require().True(gTACBal.IsPositive())

	// Unstake half.
	unstakeAmt := gTACBal.Amount.Quo(sdkmath.NewInt(2))
	msg := types.NewMsgLiquidUnstake(staker, sdk.NewCoin(params.LiquidBondDenom, unstakeAmt))

	ctx = s.ctx()
	resp, err := srv.LiquidUnstake(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().False(resp.CompletionTime.IsZero(), "completion time must be set")
}

func (s *KeeperTestSuite) TestMsgServer_LiquidUnstake_NoBalance() {
	s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	params := s.keeper.GetParams(ctx)

	srv := keeper.NewMsgServerImpl(s.keeper)
	// Staker has no gTAC — unstake must fail.
	msg := types.NewMsgLiquidUnstake(s.delAddrs[0], sdk.NewCoin(params.LiquidBondDenom, sdkmath.NewInt(100_000)))

	_, err := srv.LiquidUnstake(ctx, msg)
	s.Require().Error(err)
}

// ---------------------------------------------------------------------------
// MsgUpdateParams
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestMsgServer_UpdateParams_Success() {
	ctx := s.ctx()
	srv := keeper.NewMsgServerImpl(s.keeper)

	newFeeRate := sdkmath.LegacyMustNewDecFromStr("0.005")
	msg := &types.MsgUpdateParams{
		Authority: govAuthority(),
		Params: types.UpdatableParams{
			UnstakeFeeRate:       newFeeRate,
			MinLiquidStakeAmount: sdkmath.NewInt(1_000),
			FeeAccountAddress:    s.addrs[0].String(),
			AutocompoundFeeRate:  sdkmath.LegacyMustNewDecFromStr("0.01"),
		},
	}

	resp, err := srv.UpdateParams(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	got := s.keeper.GetParams(ctx)
	s.Require().Equal(newFeeRate, got.UnstakeFeeRate, "UnstakeFeeRate must be updated")
	s.Require().Equal(sdkmath.NewInt(1_000), got.MinLiquidStakeAmount)
}

func (s *KeeperTestSuite) TestMsgServer_UpdateParams_InvalidAuthority() {
	ctx := s.ctx()
	srv := keeper.NewMsgServerImpl(s.keeper)

	msg := &types.MsgUpdateParams{
		Authority: s.addrs[0].String(), // random address, not gov
		Params:    types.UpdatableParams{UnstakeFeeRate: sdkmath.LegacyZeroDec()},
	}

	_, err := srv.UpdateParams(ctx, msg)
	s.Require().Error(err, "non-authority caller must be rejected")
}

func (s *KeeperTestSuite) TestMsgServer_UpdateParams_WhitelistAdmin() {
	ctx := s.ctx()

	// Set a WhitelistAdminAddress.
	admin := s.addrs[1]
	params := s.keeper.GetParams(ctx)
	params.WhitelistAdminAddress = admin.String()
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := &types.MsgUpdateParams{
		Authority: admin.String(), // whitelist admin should be accepted
		Params: types.UpdatableParams{
			UnstakeFeeRate:       sdkmath.LegacyMustNewDecFromStr("0.002"),
			AutocompoundFeeRate:  sdkmath.LegacyMustNewDecFromStr("0.01"),
			FeeAccountAddress:    s.addrs[0].String(),
			MinLiquidStakeAmount: sdkmath.NewInt(1_000),
		},
	}

	resp, err := srv.UpdateParams(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
}

// ---------------------------------------------------------------------------
// MsgUpdateWhitelistedValidators
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestMsgServer_UpdateWhitelistedValidators_Success() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()

	validators, err := sk.GetValidators(ctx, 3)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(validators), 2)

	// Build a valid whitelist: weights must sum to TotalValidatorWeight (10000).
	half := types.TotalValidatorWeight.Quo(sdkmath.NewInt(2))
	wl := []types.WhitelistedValidator{
		{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: half},
		{ValidatorAddress: validators[1].OperatorAddress, TargetWeight: types.TotalValidatorWeight.Sub(half)},
	}

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgUpdateWhitelistedValidators(
		authtypes.NewModuleAddress(govtypes.ModuleName),
		wl,
	)

	resp, err := srv.UpdateWhitelistedValidators(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	got := s.keeper.GetParams(ctx)
	s.Require().Len(got.WhitelistedValidators, 2)
}

func (s *KeeperTestSuite) TestMsgServer_UpdateWhitelistedValidators_EmptyList() {
	ctx := s.ctx()
	srv := keeper.NewMsgServerImpl(s.keeper)

	msg := types.NewMsgUpdateWhitelistedValidators(
		authtypes.NewModuleAddress(govtypes.ModuleName),
		[]types.WhitelistedValidator{},
	)

	_, err := srv.UpdateWhitelistedValidators(ctx, msg)
	s.Require().Error(err, "empty whitelist must be rejected")
	s.Require().ErrorIs(err, types.ErrWhitelistedValidatorsList)
}

func (s *KeeperTestSuite) TestMsgServer_UpdateWhitelistedValidators_WrongWeightSum() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()

	validators, err := sk.GetValidators(ctx, 2)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(validators), 2)

	// Weights don't add up to 10000.
	wl := []types.WhitelistedValidator{
		{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: sdkmath.NewInt(3000)},
		{ValidatorAddress: validators[1].OperatorAddress, TargetWeight: sdkmath.NewInt(3000)}, // sum=6000 ≠ 10000
	}

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgUpdateWhitelistedValidators(
		authtypes.NewModuleAddress(govtypes.ModuleName),
		wl,
	)

	_, err = srv.UpdateWhitelistedValidators(ctx, msg)
	s.Require().Error(err, "wrong weight sum must be rejected")
}

func (s *KeeperTestSuite) TestMsgServer_UpdateWhitelistedValidators_InvalidAuthority() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()

	validators, err := sk.GetValidators(ctx, 2)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(validators), 2)

	half := types.TotalValidatorWeight.Quo(sdkmath.NewInt(2))
	wl := []types.WhitelistedValidator{
		{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: half},
		{ValidatorAddress: validators[1].OperatorAddress, TargetWeight: types.TotalValidatorWeight.Sub(half)},
	}

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgUpdateWhitelistedValidators(s.addrs[5], wl) // random non-authority

	_, err = srv.UpdateWhitelistedValidators(ctx, msg)
	s.Require().Error(err, "non-authority caller must be rejected")
}

// ---------------------------------------------------------------------------
// MsgSetModulePaused
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestMsgServer_SetModulePaused_Pause() {
	ctx := s.ctx()
	srv := keeper.NewMsgServerImpl(s.keeper)

	msg := types.NewMsgSetModulePaused(
		authtypes.NewModuleAddress(govtypes.ModuleName),
		true,
	)

	resp, err := srv.SetModulePaused(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	params := s.keeper.GetParams(ctx)
	s.Require().True(params.ModulePaused, "module must be paused after SetModulePaused(true)")
}

func (s *KeeperTestSuite) TestMsgServer_SetModulePaused_Unpause() {
	ctx := s.ctx()

	// First pause.
	params := s.keeper.GetParams(ctx)
	params.ModulePaused = true
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgSetModulePaused(
		authtypes.NewModuleAddress(govtypes.ModuleName),
		false,
	)

	resp, err := srv.SetModulePaused(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	params = s.keeper.GetParams(ctx)
	s.Require().False(params.ModulePaused, "module must be unpaused after SetModulePaused(false)")
}

func (s *KeeperTestSuite) TestMsgServer_SetModulePaused_InvalidAuthority() {
	ctx := s.ctx()
	srv := keeper.NewMsgServerImpl(s.keeper)

	msg := types.NewMsgSetModulePaused(s.addrs[3], true) // random address

	_, err := srv.SetModulePaused(ctx, msg)
	s.Require().Error(err, "non-authority caller must be rejected")
}

func (s *KeeperTestSuite) TestMsgServer_SetModulePaused_WhitelistAdmin() {
	ctx := s.ctx()

	admin := s.addrs[2]
	params := s.keeper.GetParams(ctx)
	params.WhitelistAdminAddress = admin.String()
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgSetModulePaused(admin, true)

	resp, err := srv.SetModulePaused(ctx, msg)
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	params = s.keeper.GetParams(ctx)
	s.Require().True(params.ModulePaused)
}
