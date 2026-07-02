package keeper

import (
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/erc20"
	"github.com/cosmos/evm/precompiles/werc20"
	"github.com/cosmos/evm/x/erc20/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type PrecompileType int

const (
	PrecompileTypeNative PrecompileType = iota
	PrecompileTypeDynamic
)

// GetERC20PrecompileInstance returns the precompile instance for the given address.
func (k Keeper) GetERC20PrecompileInstance(
	ctx sdk.Context,
	address common.Address,
) (contract vm.PrecompiledContract, found bool, err error) {
	isNative := k.IsNativePrecompileAvailable(ctx, address)
	isDynamic := k.IsDynamicPrecompileAvailable(ctx, address)

	if available := isNative || isDynamic; !available {
		return nil, false, nil
	}

	precompile, err := k.InstantiateERC20Precompile(ctx, address, isNative)
	if err != nil {
		return nil, false, errorsmod.Wrapf(err, "precompiled contract not initialized: %s", address.String())
	}

	return precompile, true, nil
}

// InstantiateERC20Precompile returns an ERC20 precompile instance for the given
// contract address.
// If the `hasWrappedMethods` boolean is true, the ERC20 instance returned
// exposes methods for `withdraw` and `deposit` as it is common for wrapped tokens.
func (k Keeper) InstantiateERC20Precompile(ctx sdk.Context, contractAddr common.Address, hasWrappedMethods bool) (vm.PrecompiledContract, error) {
	address := contractAddr.String()
	// check if the precompile is an ERC20 contract
	id := k.GetTokenPairID(ctx, address)
	if len(id) == 0 {
		return nil, fmt.Errorf("precompile id not found: %s", address)
	}
	pair, ok := k.GetTokenPair(ctx, id)
	if !ok {
		return nil, fmt.Errorf("token pair not found: %s", address)
	}

	if hasWrappedMethods {
		return werc20.NewPrecompile(pair, k.bankKeeper, k, *k.transferKeeper), nil
	}

	return erc20.NewPrecompile(pair, k.bankKeeper, k, *k.transferKeeper), nil
}

// RegisterCodeHash checks if a new precompile already exists and registers the code hash it is not
func (k Keeper) RegisterCodeHash(ctx sdk.Context, addr common.Address, pType PrecompileType) error {
	shouldRegister := false
	switch pType {
	case PrecompileTypeNative:
		shouldRegister = !k.IsNativePrecompileAvailable(ctx, addr)
	case PrecompileTypeDynamic:
		shouldRegister = !k.IsDynamicPrecompileAvailable(ctx, addr)
	default:
		return fmt.Errorf("invalid precompile type: %v", pType)
	}

	if shouldRegister {
		if err := k.RegisterERC20CodeHash(ctx, addr); err != nil {
			return err
		}
	}

	return nil
}

// EnableNativePrecompile adds the address of the given precompile to the prefix store
func (k Keeper) EnableNativePrecompile(ctx sdk.Context, addr common.Address) error {
	k.Logger(ctx).Info("Added new precompiles", "addresses", addr)
	if err := k.RegisterCodeHash(ctx, addr, PrecompileTypeNative); err != nil {
		return err
	}
	k.SetNativePrecompile(ctx, addr)
	return nil
}

// Only to be used by ExportGenesis, not to be directly used
func (k Keeper) GetNativePrecompiles(ctx sdk.Context) []string {
	iterator := storetypes.KVStorePrefixIterator(ctx.KVStore(k.storeKey), types.KeyPrefixNativePrecompiles)
	defer iterator.Close()

	nps := make([]string, 0)
	for ; iterator.Valid(); iterator.Next() {
		key := iterator.Key()[len(types.KeyPrefixNativePrecompiles):]
		nps = append(nps, string(key))
	}

	slices.Sort(nps)
	return nps
}

func (k Keeper) IsNativePrecompileAvailable(ctx sdk.Context, precompile common.Address) bool {
	if k.legacyPrecompiles.Below(ctx) {
		return k.legacyPrecompileRegistered(ctx, legacyNativePrecompilesKey, precompile)
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixNativePrecompiles)
	return store.Has([]byte(precompile.Hex()))
}

func (k Keeper) SetNativePrecompile(ctx sdk.Context, precompile common.Address) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixNativePrecompiles)
	store.Set([]byte(precompile.Hex()), isTrue)
}

func (k Keeper) DeleteNativePrecompile(ctx sdk.Context, precompile common.Address) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixNativePrecompiles)
	store.Delete([]byte(precompile.Hex()))
}

// EnableDynamicPrecompile adds the address of the given precompile to the prefix store
func (k Keeper) EnableDynamicPrecompile(ctx sdk.Context, address common.Address) error {
	k.Logger(ctx).Info("Added new precompiles", "addresses", address)
	if err := k.RegisterCodeHash(ctx, address, PrecompileTypeDynamic); err != nil {
		return err
	}
	k.SetDynamicPrecompile(ctx, address)
	return nil
}

// Only to be used by ExportGenesis, not to be directly used
func (k Keeper) GetDynamicPrecompiles(ctx sdk.Context) []string {
	iterator := storetypes.KVStorePrefixIterator(ctx.KVStore(k.storeKey), types.KeyPrefixDynamicPrecompiles)
	defer iterator.Close()

	dps := make([]string, 0)
	for ; iterator.Valid(); iterator.Next() {
		key := iterator.Key()[len(types.KeyPrefixDynamicPrecompiles):]
		dps = append(dps, string(key))
	}

	slices.Sort(dps)
	return dps
}

func (k Keeper) IsDynamicPrecompileAvailable(ctx sdk.Context, precompile common.Address) bool {
	if k.legacyPrecompiles.Below(ctx) {
		return k.legacyPrecompileRegistered(ctx, legacyDynamicPrecompilesKey, precompile)
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixDynamicPrecompiles)
	return store.Has([]byte(precompile.Hex()))
}

// legacyNativePrecompilesKey and legacyDynamicPrecompilesKey are the
// pre-migration store keys that held the precompile lists as a single
// concatenated blob of 42-character ASCII hex addresses. They were replaced by
// per-address keys ({0x06}/{0x07}) and deleted during the store migration, so
// they only remain in historical state below the migration height.
var (
	legacyNativePrecompilesKey  = []byte("NativePrecompiles")
	legacyDynamicPrecompilesKey = []byte("DynamicPrecompiles")
)

const legacyPrecompileAddrLen = 42 // len("0x" + 40 hex chars)

// legacyPrecompileRegistered reports whether precompile appears in the legacy
// concatenated-blob list stored under legacyKey. Addresses are compared by value
// (case-insensitive) to be robust to checksum-casing differences between how the
// old node wrote the list and the queried address.
func (k Keeper) legacyPrecompileRegistered(ctx sdk.Context, legacyKey []byte, precompile common.Address) bool {
	return legacyBlobContains(ctx.KVStore(k.storeKey).Get(legacyKey), precompile)
}

// legacyBlobContains reports whether precompile appears in a legacy
// concatenated-blob precompile list (42-character ASCII hex addresses back to
// back). A blob whose length is not a multiple of the address width is treated
// as absent rather than parsed partially.
func legacyBlobContains(blob []byte, precompile common.Address) bool {
	if len(blob) == 0 || len(blob)%legacyPrecompileAddrLen != 0 {
		return false
	}
	for i := 0; i+legacyPrecompileAddrLen <= len(blob); i += legacyPrecompileAddrLen {
		hexAddr := string(blob[i : i+legacyPrecompileAddrLen])
		if common.IsHexAddress(hexAddr) && common.HexToAddress(hexAddr) == precompile {
			return true
		}
	}
	return false
}

func (k Keeper) SetDynamicPrecompile(ctx sdk.Context, precompile common.Address) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixDynamicPrecompiles)
	store.Set([]byte(precompile.Hex()), isTrue)
}

func (k Keeper) DeleteDynamicPrecompile(ctx sdk.Context, precompile common.Address) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixDynamicPrecompiles)
	store.Delete([]byte(precompile.Hex()))
}
