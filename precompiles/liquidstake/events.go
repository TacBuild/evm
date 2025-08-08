package liquidstake

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "github.com/cosmos/evm/x/liquidstake/types"
)

const (
	// EventTypeLiquidStake defines the event type for the LiquidStake transaction.
	EventTypeLiquidStake = "LiquidStake"
	// EventTypeStakeToLP defines the event type for the StakeToLP transaction.
	EventTypeStakeToLP = "StakeToLP"
	// EventTypeLiquidUnstake defines the event type for the LiquidUnstake transaction.
	EventTypeLiquidUnstake = "LiquidUnstake"
	// EventTypeUpdateParams defines the event type for the UpdateParams transaction.
	EventTypeUpdateParams = "UpdateParams"
	// EventTypeUpdateWhitelistedValidator defines the event type for the UpdateWhitelistedValidator transaction.
	EventTypeUpdateWhitelistedValidator = "UpdateWhitelistedValidator"
	// EventTypeSetModulePaused defines the event type for the SetModulePaused transaction.
	EventTypeSetModulePaused = "SetModulePaused"
)

// EmitApprovalEvent creates a new approval event emitted on an Approve, IncreaseAllowance and DecreaseAllowance transactions.
func (p Precompile) EmitApprovalEvent(ctx sdk.Context, stateDB vm.StateDB, grantee, granter common.Address, coin *sdk.Coin, typeUrls []string) error {
	// Prepare the event topics
	event := p.ABI.Events[authorization.EventTypeApproval]
	topics := make([]common.Hash, 3)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	var err error
	topics[1], err = cmn.MakeTopic(grantee)
	if err != nil {
		return err
	}

	topics[2], err = cmn.MakeTopic(granter)
	if err != nil {
		return err
	}

	// Check if the coin is set to infinite
	value := abi.MaxUint256
	if coin != nil {
		value = coin.Amount.BigInt()
	}

	// Pack the arguments to be used as the Data field
	arguments := abi.Arguments{event.Inputs[2], event.Inputs[3]}
	packed, err := arguments.Pack(typeUrls, value)
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

// EmitAllowanceChangeEvent creates a new allowance change event emitted on an IncreaseAllowance and DecreaseAllowance transactions.
func (p Precompile) EmitAllowanceChangeEvent(ctx sdk.Context, stateDB vm.StateDB, grantee, granter common.Address, typeUrls []string) error {
	// Prepare the event topics
	event := p.ABI.Events[authorization.EventTypeAllowanceChange]
	topics := make([]common.Hash, 3)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	var err error
	topics[1], err = cmn.MakeTopic(grantee)
	if err != nil {
		return err
	}

	topics[2], err = cmn.MakeTopic(granter)
	if err != nil {
		return err
	}

	newValues := make([]*big.Int, len(typeUrls))
	for i, msgURL := range typeUrls {
		// Not including expiration and convert check because we have already checked it in the previous call
		msgAuthz, _ := p.AuthzKeeper.GetAuthorization(ctx, grantee.Bytes(), granter.Bytes(), msgURL)
		liquidAuthz, _ := msgAuthz.(*types.LiquidStakeAuthorization)
		if liquidAuthz.MaxTokens == nil {
			newValues[i] = abi.MaxUint256
		} else {
			newValues[i] = liquidAuthz.MaxTokens.Amount.BigInt()
		}
	}

	// Pack the arguments to be used as the Data field
	arguments := abi.Arguments{event.Inputs[2], event.Inputs[3]}
	packed, err := arguments.Pack(typeUrls, newValues)
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

// EmitStakeToLPEvent creates a new stake to LP event emitted on a StakeToLP transaction.
func (p Precompile) EmitStakeToLPEvent(ctx sdk.Context, stateDB vm.StateDB, msg *types.MsgStakeToLP, delegatorAddr common.Address) error {
	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return err
	}
	validatorAddr := common.BytesToAddress(valAddr.Bytes())

	// Prepare the event topics
	event := p.ABI.Events[EventTypeStakeToLP]
	topics := make([]common.Hash, 3)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	topics[1], err = cmn.MakeTopic(delegatorAddr)
	if err != nil {
		return err
	}

	topics[2], err = cmn.MakeTopic(validatorAddr)
	if err != nil {
		return err
	}

	// Pack the arguments to be used as the Data field
	// Only stakedAmount and liquidAmount are not indexed, so only pack those
	arguments := abi.Arguments{event.Inputs[2], event.Inputs[3]} // stakedAmount and liquidAmount fields
	packed, err := arguments.Pack(msg.StakedAmount.Amount.BigInt(), msg.LiquidAmount.Amount.BigInt())
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
	paramsOutput := NewLiquidStakeParamsOutput(&msg.Params)

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
