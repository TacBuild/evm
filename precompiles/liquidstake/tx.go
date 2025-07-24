package liquidstake

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/authorization"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	keeper "github.com/cosmos/evm/x/liquidstake/keeper"
	types "github.com/cosmos/evm/x/liquidstake/types"
)

const (
	LiquidStakeMethod                 = "liquidStake"
	StakeToLPMethod                   = "stakeToLP"
	LiquidUnstakeMethod               = "liquidUnstake"
	UpdateParamsMethod                = "updateParams"
	UpdateWhitelistedValidatorsMethod = "updateWhitelistedValidators"
	SetModulePausedMethod             = "setModulePaused"
)

// Ensure imports are used (compiler workaround)
var (
	_ *types.MsgLiquidStake
	_ *stakingtypes.StakeAuthorization
)

// Maybe its better to just use those from staking precompile
const (
	// DelegateAuthz defines the authorization type for the staking Delegate
	DelegateAuthz = stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_DELEGATE
	// UndelegateAuthz defines the authorization type for the staking Undelegate
	UndelegateAuthz = stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_UNDELEGATE
	// RedelegateAuthz defines the authorization type for the staking Redelegate
	RedelegateAuthz = stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_REDELEGATE
	// CancelUnbondingDelegationAuthz defines the authorization type for the staking
	CancelUnbondingDelegationAuthz = stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_CANCEL_UNBONDING_DELEGATION
)

func (p Precompile) LiquidStake(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)

	if err != nil {
		return nil, err
	}

	msg, err := NewMsgLiquidStake(args, bondDenom)
	if err != nil {
		return nil, err
	}

	isCallerOrigin := contract.CallerAddress == origin

	delegatorAddr, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}

	if !isCallerOrigin {
		stakeAuthz, expiration, err := authorization.CheckAuthzAndAllowanceForGranter(ctx, p.AuthzKeeper, contract.CallerAddress, common.BytesToAddress(delegatorAddr), &msg.Amount, LiquidStakeMsg)
		if err != nil {
			return nil, err
		}
		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, common.BytesToAddress(delegatorAddr), stakeAuthz, expiration, LiquidStakeMsg, msg); err != nil {
			return nil, err
		}
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	if _, err = msgSrv.LiquidStake(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) StakeToLP(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	liquidBondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	msg, err := NewMsgStakeToLP(args, liquidBondDenom, bondDenom)
	if err != nil {
		return nil, err
	}

	isCallerOrigin := contract.CallerAddress == origin

	delegatorAddr, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}

	if !isCallerOrigin {
		stakeAuthz, expiration, err := authorization.CheckAuthzAndAllowanceForGranter(ctx, p.AuthzKeeper, contract.CallerAddress, common.BytesToAddress(delegatorAddr), &msg.StakedAmount, StakeToLPMsg)
		if err != nil {
			return nil, err
		}
		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, common.BytesToAddress(delegatorAddr), stakeAuthz, expiration, StakeToLPMsg, msg); err != nil {
			return nil, err
		}
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	if _, err = msgSrv.StakeToLP(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) LiquidUnstake(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	msg, err := NewMsgLiquidUnstake(args, bondDenom)
	if err != nil {
		return nil, err
	}

	isCallerOrigin := contract.CallerAddress == origin

	delegatorAddr, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
	if err != nil {
		return nil, err
	}

	if !isCallerOrigin {
		stakeAuthz, expiration, err := authorization.CheckAuthzAndAllowanceForGranter(ctx, p.AuthzKeeper, contract.CallerAddress, common.BytesToAddress(delegatorAddr), &msg.Amount, LiquidUnstakeMsg)
		if err != nil {
			return nil, err
		}
		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, common.BytesToAddress(delegatorAddr), stakeAuthz, expiration, LiquidUnstakeMsg, msg); err != nil {
			return nil, err
		}
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	responce, err := msgSrv.LiquidUnstake(ctx, msg)
	if err != nil {
		return nil, err
	}

	return method.Outputs.Pack(responce.CompletionTime.Unix())
}

func (p Precompile) UpdateParams(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	msg, err := NewMsgUpdateParams(args, bondDenom)
	if err != nil {
		return nil, err
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	if _, err = msgSrv.UpdateParams(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) UpdateWhitelistedValidators(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	msg, err := NewMsgUpdateWhitelistedValidators(args, bondDenom)
	if err != nil {
		return nil, err
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	if _, err = msgSrv.UpdateWhitelistedValidators(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) SetModulePaused(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	msg, err := NewMsgSetModulePaused(args, bondDenom)
	if err != nil {
		return nil, err
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	if _, err = msgSrv.SetModulePaused(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}
