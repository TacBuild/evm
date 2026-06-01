package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/cometbft/cometbft/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/evm/x/liquidstake/types"
)

func TestGenesisState_Validate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		malleate    func(genState *types.GenesisState)
		expectedErr string
	}{
		{
			"default is valid",
			func(genState *types.GenesisState) {},
			"",
		},
		{
			"invalid liquid validator address",
			func(genState *types.GenesisState) {
				genState.LiquidValidators = []types.LiquidValidator{
					{
						OperatorAddress: "invalidAddr",
					},
				}
			},
			"invalid liquid validator {invalidAddr}: decoding bech32 failed: string not all lowercase or all uppercase: invalid address",
		},
		{
			"empty liquid validator address",
			func(genState *types.GenesisState) {
				genState.LiquidValidators = []types.LiquidValidator{
					{
						OperatorAddress: "",
					},
				}
			},
			"invalid liquid validator {}: empty address string is not allowed: invalid address",
		},
		{
			"invalid params(UnstakeFeeRate)",
			func(genState *types.GenesisState) {
				genState.Params.UnstakeFeeRate = math.LegacyDec{}
			},
			"unstake fee rate must not be nil",
		},
		{
			"invalid whitelist weight sum",
			func(genState *types.GenesisState) {
				genState.Params.WhitelistedValidators = []types.WhitelistedValidator{
					{
						ValidatorAddress: sdk.ValAddress(crypto.AddressHash([]byte("validator"))).String(),
						TargetWeight:     math.NewInt(9000),
					},
				}
			},
			"liquidstake validator weights don't add up; expected 10000, got 9000",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			genState := types.DefaultGenesisState()
			tc.malleate(genState)
			err := types.ValidateGenesis(*genState)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}
