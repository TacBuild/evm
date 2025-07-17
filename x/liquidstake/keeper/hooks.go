package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	// TODO: replace it with local cosmos sdk epoch hooks
	// epochstypes "github.com/persistenceOne/persistence-sdk/v2/x/epochs/types"

	liquidstake "github.com/cosmos/evm/x/liquidstake/types"
)

type EpochHooks struct {
	k Keeper
}

// TODO: replace it with local cosmos sdk epoch hooks
// var _ epochstypes.EpochHooks = EpochHooks{}

func (k Keeper) EpochHooks() EpochHooks {
	return EpochHooks{k}
}

func (h EpochHooks) BeforeEpochStart(ctx sdk.Context, epochIdentifier string, epochNumber int64) error {
	return h.k.BeforeEpochStart(ctx, epochIdentifier, epochNumber)
}

func (h EpochHooks) AfterEpochEnd(_ sdk.Context, _ string, _ int64) error {
	// Nothing to do
	return nil
}

func (k Keeper) BeforeEpochStart(ctx sdk.Context, epochIdentifier string, _ int64) error {
	if !k.GetParams(ctx).ModulePaused {
		// Update the liquid validator set at the start of each epoch
		switch epochIdentifier {
		case liquidstake.AutocompoundEpoch:
			k.AutocompoundStakingRewards(ctx, liquidstake.GetWhitelistedValsMap(k.GetParams(ctx).WhitelistedValidators))
		case liquidstake.RebalanceEpoch:
			_ = k.UpdateLiquidValidatorSet(ctx, true)
		default:
			return fmt.Errorf("unknown epoch identifier: %s", epochIdentifier)
		}
	}

	return nil
}
