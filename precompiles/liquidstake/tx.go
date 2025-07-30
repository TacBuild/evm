package liquidstake

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/authorization"

	sdk "github.com/cosmos/cosmos-sdk/types"
	keeper "github.com/cosmos/evm/x/liquidstake/keeper"
	types "github.com/cosmos/evm/x/liquidstake/types"
)

const (
	LiquidStakeMethod                 = "liquidStake"
	StakeToLPMethod                   = "stakeToLP"
	LiquidUnstakeMethod               = "liquidUnstake"
)

// Ensure imports are used (compiler workaround)
var (
	_ *types.MsgLiquidStake
	_ *types.LiquidStakeAuthorization
)

// Authorization types for liquid staking operations
const (
	// LiquidStakeAuthz defines the authorization type for the liquidstake Stake
	LiquidStakeAuthz = types.AuthorizationType_AUTHORIZATION_TYPE_STAKE
	// LiquidUnstakeAuthz defines the authorization type for the liquidstake Unstake
	LiquidUnstakeAuthz = types.AuthorizationType_AUTHORIZATION_TYPE_UNSTAKE
	// StakeToLPAuthz defines the authorization type for the liquidstake StakeToLP
	StakeToLPAuthz = types.AuthorizationType_AUTHORIZATION_TYPE_STAKE_TO_LP
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
		authz, expiration, err := authorization.CheckAuthzAndAllowanceForLiquidStake(ctx, p.AuthzKeeper, contract.CallerAddress, common.BytesToAddress(delegatorAddr), &msg.Amount, LiquidStakeMsg)
		if err != nil {
			return nil, err
		}

		liquidAuthz, ok := authz.(*types.LiquidStakeAuthorization)
		if !ok {
			return nil, fmt.Errorf("unexpected authorization type: %T", authz)
		}

		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, common.BytesToAddress(delegatorAddr), liquidAuthz, expiration, LiquidStakeMsg, msg); err != nil {
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

//	delegatorAddr, err := sdk.AccAddressFromBech32(msg.DelegatorAddress)
//	if err != nil {
//		return nil, err
//	}

	//TODO: fix this
	if !isCallerOrigin {
		return nil, fmt.Errorf("TODO")
	}

//	if !isCallerOrigin {
//		authz, expiration, err := authorization.CheckAuthzAndAllowanceForLiquidStake(ctx, p.AuthzKeeper, contract.CallerAddress, common.BytesToAddress(delegatorAddr), &msg.StakedAmount, StakeToLPMsg)
//		if err != nil {
//			return nil, err
//		}
//
//		liquidAuthz, ok := authz.(*types.LiquidStakeAuthorization)
//		if !ok {
//			return nil, fmt.Errorf("unexpected authorization type: %T", authz)
//		}
//
//		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, common.BytesToAddress(delegatorAddr), liquidAuthz, expiration, StakeToLPMsg, msg); err != nil {
//			return nil, err
//		}
//	}

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
		authz, expiration, err := authorization.CheckAuthzAndAllowanceForLiquidStake(ctx, p.AuthzKeeper, contract.CallerAddress, common.BytesToAddress(delegatorAddr), &msg.Amount, LiquidUnstakeMsg)
		if err != nil {
			return nil, err
		}

		liquidAuthz, ok := authz.(*types.LiquidStakeAuthorization)
		if !ok {
			return nil, fmt.Errorf("unexpected authorization type: %T", authz)
		}

		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, common.BytesToAddress(delegatorAddr), liquidAuthz, expiration, LiquidUnstakeMsg, msg); err != nil {
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

