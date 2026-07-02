package keeper

import (
	"fmt"

	"github.com/cosmos/evm/utils"
	"github.com/cosmos/evm/x/erc20/types"
	transferkeeper "github.com/cosmos/ibc-go/v10/modules/apps/transfer/keeper"

	"cosmossdk.io/core/address"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Keeper of this module maintains collections of erc20.
type Keeper struct {
	storeKey storetypes.StoreKey
	cdc      codec.BinaryCodec
	// the address capable of executing a MsgUpdateParams message. Typically, this should be the x/gov module account.
	authority sdk.AccAddress
	addrCodec address.Codec

	accountKeeper  types.AccountKeeper
	bankKeeper     types.BankKeeper
	evmKeeper      types.EVMKeeper
	stakingKeeper  types.StakingKeeper
	transferKeeper *transferkeeper.Keeper

	// legacyPrecompiles, when set, enables reading the pre-migration precompile
	// address lists for historical heights below the migration height (see
	// IsNativePrecompileAvailable / IsDynamicPrecompileAvailable). It is nil by
	// default so chains that never migrated the precompile key format are
	// unaffected.
	legacyPrecompiles *utils.LazyUpgradeHeight
}

// SetLegacyPrecompilesHeightResolver enables historical lookups of ERC20
// precompile availability for state written before the precompile key-format
// migration. resolve must return the height at which this chain migrated the
// precompile store layout (0 before it is applied); below that height the legacy
// concatenated-blob keys are consulted. Passing nil disables the behavior.
func (k *Keeper) SetLegacyPrecompilesHeightResolver(resolve func(ctx sdk.Context) int64) *Keeper {
	k.legacyPrecompiles = utils.NewLazyUpgradeHeight(resolve)
	return k
}

// PrimeLegacyPrecompilesHeight resolves and caches the migration height from the
// given (current-height) context. Call it from a per-block hook so the cache is
// warmed from live state where the applied upgrade height is visible; unlike
// GetParams, precompile-availability checks are not hit every block, so the
// cache would otherwise stay cold until a historical query resolves it as 0.
func (k Keeper) PrimeLegacyPrecompilesHeight(ctx sdk.Context) {
	k.legacyPrecompiles.Height(ctx)
}

// NewKeeper creates new instances of the erc20 Keeper
func NewKeeper(
	storeKey storetypes.StoreKey,
	cdc codec.BinaryCodec,
	authority sdk.AccAddress,
	ak types.AccountKeeper,
	bk types.BankKeeper,
	evmKeeper types.EVMKeeper,
	sk types.StakingKeeper,
	transferKeeper *transferkeeper.Keeper,
) Keeper {
	// ensure gov module account is set and is not nil
	if err := sdk.VerifyAddressFormat(authority); err != nil {
		panic(err)
	}

	return Keeper{
		authority:      authority,
		storeKey:       storeKey,
		cdc:            cdc,
		accountKeeper:  ak,
		bankKeeper:     bk,
		evmKeeper:      evmKeeper,
		stakingKeeper:  sk,
		transferKeeper: transferKeeper,
		addrCodec:      ak.AddressCodec(),
	}
}

// Logger returns a module-specific logger.
func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}
