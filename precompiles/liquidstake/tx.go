package liquidstake

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"
	erc20precompile "github.com/cosmos/evm/precompiles/erc20"

	sdk "github.com/cosmos/cosmos-sdk/types"
	keeper "github.com/cosmos/evm/x/liquidstake/keeper"
	types "github.com/cosmos/evm/x/liquidstake/types"
)

const (
	LiquidStakeMethod                 = "liquidStake"
	LiquidUnstakeMethod               = "liquidUnstake"
	UpdateParamsMethod                = "updateParams"
	UpdateWhitelistedValidatorsMethod = "updateWhitelistedValidators"
	SetModulePausedMethod             = "setModulePaused"
)

// Ensure imports are used (compiler workaround)
var _ *types.MsgLiquidStake

// emitLiquidBondDenomTransferEvent resolves the ERC-20 contract for the
// liquidBondDenom from the erc20 module state and emits a Transfer event.
// This is needed because native cosmos token mints/burns do not emit ERC-20
// events automatically.
func (p Precompile) emitLiquidBondDenomTransferEvent(
	ctx sdk.Context,
	stateDB vm.StateDB,
	from, to common.Address,
	amount *big.Int,
) error {
	liquidBondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	id := p.erc20Keeper.GetTokenPairID(ctx, liquidBondDenom)
	pair, ok := p.erc20Keeper.GetTokenPair(ctx, id)
	if !ok {
		return fmt.Errorf("ERC-20 token pair not found for liquidBondDenom %s", liquidBondDenom)
	}

	contract, err := p.erc20Keeper.InstantiateERC20Precompile(ctx, pair.GetERC20Contract(), false)
	if err != nil {
		return err
	}

	erc20pc, ok := contract.(*erc20precompile.Precompile)
	if !ok {
		return fmt.Errorf("unexpected precompile type for liquidBondDenom %s", liquidBondDenom)
	}

	return erc20pc.EmitTransferEvent(ctx, stateDB, from, to, amount)
}

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

	resp, err := msgSrv.LiquidStake(ctx, msg)
	if err != nil {
		return nil, err
	}

	if err := p.emitLiquidBondDenomTransferEvent(ctx, stateDB, common.Address{}, *delegatorAddr, resp.MintedAmount.BigInt()); err != nil {
		return nil, err
	}

	if err := p.EmitLiquidStakeEvent(ctx, stateDB, msg, *delegatorAddr); err != nil {
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

	if err := p.emitLiquidBondDenomTransferEvent(ctx, stateDB, *delegatorAddr, common.Address{}, resp.BurnedAmount.BigInt()); err != nil {
		return nil, err
	}

	if err := p.EmitLiquidUnstakeEvent(ctx, stateDB, msg, *delegatorAddr); err != nil {
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

	if err := p.EmitUpdateParamsEvent(ctx, stateDB, msg); err != nil {
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

	if err := p.EmitUpdateWhitelistedValidatorEvent(ctx, stateDB, msg); err != nil {
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

	if err := p.EmitSetModulePausedEvent(ctx, stateDB, msg); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}
