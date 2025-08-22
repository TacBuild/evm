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
	liquidtypes "github.com/cosmos/evm/x/liquidstake/types"
)

var (
	// LiquidStakeMsg defines the authorization type for MsgLiquidStake
	LiquidStakeMsg = sdk.MsgTypeURL(&liquidtypes.MsgLiquidStake{})
	// LiquidUnstakeMsg defines the authorization type for MsgLiquidUnstake
	LiquidUnstakeMsg = sdk.MsgTypeURL(&liquidtypes.MsgLiquidUnstake{})

	// We do not allow StakeToLp delegation through smart-contract,
	// because current authorization interface doesnt provide opportunity to pass more arguments required for this delegation
	// It is still possible to delegate through native cosmos transaction, or call StakeToLP from smart-contract directly
)

func coinToLiquidCoin(coin *sdk.Coin, liquidDenom string) *sdk.Coin {
	if coin == nil {
		return nil
	}

	return &sdk.Coin{
		Denom:  liquidDenom,
		Amount: coin.Amount,
	}
}

// Approve sets an amount as the allowance of a grantee over the caller's tokens.
// Returns a boolean value indicating whether the operation succeeded.
func (p Precompile) Approve(
	ctx sdk.Context,
	caller common.Address,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	grantee, coin, typeURLs, err := authorization.CheckApprovalArgs(args, bondDenom)
	if err != nil {
		return nil, err
	}

	for _, typeURL := range typeURLs {
		switch typeURL {
		case LiquidStakeMsg:
			if err = p.grantOrDeleteLiquidStakeAuthz(ctx, grantee, caller, coin, typeURL); err != nil {
				return nil, err
			}
		case LiquidUnstakeMsg:
			LiquidCoin := coinToLiquidCoin(coin, p.liquidStakeKeeper.LiquidBondDenom(ctx))

			if err = p.grantOrDeleteLiquidStakeAuthz(ctx, grantee, caller, LiquidCoin, typeURL); err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	// TODO: do we want to emit one approval for all typeUrls, or one approval for each typeUrl?
	// NOTE: This might have gas implications as we are emitting a slice of strings
	if err := p.EmitApprovalEvent(ctx, stateDB, grantee, caller, coin, typeURLs); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

// Revoke removes the authorization grants given in the typeUrls for a given granter to a given grantee.
func (p Precompile) Revoke(
	ctx sdk.Context,
	caller common.Address,
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
		case LiquidStakeMsg, LiquidUnstakeMsg:
			if err = p.AuthzKeeper.DeleteGrant(ctx, grantee.Bytes(), caller.Bytes(), typeURL); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	// NOTE: Using the new more generic event emitter that was created
	if err = authorization.EmitRevocationEvent(cmn.EmitEventArgs{
		Ctx:            ctx,
		StateDB:        stateDB,
		ContractAddr:   p.Address(),
		ContractEvents: p.ABI.Events,
		EventData: authorization.EventRevocation{
			Granter:  caller,
			Grantee:  grantee,
			TypeUrls: typeURLs,
		},
	}); err != nil {
		return nil, err
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
	//if coin is negative or zero delete grant (nil is valid and means unlimited)
	if coin != nil && !coin.Amount.IsPositive() {
		return p.AuthzKeeper.DeleteGrant(ctx, grantee.Bytes(), granter.Bytes(), msgURL)
	}

	var authzType liquidtypes.AuthorizationType
	switch msgURL {
	case LiquidStakeMsg:
		authzType = liquidtypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE
	case LiquidUnstakeMsg:
		authzType = liquidtypes.AuthorizationType_AUTHORIZATION_TYPE_UNSTAKE
	default:
		return fmt.Errorf("invalid message type URL: %s", msgURL)
	}

	// Create a LiquidStakeAuthorization with the specified amount
	liquidAuthz, err := liquidtypes.NewStakeAuthorization(authzType, coin, nil, nil)
	if err != nil {
		return err
	}

	if err := liquidAuthz.ValidateBasic(); err != nil {
		return err
	}

	expiration := ctx.BlockTime().Add(p.ApprovalExpiration).UTC()
	return p.AuthzKeeper.SaveGrant(ctx, grantee.Bytes(), granter.Bytes(), liquidAuthz, &expiration)
}

// UpdateLiquidStakeAuthorization updates the liquid stake authorization based on the authz AcceptResponse
func (p Precompile) UpdateLiquidStakeAuthorization(
	ctx sdk.Context,
	grantee, granter common.Address,
	liquidAuthz *liquidtypes.LiquidStakeAuthorization,
	expiration *time.Time,
	messageType string,
	msg sdk.Msg,
) error {
	updatedResponse, err := liquidAuthz.Accept(ctx, msg)
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
	caller common.Address,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	grantee, coin, typeUrls, err := authorization.CheckApprovalArgs(args, bondDenom)
	if err != nil {
		return nil, err
	}

	for _, typeURL := range typeUrls {
		switch typeURL {
		case LiquidStakeMsg:
			if err = p.increaseAllowance(ctx, grantee, caller, coin, typeURL); err != nil {
				return nil, err
			}
		case LiquidUnstakeMsg:
			LiquidCoin := coinToLiquidCoin(coin, p.liquidStakeKeeper.LiquidBondDenom(ctx))

			if err = p.increaseAllowance(ctx, grantee, caller, LiquidCoin, typeURL); err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	if err := p.EmitAllowanceChangeEvent(ctx, stateDB, grantee, caller, typeUrls); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

// DecreaseAllowance decreases the allowance of grantee over the caller's tokens by the amount.
func (p Precompile) DecreaseAllowance(
	ctx sdk.Context,
	caller common.Address,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	grantee, coin, typeUrls, err := authorization.CheckApprovalArgs(args, bondDenom)
	if err != nil {
		return nil, err
	}

	for _, typeURL := range typeUrls {
		switch typeURL {
		case LiquidStakeMsg:
			authzGrant, expiration, err := authorization.CheckAuthzExists(ctx, p.AuthzKeeper, grantee, caller, typeURL)
			if err != nil {
				return nil, err
			}

			liquidAuthz, ok := authzGrant.(*liquidtypes.LiquidStakeAuthorization)
			if !ok {
				return nil, errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "expected: *types.LiquidStakeAuthorization, received: %T", authzGrant)
			}

			if err = p.decreaseAllowance(ctx, grantee, caller, coin, liquidAuthz, expiration); err != nil {
				return nil, err
			}

		case LiquidUnstakeMsg:
			LiquidCoin := coinToLiquidCoin(coin, p.liquidStakeKeeper.LiquidBondDenom(ctx))

			authzGrant, expiration, err := authorization.CheckAuthzExists(ctx, p.AuthzKeeper, grantee, caller, typeURL)
			if err != nil {
				return nil, err
			}

			liquidAuthz, ok := authzGrant.(*liquidtypes.LiquidStakeAuthorization)
			if !ok {
				return nil, errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "expected: *types.LiquidStakeAuthorization, received: %T", authzGrant)
			}

			if err = p.decreaseAllowance(ctx, grantee, caller, LiquidCoin, liquidAuthz, expiration); err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf(cmn.ErrInvalidMsgType, "liquidstake", typeURL)
		}
	}

	if err := p.EmitAllowanceChangeEvent(ctx, stateDB, grantee, caller, typeUrls); err != nil {
		return nil, err
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

	// Cast the authorization to a liquidstake authorization
	liquidAuthz, ok := existingAuthz.(*liquidtypes.LiquidStakeAuthorization)
	if !ok {
		return errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "expected: *types.LiquidStakeAuthorization, received: %T", existingAuthz)
	}

	// If the authorization has no limit, no operation is performed
	if liquidAuthz.MaxTokens == nil {
		p.Logger(ctx).Debug("increaseAllowance called with no limit (liquidAuthz.MaxTokens == nil): no-op")
		return nil
	}

	// Add the amount to the limit
	liquidAuthz.MaxTokens.Amount = liquidAuthz.MaxTokens.Amount.Add(coin.Amount)

	return p.AuthzKeeper.SaveGrant(ctx, grantee.Bytes(), granter.Bytes(), liquidAuthz, expiration)
}

// decreaseAllowance decreases the allowance of spender over the caller's tokens by the amount.
func (p Precompile) decreaseAllowance(
	ctx sdk.Context,
	grantee, granter common.Address,
	coin *sdk.Coin,
	liquidAuthz *liquidtypes.LiquidStakeAuthorization,
	expiration *time.Time,
) error {
	// If the authorization has no limit, no operation is performed
	if liquidAuthz.MaxTokens == nil {
		p.Logger(ctx).Debug("decreaseAllowance called with no limit (liquidAuthz.MaxTokens == nil): no-op")
		return nil
	}

	// If the authorization limit is less than the subtraction amount, return error
	if liquidAuthz.MaxTokens.Amount.LT(coin.Amount) {
		return fmt.Errorf("amount by which the allowance should be decreased is greater than the authorization limit: %s > %s", coin.Amount, liquidAuthz.MaxTokens.Amount)
	}

	// If amount is less than or equal to the Authorization amount, subtract the amount from the limit
	if coin.Amount.LTE(liquidAuthz.MaxTokens.Amount) {
		liquidAuthz.MaxTokens.Amount = liquidAuthz.MaxTokens.Amount.Sub(coin.Amount)
	}

	return p.AuthzKeeper.SaveGrant(ctx, grantee.Bytes(), granter.Bytes(), liquidAuthz, expiration)
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
	existingAuthz, _, err := authorization.CheckAuthzExists(ctx, p.AuthzKeeper, grantee, granter, typeURL)
	if err != nil {
		return method.Outputs.Pack(0)
	}

	// Cast the authorization to a liquidstake authorization
	liquidAuthz, ok := existingAuthz.(*liquidtypes.LiquidStakeAuthorization)
	if !ok {
		return nil, errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "expected: *types.LiquidStakeAuthorization, received: %T", existingAuthz)
	}

	// If the authorization has no limit, return max uint256
	if liquidAuthz.MaxTokens == nil {
		return method.Outputs.Pack(abi.MaxUint256)
	}

	return method.Outputs.Pack(liquidAuthz.MaxTokens.Amount.BigInt())
}
