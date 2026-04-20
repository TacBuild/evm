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
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

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
			LsmDisabled:          true,
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

// ---------------------------------------------------------------------------
// MsgStakeToLP
// ---------------------------------------------------------------------------

// TestMsgServer_StakeToLP_LsmDisabled verifies that StakeToLP returns
// ErrDisabledLSM when LsmDisabled=true (the default in SetupTest).
func (s *KeeperTestSuite) TestMsgServer_StakeToLP_LsmDisabled() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	// Default setup has LsmDisabled=true – StakeToLP must immediately return ErrDisabledLSM.
	params := s.keeper.GetParams(ctx)
	s.Require().True(params.LsmDisabled, "precondition: LsmDisabled must be true")

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgStakeToLP(
		s.delAddrs[0],
		valOpers[0],
		sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000)),
		sdk.Coin{},
	)

	_, err = srv.StakeToLP(ctx, msg)
	s.Require().ErrorIs(err, types.ErrDisabledLSM)
}

// TestMsgServer_StakeToLP_ModulePaused verifies that StakeToLP respects the
// module-paused flag even when LsmDisabled=false.
func (s *KeeperTestSuite) TestMsgServer_StakeToLP_ModulePaused() {
	_, valOpers := s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	params := s.keeper.GetParams(ctx)
	params.LsmDisabled = false
	params.ModulePaused = true
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgStakeToLP(
		s.delAddrs[0],
		valOpers[0],
		sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000)),
		sdk.Coin{},
	)

	_, err = srv.StakeToLP(ctx, msg)
	s.Require().ErrorIs(err, types.ErrModulePaused)
}

// TestMsgServer_StakeToLP_NonWhitelistedValidator verifies that StakeToLP
// rejects a validator that is not in the whitelist (LSM enabled path).
func (s *KeeperTestSuite) TestMsgServer_StakeToLP_NonWhitelistedValidator() {
	// Set up 2 whitelisted validators.
	s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	// Find a validator that is NOT in the whitelist.
	allVals, err := sk.GetValidators(ctx, 5)
	s.Require().NoError(err)

	params := s.keeper.GetParams(ctx)
	wvMap := params.WhitelistedValsMap()
	var nonWhitelistedVal sdk.ValAddress
	for _, v := range allVals {
		if !wvMap.IsListed(v.OperatorAddress) {
			var parseErr error
			nonWhitelistedVal, parseErr = sdk.ValAddressFromBech32(v.OperatorAddress)
			s.Require().NoError(parseErr)
			break
		}
	}
	if nonWhitelistedVal == nil {
		s.T().Skip("all validators are whitelisted – need at least 3 validators")
	}

	// Enable LSM.
	params.LsmDisabled = false
	s.Require().NoError(s.keeper.SetParams(ctx, params))

	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgStakeToLP(
		s.delAddrs[0],
		nonWhitelistedVal,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000)),
		sdk.Coin{},
	)

	_, err = srv.StakeToLP(ctx, msg)
	s.Require().Error(err, "non-whitelisted validator must be rejected")
}

// TestMsgServer_StakeToLP_Success performs an end-to-end StakeToLP flow:
//  1. Create a new validator via CreateValidators (which also calls MsgValidatorBond
//     on the creator — that delegator's bond is a ValidatorBond and CANNOT be
//     tokenized via MsgTokenizeShares).
//  2. Create a SEPARATE fresh delegator who delegates to the same validator
//     WITHOUT calling MsgValidatorBond, so delegation.ValidatorBond = false.
//  3. Whitelist the validator, enable LSM.
//  4. Call StakeToLP from the fresh delegator.
func (s *KeeperTestSuite) TestMsgServer_StakeToLP_Success() {
	// Step 1: create a new validator. CreateValidators marks the creator's
	// delegation as ValidatorBond=true — we do NOT use that address for StakeToLP.
	_, valOpers, _ := s.CreateValidators([]int64{2_000_000})

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	s.Require().NoError(err)

	// Step 2: fresh delegator — plain delegation, ValidatorBond stays false.
	// Use s.delAddrs[0] — a pre-funded address from the suite keyring,
	// distinct from the addresses generated inside CreateValidators.
	freshDelegator := s.delAddrs[0]
	s.Require().NoError(s.nw.FundAccount(freshDelegator, sdk.NewCoins(
		sdk.NewCoin(bondDenom, sdkmath.NewInt(2_000_000)),
	)))
	val, err := sk.GetValidator(ctx, valOpers[0])
	s.Require().NoError(err)
	_, err = sk.Delegate(ctx, freshDelegator, sdkmath.NewInt(1_000_000), stakingtypes.Unbonded, val, true)
	s.Require().NoError(err)

	// Step 3: whitelist validator, enable LSM.
	params := s.keeper.GetParams(ctx)
	params.LsmDisabled = false
	params.ModulePaused = false
	params.WhitelistedValidators = []types.WhitelistedValidator{
		{ValidatorAddress: valOpers[0].String(), TargetWeight: types.TotalValidatorWeight},
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)
	// Commit so params survive; then NextBlock to refresh ctx to the committed store.
	s.Require().NoError(s.nw.CommitState())
	s.Require().NoError(s.nw.NextBlock())

	ctx = s.ctx()
	wvMap := s.keeper.GetParams(ctx).WhitelistedValsMap()
	activeVals := s.keeper.GetActiveLiquidValidators(ctx, wvMap)
	s.Require().NotEmpty(activeVals, "validator must be active before StakeToLP")

	// Step 4: StakeToLP using the fresh delegator (ValidatorBond=false → tokenizable).
	srv := keeper.NewMsgServerImpl(s.keeper)
	msg := types.NewMsgStakeToLP(
		freshDelegator,
		valOpers[0],
		sdk.NewCoin(bondDenom, sdkmath.NewInt(500_000)),
		sdk.Coin{}, // no additional liquid amount
	)

	resp, err := srv.StakeToLP(ctx, msg)
	s.Require().NoError(err, "StakeToLP must succeed with a plain bonded delegation")
	s.Require().NotNil(resp)

	// Fresh delegator must have received gTAC.
	ctx = s.ctx()
	params = s.keeper.GetParams(ctx)
	gTACBal := s.nw.App.GetBankKeeper().GetBalance(ctx, freshDelegator, params.LiquidBondDenom)
	s.Require().True(gTACBal.IsPositive(), "delegator must hold gTAC after StakeToLP")
}
