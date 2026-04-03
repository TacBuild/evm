package liquidstake

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
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
var _ *types.MsgLiquidStake

func (p Precompile) LiquidStake(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	delegatorAddr, msg, err := NewMsgLiquidStake(args, bondDenom)
	if err != nil {
		return nil, err
	}

	msgSender := contract.Caller()
	if msgSender != *delegatorAddr {
		return nil, fmt.Errorf(cmn.ErrRequesterIsNotMsgSender, msgSender.String(), delegatorAddr.String())
	}

	p.Logger(ctx).Debug(
		"tx called",
		"method", method.Name,
		"delegator_address", msg.DelegatorAddress,
		"amount", msg.Amount.String(),
	)

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	if _, err = msgSrv.LiquidStake(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) StakeToLP(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	delegatorAddr, msg, err := NewMsgStakeToLP(args, bondDenom)
	if err != nil {
		return nil, err
	}

	msgSender := contract.Caller()
	if msgSender != *delegatorAddr {
		return nil, fmt.Errorf(cmn.ErrRequesterIsNotMsgSender, msgSender.String(), delegatorAddr.String())
	}

	p.Logger(ctx).Debug(
		"tx called",
		"method", method.Name,
		"delegator_address", msg.DelegatorAddress,
		"validator_address", msg.ValidatorAddress,
		"staked_amount", msg.StakedAmount.String(),
		"liquid_amount", msg.LiquidAmount.String(),
	)

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	if _, err = msgSrv.StakeToLP(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) LiquidUnstake(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	delegatorAddr, msg, err := NewMsgLiquidUnstake(args, bondDenom)
	if err != nil {
		return nil, err
	}

	msgSender := contract.Caller()
	if msgSender != *delegatorAddr {
		return nil, fmt.Errorf(cmn.ErrRequesterIsNotMsgSender, msgSender.String(), delegatorAddr.String())
	}

	p.Logger(ctx).Debug(
		"tx called",
		"method", method.Name,
		"delegator_address", msg.DelegatorAddress,
		"amount", msg.Amount.String(),
	)

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	resp, err := msgSrv.LiquidUnstake(ctx, msg)
	if err != nil {
		return nil, err
	}

	return method.Outputs.Pack(resp.CompletionTime.Unix())
}

func (p Precompile) UpdateParams(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	msg, err := NewMsgUpdateParams(args, bondDenom, contract.Caller())
	if err != nil {
		return nil, err
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	if _, err = msgSrv.UpdateParams(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) UpdateWhitelistedValidators(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	msg, err := NewMsgUpdateWhitelistedValidators(args, bondDenom, contract.Caller())
	if err != nil {
		return nil, err
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	if _, err = msgSrv.UpdateWhitelistedValidators(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

func (p Precompile) SetModulePaused(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	msg, err := NewMsgSetModulePaused(args, bondDenom, contract.Caller())
	if err != nil {
		return nil, err
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	if _, err = msgSrv.SetModulePaused(ctx, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}
