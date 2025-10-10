package types

import (
	context "context"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

// TODO: Revisit this once we have propoer gas fee framework.
// Tracking issues https://github.com/cosmos/cosmos-sdk/issues/9054, https://github.com/cosmos/cosmos-sdk/discussions/9072
const gasCostPerIteration = uint64(10)

var _ authz.Authorization = &LiquidStakeAuthorization{}

// NewStakeAuthorization creates a new StakeAuthorization object.
func NewStakeAuthorization(authzType AuthorizationType, amount *sdk.Coin, minLiquidAmpunt *sdk.Coin, Validator *sdk.ValAddress) (*LiquidStakeAuthorization, error) {
	a := LiquidStakeAuthorization{}

	if amount != nil {
		a.MaxTokens = amount
	}

	a.AuthorizationType = authzType

	if authzType == AuthorizationType_AUTHORIZATION_TYPE_STAKE_TO_LP && Validator != nil {
		a.Validator = string(*Validator)
		if minLiquidAmpunt != nil {
			a.MinLiquidTokens = minLiquidAmpunt
		}
	}

	return &a, nil
}

// MsgTypeURL implements Authorization.MsgTypeURL.
func (a LiquidStakeAuthorization) MsgTypeURL() string {
	authzType, err := normalizeAuthzType(a.AuthorizationType)
	if err != nil {
		panic(err)
	}

	return authzType
}

// ValidateBasic performs a stateless validation of the fields.
// It fails if MaxTokens is either undefined or negative or if the authorization
// is unspecified.
func (a LiquidStakeAuthorization) ValidateBasic() error {
	if a.MaxTokens != nil && a.MaxTokens.IsNegative() {
		return errorsmod.Wrapf(authz.ErrNegativeMaxTokens, "negative coin amount: %v", a.MaxTokens)
	}

	if a.AuthorizationType == AuthorizationType_AUTHORIZATION_TYPE_UNSPECIFIED {
		return authz.ErrUnknownAuthorizationType
	}

	return nil
}

// Accept implements Authorization.Accept. It checks, that the validator is not in the denied list,
// and, should the allowed list not be empty, if the validator is in the allowed list.
// If these conditions are met, the authorization amount is validated and if successful, the
// corresponding AcceptResponse is returned.
func (a LiquidStakeAuthorization) Accept(ctx context.Context, msg sdk.Msg) (authz.AcceptResponse, error) {
	var (
		amount           sdk.Coin
	)

	switch msg := msg.(type) {
	case *MsgLiquidStake:
		amount = msg.Amount
	case *MsgLiquidUnstake:
		amount = msg.Amount
	case *MsgStakeToLP:
		validatorAddress := &msg.ValidatorAddress
		amount = msg.StakedAmount
		LiquidAmount := msg.LiquidAmount

		if a.Validator != *validatorAddress {
			return authz.AcceptResponse{Accept: false, Delete : false}, ErrStakeToLPFailedInvalidValidator
		}

		if a.MaxTokens != nil && amount != *a.MaxTokens {
			return authz.AcceptResponse{}, ErrStakeToLPFailedAmountNotSame
		}

		if a.MinLiquidTokens != nil {
			_, err := LiquidAmount.SafeSub(*a.MinLiquidTokens)
			if err != nil {
				return authz.AcceptResponse{}, ErrStakeToLPFailedInvalidLiquidAmount 
			}
		}

		return authz.AcceptResponse{Accept: true, Delete: true}, nil
	default:
		return authz.AcceptResponse{}, sdkerrors.ErrInvalidRequest.Wrap("unknown msg type")
	}


	if a.MaxTokens == nil {
		return authz.AcceptResponse{
			Accept: true,
			Delete: false,
			Updated: &LiquidStakeAuthorization{
				MaxTokens: nil,
				AuthorizationType: a.GetAuthorizationType(),
			},
		}, nil
	}

	limitLeft, err := a.MaxTokens.SafeSub(amount)
	if err != nil {
		return authz.AcceptResponse{}, err
	}

	if limitLeft.IsZero() {
		return authz.AcceptResponse{Accept: true, Delete: true}, nil
	}

	return authz.AcceptResponse{
		Accept: true,
		Delete: false,
		Updated: &LiquidStakeAuthorization{
			AuthorizationType: a.GetAuthorizationType(),
			MaxTokens:         &limitLeft,
		},
	}, nil
}

// Normalized Msg type URLs
func normalizeAuthzType(authzType AuthorizationType) (string, error) {
	switch authzType {
	case AuthorizationType_AUTHORIZATION_TYPE_STAKE:
		return sdk.MsgTypeURL(&MsgLiquidStake{}), nil
	case AuthorizationType_AUTHORIZATION_TYPE_UNSTAKE:
		return sdk.MsgTypeURL(&MsgLiquidUnstake{}), nil
	case AuthorizationType_AUTHORIZATION_TYPE_STAKE_TO_LP:
		return sdk.MsgTypeURL(&MsgStakeToLP{}), nil
	default:
		return "", errorsmod.Wrapf(authz.ErrUnknownAuthorizationType, "cannot normalize authz type with %T", authzType)
	}
}

