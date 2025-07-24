package liquidstake

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
	liquidstakekeeper "github.com/cosmos/evm/x/liquidstake/keeper"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
)

const (
	ParamsMethod           = "params"
	LiquidValidatorsMethod = "liquidValidators"
	StatesMethod           = "states"
)

// Params returns the liquidstake module parameters.
func (p Precompile) Params(
	ctx sdk.Context,
	_ *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 0, len(args))
	}

	queryServer := liquidstakekeeper.Querier{Keeper: p.liquidStakeKeeper}

	res, err := queryServer.Params(ctx, &liquidstaketypes.QueryParamsRequest{})
	if err != nil {
		return nil, err
	}

	paramsOutput := NewLiquidStakeParamsOutput(&res.Params)

	return method.Outputs.Pack(paramsOutput)
}

// LiquidValidators returns all liquid validators.
func (p Precompile) LiquidValidators(
	ctx sdk.Context,
	_ *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 0, len(args))
	}

	queryServer := liquidstakekeeper.Querier{Keeper: p.liquidStakeKeeper}

	res, err := queryServer.LiquidValidators(ctx, &liquidstaketypes.QueryLiquidValidatorsRequest{})
	if err != nil {
		return nil, err
	}

	return PackLiquidValidatorOutputs(res.LiquidValidators, method.Outputs)
}

// States returns the liquidstake module states.
func (p Precompile) States(
	ctx sdk.Context,
	_ *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 0, len(args))
	}

	queryServer := liquidstakekeeper.Querier{Keeper: p.liquidStakeKeeper}

	res, err := queryServer.States(ctx, &liquidstaketypes.QueryStatesRequest{})
	if err != nil {
		return nil, err
	}

	statesOutput := NewNetAmount(&res.NetAmountState)

	return method.Outputs.Pack(statesOutput)
}
