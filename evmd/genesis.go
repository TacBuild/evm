package evmd

import (
	"encoding/json"
	"time"

	"github.com/cosmos/evm/config"
	testconstants "github.com/cosmos/evm/testutil/constants"
	epochstypes "github.com/cosmos/evm/x/epochs/types"
	erc20types "github.com/cosmos/evm/x/erc20/types"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
)

// GenesisState of the blockchain is represented here as a map of raw json
// messages key'd by an identifier string.
// The identifier is used to determine which module genesis information belongs
// to so it may be appropriately routed during init chain.
// Within this application default genesis information is retrieved from
// the ModuleBasicManager which populates json from each BasicModule
// object provided to it during init.
type GenesisState map[string]json.RawMessage

// NewEVMGenesisState returns the default genesis state for the EVM module.
//
// NOTE: for the example chain implementation we need to set the default EVM denomination,
// enable ALL precompiles, and include default preinstalls.
func NewEVMGenesisState() *evmtypes.GenesisState {
	evmGenState := evmtypes.DefaultGenesisState()
	evmGenState.Params.ActiveStaticPrecompiles = evmtypes.AvailableStaticPrecompiles
	evmGenState.Preinstalls = evmtypes.DefaultPreinstalls

	return evmGenState
}

// NewErc20GenesisState returns the default genesis state for the ERC20 module.
//
// NOTE: for the example chain implementation we are also adding a default token pair,
// which is the base denomination of the chain (i.e. the WEVMOS contract).
func NewErc20GenesisState() *erc20types.GenesisState {
	erc20GenState := erc20types.DefaultGenesisState()
	erc20GenState.TokenPairs = testconstants.ExampleTokenPairs
	erc20GenState.NativePrecompiles = []string{testconstants.WEVMOSContractMainnet}

	return erc20GenState
}

// NewMintGenesisState returns the default genesis state for the mint module.
//
// NOTE: for the example chain implementation we are also adding a default minter.
func NewMintGenesisState() *minttypes.GenesisState {
	mintGenState := minttypes.DefaultGenesisState()
	mintGenState.Params.MintDenom = config.ExampleChainDenom

	return mintGenState
}

// NewFeeMarketGenesisState returns the default genesis state for the feemarket module.
//
// NOTE: for the example chain implementation we are disabling the base fee.
func NewFeeMarketGenesisState() *feemarkettypes.GenesisState {
	feeMarketGenState := feemarkettypes.DefaultGenesisState()
	feeMarketGenState.Params.NoBaseFee = true

	return feeMarketGenState
}

// NewLiquidStakeGenesisState returns a genesis state for the liquidstake module
// with ModulePaused set to false, suitable for test environments.
func NewLiquidStakeGenesisState() *liquidstaketypes.GenesisState {
	lsGenState := liquidstaketypes.DefaultGenesisState()
	lsGenState.Params.ModulePaused = false
	return lsGenState
}

// NewEpochsGenesisState returns a genesis state for the epochs module where
// epochs are marked as already started (EpochCountingStarted=true) so they do
// not fire immediately on the very first block. The next epoch transition will
// occur after a full Duration has elapsed from startTime.
func NewEpochsGenesisState() *epochstypes.GenesisState {
	startTime := time.Now().UTC()
	epochs := []epochstypes.EpochInfo{
		{
			Identifier:              "day",
			StartTime:               startTime,
			Duration:                time.Hour * 24,
			CurrentEpoch:            1,
			CurrentEpochStartHeight: 1,
			CurrentEpochStartTime:   startTime,
			EpochCountingStarted:    true,
		},
		{
			Identifier:              "hour",
			StartTime:               startTime,
			Duration:                time.Hour,
			CurrentEpoch:            1,
			CurrentEpochStartHeight: 1,
			CurrentEpochStartTime:   startTime,
			EpochCountingStarted:    true,
		},
		{
			Identifier:              "week",
			StartTime:               startTime,
			Duration:                time.Hour * 24 * 7,
			CurrentEpoch:            1,
			CurrentEpochStartHeight: 1,
			CurrentEpochStartTime:   startTime,
			EpochCountingStarted:    true,
		},
	}
	return epochstypes.NewGenesisState(epochs)
}
