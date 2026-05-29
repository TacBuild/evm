package liquidstake

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "github.com/cosmos/evm/x/liquidstake/types"
)

const (
	// EventTypeLiquidStake defines the event type for the LiquidStake transaction.
	EventTypeLiquidStake = "LiquidStake"
	// EventTypeLiquidUnstake defines the event type for the LiquidUnstake transaction.
	EventTypeLiquidUnstake = "LiquidUnstake"
	// EventTypeUpdateParams defines the event type for the UpdateParams transaction.
	EventTypeUpdateParams = "UpdateParams"
	// EventTypeUpdateWhitelistedValidator defines the event type for the UpdateWhitelistedValidator transaction.
	EventTypeUpdateWhitelistedValidator = "UpdateWhitelistedValidator"
	// EventTypeSetModulePaused defines the event type for the SetModulePaused transaction.
	EventTypeSetModulePaused = "SetModulePaused"
)

// EmitLiquidStakeEvent creates a new liquid stake event emitted on a LiquidStake transaction.
func (p Precompile) EmitLiquidStakeEvent(ctx sdk.Context, stateDB vm.StateDB, msg *types.MsgLiquidStake, delegatorAddr common.Address) error {
	// Prepare the event topics
	event := p.ABI.Events[EventTypeLiquidStake]
	topics := make([]common.Hash, 2)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	var err error
	topics[1], err = cmn.MakeTopic(delegatorAddr)
	if err != nil {
		return err
	}

	// Pack the arguments to be used as the Data field
	// Only amount is not indexed, so only pack the amount
	arguments := abi.Arguments{event.Inputs[1]} // event.Inputs[1] is the amount field
	packed, err := arguments.Pack(msg.Amount.Amount.BigInt())
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115 // won't exceed uint64
	})

	return nil
}

// EmitLiquidUnstakeEvent creates a new liquid unstake event emitted on a LiquidUnstake transaction.
func (p Precompile) EmitLiquidUnstakeEvent(ctx sdk.Context, stateDB vm.StateDB, msg *types.MsgLiquidUnstake, delegatorAddr common.Address) error {
	// Prepare the event topics
	event := p.ABI.Events[EventTypeLiquidUnstake]
	topics := make([]common.Hash, 2)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	var err error
	topics[1], err = cmn.MakeTopic(delegatorAddr)
	if err != nil {
		return err
	}

	// Pack the arguments to be used as the Data field
	// Only amount is not indexed, so only pack the amount
	arguments := abi.Arguments{event.Inputs[1]} // event.Inputs[1] is the amount field
	packed, err := arguments.Pack(msg.Amount.Amount.BigInt())
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115 // won't exceed uint64
	})

	return nil
}

// EmitUpdateParamsEvent creates a new update params event emitted on an UpdateParams transaction.
func (p Precompile) EmitUpdateParamsEvent(ctx sdk.Context, stateDB vm.StateDB, msg *types.MsgUpdateParams) error {
	// Prepare the event topics
	event := p.ABI.Events[EventTypeUpdateParams]
	topics := make([]common.Hash, 1)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	// Convert the params to the format expected by the event
	paramsOutput := NewLiquidStakeUpdatableParamsOutput(&msg.Params)

	// Pack the arguments to be used as the Data field
	// params field is not indexed, so pack it as data
	arguments := abi.Arguments{event.Inputs[0]} // event.Inputs[0] is the params field
	packed, err := arguments.Pack(paramsOutput)
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115 // won't exceed uint64
	})

	return nil
}

// EmitUpdateWhitelistedValidatorEvent creates a new update whitelisted validator event emitted on an UpdateWhitelistedValidators transaction.
func (p Precompile) EmitUpdateWhitelistedValidatorEvent(ctx sdk.Context, stateDB vm.StateDB, msg *types.MsgUpdateWhitelistedValidators) error {
	// Prepare the event topics
	event := p.ABI.Events[EventTypeUpdateWhitelistedValidator]
	topics := make([]common.Hash, 1)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	// Convert whitelisted validators to the format expected by the event
	whitelistedValidators := make([]WhitelistedValidator, len(msg.WhitelistedValidators))
	for i, wv := range msg.WhitelistedValidators {
		// Convert bech32 validator address to common.Address
		valAddr, err := sdk.ValAddressFromBech32(wv.ValidatorAddress)
		if err != nil {
			return err
		}
		validatorAddr := common.BytesToAddress(valAddr.Bytes())

		whitelistedValidators[i] = WhitelistedValidator{
			ValidatorAddress: validatorAddr,
			TargetWeight:     wv.TargetWeight.BigInt(),
		}
	}

	// Pack the arguments to be used as the Data field
	// whitelistedValidators field is not indexed, so pack it as data
	arguments := abi.Arguments{event.Inputs[0]} // event.Inputs[0] is the whitelistedValidators field
	packed, err := arguments.Pack(whitelistedValidators)
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115 // won't exceed uint64
	})

	return nil
}

// EmitSetModulePausedEvent creates a new set module paused event emitted on a SetModulePaused transaction.
func (p Precompile) EmitSetModulePausedEvent(ctx sdk.Context, stateDB vm.StateDB, msg *types.MsgSetModulePaused) error {
	// Prepare the event topics
	event := p.ABI.Events[EventTypeSetModulePaused]
	topics := make([]common.Hash, 1)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	// Pack the arguments to be used as the Data field
	// isPaused field is not indexed, so pack it as data
	arguments := abi.Arguments{event.Inputs[0]} // event.Inputs[0] is the isPaused field
	packed, err := arguments.Pack(msg.IsPaused)
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115 // won't exceed uint64
	})

	return nil
}
