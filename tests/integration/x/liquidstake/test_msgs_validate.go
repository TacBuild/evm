package liquidstake

// test_msgs_validate.go contains pure unit tests for ValidateBasic() on all
// liquidstake message types. These tests do NOT need a running chain — they
// exercise only the types package validation logic.

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/evm/x/liquidstake/types"
)

// ---------------------------------------------------------------------------
// MsgLiquidStake
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestValidateBasic_LiquidStake_Valid() {
	msg := types.NewMsgLiquidStake(
		s.addrs[0],
		sdk.NewCoin("aatom", sdkmath.NewInt(1_000_000)),
	)
	s.Require().NoError(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_LiquidStake_ZeroAmount() {
	msg := types.NewMsgLiquidStake(
		s.addrs[0],
		sdk.NewCoin("aatom", sdkmath.ZeroInt()),
	)
	s.Require().Error(msg.ValidateBasic(), "zero amount must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_LiquidStake_BadAddress() {
	msg := &types.MsgLiquidStake{
		DelegatorAddress: "not-a-bech32",
		Amount:           sdk.NewCoin("aatom", sdkmath.NewInt(1_000)),
	}
	s.Require().Error(msg.ValidateBasic(), "invalid address must be rejected")
}

// ---------------------------------------------------------------------------
// MsgLiquidUnstake
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestValidateBasic_LiquidUnstake_Valid() {
	msg := types.NewMsgLiquidUnstake(
		s.addrs[0],
		sdk.NewCoin("gtac", sdkmath.NewInt(500_000)),
	)
	s.Require().NoError(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_LiquidUnstake_ZeroAmount() {
	msg := types.NewMsgLiquidUnstake(
		s.addrs[0],
		sdk.NewCoin("gtac", sdkmath.ZeroInt()),
	)
	s.Require().Error(msg.ValidateBasic(), "zero amount must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_LiquidUnstake_BadAddress() {
	msg := &types.MsgLiquidUnstake{
		DelegatorAddress: "bad!!addr",
		Amount:           sdk.NewCoin("gtac", sdkmath.NewInt(1_000)),
	}
	s.Require().Error(msg.ValidateBasic())
}

// ---------------------------------------------------------------------------
// MsgStakeToLP
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestValidateBasic_StakeToLP_Valid() {
	valAddr := sdk.ValAddress(s.addrs[0])
	msg := types.NewMsgStakeToLP(
		s.addrs[1],
		valAddr,
		sdk.NewCoin("aatom", sdkmath.NewInt(200_000)),
		sdk.Coin{}, // no liquid amount
	)
	s.Require().NoError(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_StakeToLP_ZeroStakedAmount() {
	valAddr := sdk.ValAddress(s.addrs[0])
	msg := types.NewMsgStakeToLP(
		s.addrs[1],
		valAddr,
		sdk.NewCoin("aatom", sdkmath.ZeroInt()),
		sdk.Coin{},
	)
	s.Require().Error(msg.ValidateBasic(), "zero staked amount must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_StakeToLP_BadDelegatorAddress() {
	valAddr := sdk.ValAddress(s.addrs[0])
	msg := &types.MsgStakeToLP{
		DelegatorAddress: "bad-addr",
		ValidatorAddress: valAddr.String(),
		StakedAmount:     sdk.NewCoin("aatom", sdkmath.NewInt(100_000)),
	}
	s.Require().Error(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_StakeToLP_BadValidatorAddress() {
	msg := &types.MsgStakeToLP{
		DelegatorAddress: s.addrs[1].String(),
		ValidatorAddress: "not-a-val-addr",
		StakedAmount:     sdk.NewCoin("aatom", sdkmath.NewInt(100_000)),
	}
	s.Require().Error(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_StakeToLP_WithLiquidAmount() {
	valAddr := sdk.ValAddress(s.addrs[0])
	msg := types.NewMsgStakeToLP(
		s.addrs[1],
		valAddr,
		sdk.NewCoin("aatom", sdkmath.NewInt(200_000)),
		sdk.NewCoin("aatom", sdkmath.NewInt(100_000)),
	)
	s.Require().NoError(msg.ValidateBasic(), "valid liquid amount must be accepted")
}

// ---------------------------------------------------------------------------
// MsgUpdateParams
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestValidateBasic_UpdateParams_Valid() {
	msg := &types.MsgUpdateParams{
		Authority: s.addrs[0].String(),
		Params: types.UpdatableParams{
			UnstakeFeeRate:       sdkmath.LegacyZeroDec(),
			AutocompoundFeeRate:  sdkmath.LegacyMustNewDecFromStr("0.01"),
			MinLiquidStakeAmount: sdkmath.NewInt(1_000),
			FeeAccountAddress:    s.addrs[1].String(),
		},
	}
	s.Require().NoError(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_UpdateParams_BadAuthority() {
	msg := &types.MsgUpdateParams{
		Authority: "invalid!!",
		Params: types.UpdatableParams{
			UnstakeFeeRate:      sdkmath.LegacyZeroDec(),
			AutocompoundFeeRate: sdkmath.LegacyZeroDec(),
		},
	}
	s.Require().Error(msg.ValidateBasic(), "invalid authority must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_UpdateParams_NegativeFeeRate() {
	msg := &types.MsgUpdateParams{
		Authority: s.addrs[0].String(),
		Params: types.UpdatableParams{
			UnstakeFeeRate:       sdkmath.LegacyMustNewDecFromStr("-0.01"),
			AutocompoundFeeRate:  sdkmath.LegacyZeroDec(),
			MinLiquidStakeAmount: sdkmath.NewInt(1_000),
			FeeAccountAddress:    s.addrs[1].String(),
		},
	}
	s.Require().Error(msg.ValidateBasic(), "negative fee rate must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_UpdateParams_FeeRateAboveOne() {
	msg := &types.MsgUpdateParams{
		Authority: s.addrs[0].String(),
		Params: types.UpdatableParams{
			UnstakeFeeRate:       sdkmath.LegacyMustNewDecFromStr("1.5"),
			AutocompoundFeeRate:  sdkmath.LegacyZeroDec(),
			MinLiquidStakeAmount: sdkmath.NewInt(1_000),
			FeeAccountAddress:    s.addrs[1].String(),
		},
	}
	s.Require().Error(msg.ValidateBasic(), "fee rate > 1 must be rejected")
}

// ---------------------------------------------------------------------------
// MsgUpdateWhitelistedValidators
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestValidateBasic_UpdateWhitelistedValidators_Valid() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	validators, err := sk.GetValidators(ctx, 2)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(validators), 2)

	half := types.TotalValidatorWeight.Quo(sdkmath.NewInt(2))
	msg := types.NewMsgUpdateWhitelistedValidators(
		s.addrs[0],
		[]types.WhitelistedValidator{
			{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: half},
			{ValidatorAddress: validators[1].OperatorAddress, TargetWeight: types.TotalValidatorWeight.Sub(half)},
		},
	)
	s.Require().NoError(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_UpdateWhitelistedValidators_BadAuthority() {
	msg := types.NewMsgUpdateWhitelistedValidators(
		sdk.AccAddress("bad"),
		[]types.WhitelistedValidator{},
	)
	// "bad" is only 3 bytes — AccAddressFromBech32 will fail.
	// Construct the message manually with an invalid bech32 string.
	msg.Authority = "not-bech32!!"
	s.Require().Error(msg.ValidateBasic(), "invalid authority must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_UpdateWhitelistedValidators_InvalidValAddress() {
	msg := types.NewMsgUpdateWhitelistedValidators(
		s.addrs[0],
		[]types.WhitelistedValidator{
			{ValidatorAddress: "bad-val-address", TargetWeight: sdkmath.NewInt(10000)},
		},
	)
	s.Require().Error(msg.ValidateBasic(), "invalid validator address must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_UpdateWhitelistedValidators_ZeroWeight() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	validators, err := sk.GetValidators(ctx, 1)
	s.Require().NoError(err)
	s.Require().NotEmpty(validators)

	msg := types.NewMsgUpdateWhitelistedValidators(
		s.addrs[0],
		[]types.WhitelistedValidator{
			{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: sdkmath.ZeroInt()},
		},
	)
	s.Require().Error(msg.ValidateBasic(), "zero weight must be rejected")
}

func (s *KeeperTestSuite) TestValidateBasic_UpdateWhitelistedValidators_DuplicateValidator() {
	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	validators, err := sk.GetValidators(ctx, 1)
	s.Require().NoError(err)
	s.Require().NotEmpty(validators)

	msg := types.NewMsgUpdateWhitelistedValidators(
		s.addrs[0],
		[]types.WhitelistedValidator{
			{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: sdkmath.NewInt(5000)},
			{ValidatorAddress: validators[0].OperatorAddress, TargetWeight: sdkmath.NewInt(5000)},
		},
	)
	s.Require().Error(msg.ValidateBasic(), "duplicate validator must be rejected")
}

// ---------------------------------------------------------------------------
// MsgSetModulePaused
// ---------------------------------------------------------------------------

func (s *KeeperTestSuite) TestValidateBasic_SetModulePaused_Valid() {
	msg := types.NewMsgSetModulePaused(s.addrs[0], true)
	s.Require().NoError(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_SetModulePaused_Unpause() {
	msg := types.NewMsgSetModulePaused(s.addrs[0], false)
	s.Require().NoError(msg.ValidateBasic())
}

func (s *KeeperTestSuite) TestValidateBasic_SetModulePaused_BadAuthority() {
	msg := &types.MsgSetModulePaused{
		Authority: "not-valid!!",
		IsPaused:  true,
	}
	s.Require().Error(msg.ValidateBasic(), "invalid authority must be rejected")
}
