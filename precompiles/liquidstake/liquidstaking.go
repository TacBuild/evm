package liquidstake

import (
	"embed"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/evm/x/liquidstake/keeper"
)

var _ vm.PrecompiledContract = &Precompile{}

var (
	// Embed abi json file to the executable binary. Needed when importing as dependency.
	//
	//go:embed abi.json
	f   embed.FS
	ABI abi.ABI
)

func init() {
	var err error
	ABI, err = cmn.LoadABI(f, "abi.json")
	if err != nil {
		panic(err)
	}
}

// Precompile defines the precompiled contract for liquid staking.
type Precompile struct {
	cmn.Precompile

	abi.ABI
	liquidStakeKeeper keeper.Keeper
}

// NewPrecompile creates a new liquid staking Precompile instance as a
// PrecompiledContract interface.
func NewPrecompile(
	liquidStakeKeeper keeper.Keeper,
	bankKeeper cmn.BankKeeper,
) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:           storetypes.KVGasConfig(),
			TransientKVGasConfig:  storetypes.TransientGasConfig(),
			ContractAddress:       common.HexToAddress(LiquidStakingPrecompileAddress),
			BalanceHandlerFactory: cmn.NewBalanceHandlerFactory(bankKeeper),
		},
		ABI:               ABI,
		liquidStakeKeeper: liquidStakeKeeper,
	}
}

// RequiredGas returns the required bare minimum gas to execute the precompile.
func (p Precompile) RequiredGas(input []byte) uint64 {
	// NOTE: This check avoid panicking when trying to decode the method ID
	if len(input) < 4 {
		return 0
	}

	methodID := input[:4]

	method, err := p.MethodById(methodID)
	if err != nil {
		// This should never happen since this method is going to fail during Run
		return 0
	}

	return p.Precompile.RequiredGas(input, p.IsTransaction(method))
}

// Run executes the precompiled contract liquid staking methods defined in the ABI.
func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, evm.StateDB, contract, readonly)
	})
}

// Execute dispatches the precompile method call.
func (p Precompile) Execute(ctx sdk.Context, stateDB vm.StateDB, contract *vm.Contract, readOnly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	var bz []byte

	switch method.Name {
	// Transactions
	case LiquidStakeMethod:
		bz, err = p.LiquidStake(ctx, contract, stateDB, method, args)
	case StakeToLPMethod:
		bz, err = p.StakeToLP(ctx, contract, stateDB, method, args)
	case LiquidUnstakeMethod:
		bz, err = p.LiquidUnstake(ctx, contract, stateDB, method, args)
	case UpdateParamsMethod:
		bz, err = p.UpdateParams(ctx, contract, stateDB, method, args)
	case UpdateWhitelistedValidatorsMethod:
		bz, err = p.UpdateWhitelistedValidators(ctx, contract, stateDB, method, args)
	case SetModulePausedMethod:
		bz, err = p.SetModulePaused(ctx, contract, stateDB, method, args)
	// Queries
	case ParamsMethod:
		bz, err = p.Params(ctx, contract, method, args)
	case LiquidValidatorsMethod:
		bz, err = p.LiquidValidators(ctx, contract, method, args)
	case StatesMethod:
		bz, err = p.States(ctx, contract, method, args)
	default:
		return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
	}

	return bz, err
}

// IsTransaction checks if the given method name corresponds to a transaction or query.
func (Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case LiquidStakeMethod,
		StakeToLPMethod,
		LiquidUnstakeMethod,
		UpdateParamsMethod,
		UpdateWhitelistedValidatorsMethod,
		SetModulePausedMethod:
		return true
	default:
		return false
	}
}

// Logger returns a precompile-specific logger.
func (p Precompile) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("evm extension", "liquidstake")
}
