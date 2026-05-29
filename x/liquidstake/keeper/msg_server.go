package keeper

// DONTCOVER

// Although written in msg_server_test.go, it is approached at the keeper level rather than at the msgServer level
// so is not included in the coverage.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	normal_errors "errors"

	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/evm/x/liquidstake/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the liquidstake MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

func (k msgServer) LiquidStake(goCtx context.Context, msg *types.MsgLiquidStake) (*types.MsgLiquidStakeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	gTACMintAmount, err := k.Keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, msg.GetDelegator(), msg.Amount)
	if err != nil {
		return nil, err
	}

	var cValue math.LegacyDec
	if gTACMintAmount.IsPositive() {
		cValue = gTACMintAmount.ToLegacyDec().Quo(msg.Amount.Amount.ToLegacyDec())
	}

	liquidBondDenom := k.LiquidBondDenom(ctx)
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
		sdk.NewEvent(
			types.EventTypeMsgLiquidStake,
			sdk.NewAttribute(types.AttributeKeyDelegator, msg.DelegatorAddress),
			sdk.NewAttribute(types.AttributeKeyLiquidAmount, msg.Amount.String()),
			sdk.NewAttribute(types.AttributeKeyGTACMintedAmount, sdk.Coin{Denom: liquidBondDenom, Amount: gTACMintAmount}.String()),
			sdk.NewAttribute(types.AttributeKeyCValue, cValue.String()),
		),
	})
	return &types.MsgLiquidStakeResponse{MintedAmount: gTACMintAmount}, nil
}

func (k msgServer) LiquidUnstake(goCtx context.Context, msg *types.MsgLiquidUnstake) (*types.MsgLiquidUnstakeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	completionTime, unbondingAmount, _, unbondedAmount, err := k.Keeper.LiquidUnstake(ctx, types.LiquidStakeProxyAcc, msg.GetDelegator(), msg.Amount)
	if err != nil {
		return nil, err
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
		sdk.NewEvent(
			types.EventTypeMsgLiquidUnstake,
			sdk.NewAttribute(types.AttributeKeyDelegator, msg.DelegatorAddress),
			sdk.NewAttribute(types.AttributeKeyUnstakeAmount, msg.Amount.String()),
			sdk.NewAttribute(types.AttributeKeyUnbondingAmount, sdk.Coin{Denom: bondDenom, Amount: unbondingAmount}.String()),
			sdk.NewAttribute(types.AttributeKeyUnbondedAmount, sdk.Coin{Denom: bondDenom, Amount: unbondedAmount}.String()),
			sdk.NewAttribute(types.AttributeKeyCompletionTime, completionTime.Format(time.RFC3339)),
		),
	})
	return &types.MsgLiquidUnstakeResponse{
		CompletionTime: completionTime,
		BurnedAmount:   msg.Amount.Amount,
	}, nil
}

func (k msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Authority != k.authority && msg.Authority != k.GetParams(ctx).WhitelistAdminAddress {
		return nil, errors.Wrapf(sdkerrors.ErrorInvalidSigner, "invalid authority; expected %s, got %s", k.authority, msg.Authority)
	}

	paramsToSet := k.GetParams(ctx)

	// List of all updateable param
	paramsToSet.UnstakeFeeRate = msg.Params.UnstakeFeeRate
	paramsToSet.MinLiquidStakeAmount = msg.Params.MinLiquidStakeAmount
	paramsToSet.CwLockedPoolAddress = msg.Params.CwLockedPoolAddress
	paramsToSet.FeeAccountAddress = msg.Params.FeeAccountAddress
	paramsToSet.AutocompoundFeeRate = msg.Params.AutocompoundFeeRate
	paramsToSet.WhitelistAdminAddress = msg.Params.WhitelistAdminAddress

	err := k.SetParams(ctx, paramsToSet)
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
		sdk.NewEvent(
			types.EventTypeMsgUpdateParams,
			sdk.NewAttribute(types.AttributeKeyAuthority, msg.Authority),
			sdk.NewAttribute(types.AttributeKeyUpdatedParams, msg.Params.String()),
		),
	})

	return &types.MsgUpdateParamsResponse{}, nil
}

func (k msgServer) UpdateWhitelistedValidators(goCtx context.Context, msg *types.MsgUpdateWhitelistedValidators) (*types.MsgUpdateWhitelistedValidatorsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params := k.GetParams(ctx)

	if msg.Authority != k.authority && msg.Authority != params.WhitelistAdminAddress {
		return nil, errors.Wrapf(sdkerrors.ErrorInvalidSigner, "invalid authority; expected %s, got %s", params.WhitelistAdminAddress, msg.Authority)
	}

	totalWeight := math.NewInt(0)
	for _, val := range msg.WhitelistedValidators {
		totalWeight = totalWeight.Add(val.TargetWeight)

		valAddr := val.GetValidatorAddress()
		fullVal, err := k.stakingKeeper.GetValidator(ctx, valAddr)
		if err != nil {
			return nil, normal_errors.Join(
				errors.Wrapf(
					types.ErrWhitelistedValidatorsList,
					"validator not found: %s", valAddr,
				),
				err,
			)
		}

		if fullVal.Status != stakingtypes.Bonded {
			return nil, errors.Wrapf(
				types.ErrWhitelistedValidatorsList,
				"validator status %s: expected %s; got %s", valAddr, stakingtypes.Bonded.String(), fullVal.Status.String(),
			)
		}
	}

	if len(msg.WhitelistedValidators) == 0 {
		return nil, errors.Wrap(types.ErrWhitelistedValidatorsList, "whitelisted validators list cannot be empty")
	}
	if !totalWeight.Equal(types.TotalValidatorWeight) {
		return nil, errors.Wrapf(
			types.ErrWhitelistedValidatorsList,
			"weights don't add up; expected %s, got %s", types.TotalValidatorWeight.String(), totalWeight.String(),
		)
	}
	params.WhitelistedValidators = msg.WhitelistedValidators

	err := k.SetParams(ctx, params)
	if err != nil {
		return nil, err
	}

	updatedValidatorsListJSON, _ := json.Marshal(msg.WhitelistedValidators)

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
		sdk.NewEvent(
			types.EventTypeMsgUpdateWhitelistedValidators,
			sdk.NewAttribute(types.AttributeKeyAuthority, msg.Authority),
			sdk.NewAttribute(types.AttributeKeyUpdatedWhitelistedValidators, string(updatedValidatorsListJSON)),
		),
	})

	return &types.MsgUpdateWhitelistedValidatorsResponse{}, nil
}

func (k msgServer) SetModulePaused(goCtx context.Context, msg *types.MsgSetModulePaused) (*types.MsgSetModulePausedResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params := k.GetParams(ctx)

	if msg.Authority != k.authority && msg.Authority != params.WhitelistAdminAddress {
		return nil, errors.Wrapf(sdkerrors.ErrorInvalidSigner, "invalid authority; expected %s, got %s", params.WhitelistAdminAddress, msg.Authority)
	}

	params.ModulePaused = msg.IsPaused

	err := k.SetParams(ctx, params)
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
		sdk.NewEvent(
			types.EventTypeMsgSetModulePaused,
			sdk.NewAttribute(types.AttributeKeyAuthority, msg.Authority),
			sdk.NewAttribute(types.AttributeKeyModulePaused, fmt.Sprintf("%t", msg.IsPaused)),
		),
	})

	return &types.MsgSetModulePausedResponse{}, nil
}
