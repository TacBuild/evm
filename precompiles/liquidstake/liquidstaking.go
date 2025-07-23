package liquidstake

import (
	"embed"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	"github.com/cosmos/evm/x/liquidstake/keeper"
)

var _ vm.PrecompiledContract = &Precompile{}

// Embed abi json file to the executable binary. Needed when importing as dependency.
//
//go:embed abi.json
var f embed.FS

// Precompile defines the precompiled contract for staking.
type Precompile struct {
	cmn.Precompile
	liquidStakeKeeper keeper.Keeper
}

// LoadABI loads the staking ABI from the embedded abi.json file
// for the staking precompile.
func LoadABI() (abi.ABI, error) {
	return cmn.LoadABI(f, "abi.json")
}

// NewPrecompile creates a new staking Precompile instance as a
// PrecompiledContract interface.
func NewPrecompile(
	liquidStakeKeeper keeper.Keeper,
	authzKeeper authzkeeper.Keeper,
) (*Precompile, error) {
	abi, err := LoadABI()
	if err != nil {
		return nil, err
	}

	p := &Precompile{
		Precompile: cmn.Precompile{
			ABI:                  abi,
			AuthzKeeper:          authzKeeper,
			KvGasConfig:          storetypes.KVGasConfig(),
			TransientKVGasConfig: storetypes.TransientGasConfig(),
			ApprovalExpiration:   cmn.DefaultExpirationDuration, // should be configurable in the future.
		},
		liquidStakeKeeper: liquidStakeKeeper,
	}
	// SetAddress defines the address of the staking precompiled contract.
	p.SetAddress(common.HexToAddress(LiquidStakingPrecompileAddress))

	return p, nil
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

// Run executes the precompiled contract staking methods defined in the ABI.
func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readOnly bool) (bz []byte, err error) {
	ctx, stateDB, snapshot, method, initialGas, args, err := p.RunSetup(evm, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	// This handles any out of gas errors that may occur during the execution of a precompile tx or query.
	// It avoids panics and returns the out of gas error so the EVM can continue gracefully.
	defer cmn.HandleGasError(ctx, contract, initialGas, &err, stateDB, snapshot)()

	return p.RunAtomic(
		snapshot,
		stateDB,
		func() ([]byte, error) {
			switch method.Name {
			// Transactions
			case LiquidStakeMethod:
				bz, err = p.LiquidStake(ctx, evm.Origin, contract, stateDB, method, args)
			case StakeToLPMethod:
				bz, err = p.StakeToLP(ctx, evm.Origin, contract, stateDB, method, args)
			case LiquidUnstakeMethod:
				bz, err = p.LiquidUnstake(ctx, evm.Origin, contract, stateDB, method, args)
			case UpdateParamsMethod:
				bz, err = p.UpdateParams(ctx, evm.Origin, contract, stateDB, method, args)
			case UpdateWhitelistedValidatorsMethod:
				bz, err = p.UpdateWhitelistedValidators(ctx, evm.Origin, contract, stateDB, method, args)
			case SetModulePausedMethod:
				bz, err = p.SetModulePaused(ctx, evm.Origin, contract, stateDB, method, args)

			// Authorization transactions
			case authorization.ApproveMethod:
				bz, err = p.Approve(ctx, evm.Origin, stateDB, method, args)
			case authorization.RevokeMethod:
				bz, err = p.Revoke(ctx, evm.Origin, stateDB, method, args)
			case authorization.IncreaseAllowanceMethod:
				bz, err = p.IncreaseAllowance(ctx, evm.Origin, stateDB, method, args)
			case authorization.DecreaseAllowanceMethod:
				bz, err = p.DecreaseAllowance(ctx, evm.Origin, stateDB, method, args)
			// Authorization queries
			case authorization.AllowanceMethod:
				bz, err = p.Allowance(ctx, method, contract, args)
			}

			if err != nil {
				return nil, err
			}

			cost := ctx.GasMeter().GasConsumed() - initialGas

			if !contract.UseGas(cost) {
				return nil, vm.ErrOutOfGas
			}

			if err := p.AddJournalEntries(stateDB, snapshot); err != nil {
				return nil, err
			}

			return bz, nil
		},
	)
}

// IsTransaction checks if the given method name corresponds to a transaction or query.
func (Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case // tx
		LiquidStakeMethod,
		StakeToLPMethod,
		LiquidUnstakeMethod,
		UpdateParamsMethod,
		UpdateWhitelistedValidatorsMethod,
		SetModulePausedMethod:
		return true
	case // tx auth
		authorization.ApproveMethod,
		authorization.RevokeMethod,
		authorization.IncreaseAllowanceMethod,
		authorization.DecreaseAllowanceMethod:
		return true
	default:
		return false
	}
}

// Logger returns a precompile-specific logger.
func (p Precompile) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("evm extension", "staking")
}
