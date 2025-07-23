package liquidstake

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	liquidtypes "github.com/cosmos/evm/x/liquidstake/types"
)

var (
	// LiquidStakeMsg defines the authorization type for MsgLiquidStake
	LiquidStakeMsg = sdk.MsgTypeURL(&liquidtypes.MsgLiquidStake{})
	// StakeToLPMsg defines the authorization type for MsgStakeToLP
	StakeToLPMsg = sdk.MsgTypeURL(&liquidtypes.MsgStakeToLP{})
	// LiquidUnstakeMsg defines the authorization type for MsgLiquidUnstake
	LiquidUnstakeMsg = sdk.MsgTypeURL(&liquidtypes.MsgLiquidUnstake{})
)

// Approve sets an amount as the allowance of a grantee over the caller's tokens.
// Returns a boolean value indicating whether the operation succeeded.
func (p Precompile) Approve(
	ctx sdk.Context,
	origin common.Address,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)
	grantee, coin, typeURLs, err := authorization.CheckApprovalArgs(args, bondDenom)
	if err != nil {
		return nil, err
	}

	for _, typeURL := range typeURLs {
		switch typeURL {
		case LiquidStakeMsg, StakeToLPMsg, LiquidUnstakeMsg:
			if err = p.grantOrDeleteLiquidStakeAuthz(ctx, grantee, origin, coin, typeURL); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	return method.Outputs.Pack(true)
}

// Revoke removes the authorization grants given in the typeUrls for a given granter to a given grantee.
func (p Precompile) Revoke(
	ctx sdk.Context,
	origin common.Address,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	grantee, typeURLs, err := authorization.CheckRevokeArgs(args)
	if err != nil {
		return nil, err
	}

	for _, typeURL := range typeURLs {
		switch typeURL {
		case LiquidStakeMsg, StakeToLPMsg, LiquidUnstakeMsg:
			if err = p.AuthzKeeper.DeleteGrant(ctx, grantee.Bytes(), origin.Bytes(), typeURL); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	return method.Outputs.Pack(true)
}

// grantOrDeleteLiquidStakeAuthz grants or deletes the authorization for liquid stake operations
func (p Precompile) grantOrDeleteLiquidStakeAuthz(
	ctx sdk.Context,
	grantee, granter common.Address,
	coin *sdk.Coin,
	msgURL string,
) error {
	// If coin is nil, delete the grant
	if coin == nil {
		return p.AuthzKeeper.DeleteGrant(ctx, grantee.Bytes(), granter.Bytes(), msgURL)
	}

	// Create a StakeAuthorization with the specified amount
	stakeAuthz, err := stakingtypes.NewStakeAuthorization(
		[]sdk.ValAddress{}, // empty allowList means any validator
		nil,                // empty denyList means no restrictions
		stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_DELEGATE,
		coin,
	)
	if err != nil {
		return err
	}

	if err := stakeAuthz.ValidateBasic(); err != nil {
		return err
	}

	expiration := ctx.BlockTime().Add(p.ApprovalExpiration).UTC()
	return p.AuthzKeeper.SaveGrant(ctx, grantee.Bytes(), granter.Bytes(), stakeAuthz, &expiration)
}

// UpdateLiquidStakeAuthorization updates the liquid stake authorization based on the authz AcceptResponse
func (p Precompile) UpdateLiquidStakeAuthorization(
	ctx sdk.Context,
	grantee, granter common.Address,
	stakeAuthz *stakingtypes.StakeAuthorization,
	expiration *time.Time,
	messageType string,
	msg sdk.Msg,
) error {
	updatedResponse, err := stakeAuthz.Accept(ctx, msg)
	if err != nil {
		return err
	}

	if updatedResponse.Delete {
		err = p.AuthzKeeper.DeleteGrant(ctx, grantee.Bytes(), granter.Bytes(), messageType)
	} else {
		err = p.AuthzKeeper.SaveGrant(ctx, grantee.Bytes(), granter.Bytes(), updatedResponse.Updated, expiration)
	}

	return err
}

// IncreaseAllowance increases the allowance of grantee over the caller's tokens by the amount.
func (p Precompile) IncreaseAllowance(
	ctx sdk.Context,
	origin common.Address,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)
	grantee, coin, typeUrls, err := authorization.CheckApprovalArgs(args, bondDenom)
	if err != nil {
		return nil, err
	}

	for _, typeURL := range typeUrls {
		switch typeURL {
		case LiquidStakeMsg, StakeToLPMsg, LiquidUnstakeMsg:
			if err = p.increaseAllowance(ctx, grantee, origin, coin, typeURL); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	return method.Outputs.Pack(true)
}

// DecreaseAllowance decreases the allowance of grantee over the caller's tokens by the amount.
func (p Precompile) DecreaseAllowance(
	ctx sdk.Context,
	origin common.Address,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)
	grantee, coin, typeUrls, err := authorization.CheckApprovalArgs(args, bondDenom)
	if err != nil {
		return nil, err
	}

	for _, typeURL := range typeUrls {
		switch typeURL {
		case LiquidStakeMsg, StakeToLPMsg, LiquidUnstakeMsg:
			authzGrant, expiration, err := authorization.CheckAuthzExists(ctx, p.AuthzKeeper, grantee, origin, typeURL)
			if err != nil {
				return nil, err
			}

			stakeAuthz, ok := authzGrant.(*stakingtypes.StakeAuthorization)
			if !ok {
				return nil, errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "expected: *types.StakeAuthorization, received: %T", authzGrant)
			}

			if err = p.decreaseAllowance(ctx, grantee, origin, coin, stakeAuthz, expiration); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	return method.Outputs.Pack(true)
}

// increaseAllowance increases the allowance of spender over the caller's tokens by the amount.
func (p Precompile) increaseAllowance(
	ctx sdk.Context,
	grantee, granter common.Address,
	coin *sdk.Coin,
	msgURL string,
) error {
	// Check if the authorization exists for the given spender
	existingAuthz, expiration, err := authorization.CheckAuthzExists(ctx, p.AuthzKeeper, grantee, granter, msgURL)
	if err != nil {
		return err
	}

	// Cast the authorization to a staking authorization
	stakeAuthz, ok := existingAuthz.(*stakingtypes.StakeAuthorization)
	if !ok {
		return errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "expected: *types.StakeAuthorization, received: %T", existingAuthz)
	}

	// If the authorization has no limit, no operation is performed
	if stakeAuthz.MaxTokens == nil {
		p.Logger(ctx).Debug("increaseAllowance called with no limit (stakeAuthz.MaxTokens == nil): no-op")
		return nil
	}

	// Add the amount to the limit
	stakeAuthz.MaxTokens.Amount = stakeAuthz.MaxTokens.Amount.Add(coin.Amount)

	return p.AuthzKeeper.SaveGrant(ctx, grantee.Bytes(), granter.Bytes(), stakeAuthz, expiration)
}

// decreaseAllowance decreases the allowance of spender over the caller's tokens by the amount.
func (p Precompile) decreaseAllowance(
	ctx sdk.Context,
	grantee, granter common.Address,
	coin *sdk.Coin,
	stakeAuthz *stakingtypes.StakeAuthorization,
	expiration *time.Time,
) error {
	// If the authorization has no limit, no operation is performed
	if stakeAuthz.MaxTokens == nil {
		p.Logger(ctx).Debug("decreaseAllowance called with no limit (stakeAuthz.MaxTokens == nil): no-op")
		return nil
	}

	// If the authorization limit is less than the subtraction amount, return error
	if stakeAuthz.MaxTokens.Amount.LT(coin.Amount) {
		return fmt.Errorf("amount by which the allowance should be decreased is greater than the authorization limit: %s > %s", coin.Amount, stakeAuthz.MaxTokens.Amount)
	}

	// If amount is less than or equal to the Authorization amount, subtract the amount from the limit
	if coin.Amount.LTE(stakeAuthz.MaxTokens.Amount) {
		stakeAuthz.MaxTokens.Amount = stakeAuthz.MaxTokens.Amount.Sub(coin.Amount)
	}

	return p.AuthzKeeper.SaveGrant(ctx, grantee.Bytes(), granter.Bytes(), stakeAuthz, expiration)
}

// Allowance returns the allowance of spender over the caller's tokens.
func (p Precompile) Allowance(
	ctx sdk.Context,
	method *abi.Method,
	contract *vm.Contract,
	args []interface{},
) ([]byte, error) {
	grantee, granter, typeURL, err := authorization.CheckAllowanceArgs(args)
	if err != nil {
		return nil, err
	}

	// Check if the authorization exists for the given spender
	existingAuthz, expiration, err := authorization.CheckAuthzExists(ctx, p.AuthzKeeper, grantee, granter, typeURL)
	if err != nil {
		return nil, err
	}

	// Cast the authorization to a staking authorization
	stakeAuthz, ok := existingAuthz.(*stakingtypes.StakeAuthorization)
	if !ok {
		return nil, errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "expected: *types.StakeAuthorization, received: %T", existingAuthz)
	}

	// If the authorization has no limit, return max uint256
	if stakeAuthz.MaxTokens == nil {
		return method.Outputs.Pack(abi.MaxUint256, expiration.Unix())
	}

	return method.Outputs.Pack(stakeAuthz.MaxTokens.Amount.BigInt(), expiration.Unix())
}
