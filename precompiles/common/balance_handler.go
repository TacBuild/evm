package common

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/holiman/uint256"

	"github.com/cosmos/evm/utils"
	precisebanktypes "github.com/cosmos/evm/x/precisebank/types"
	"github.com/cosmos/evm/x/vm/statedb"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// BalanceHandlerFactory is a factory struct to create BalanceHandler instances.
type BalanceHandlerFactory struct {
	bankKeeper BankKeeper
}

// NewBalanceHandler creates a new BalanceHandler instance.
func NewBalanceHandlerFactory(bankKeeper BankKeeper) *BalanceHandlerFactory {
	return &BalanceHandlerFactory{
		bankKeeper: bankKeeper,
	}
}

func (bhf BalanceHandlerFactory) NewBalanceHandler() *BalanceHandler {
	return &BalanceHandler{
		bankKeeper:    bhf.bankKeeper,
		prevEventsLen: 0,
	}
}

// BalanceHandler is a struct that handles balance changes in the Cosmos SDK context.
type BalanceHandler struct {
	bankKeeper    BankKeeper
	prevEventsLen int
}

// BeforeBalanceChange is called before any balance changes by precompile methods.
// It records the current number of events in the context to later process balance changes
// using the recorded events.
func (bh *BalanceHandler) BeforeBalanceChange(ctx sdk.Context) {
	bh.prevEventsLen = len(ctx.EventManager().Events())
}

// AfterBalanceChange processes the recorded events and updates the stateDB accordingly.
// It handles the bank events for coin spent and coin received, updating the balances
// of the spender and receiver addresses respectively.
//
// NOTES: Balance change events involving BlockedAddresses are bypassed.
// Native balances are handled separately to prevent cases where a bank coin transfer
// initiated by a precompile is unintentionally overwritten by balance changes from within a contract.
// Typically, accounts registered as BlockedAddresses in app.go—such as module accounts—are not expected to receive coins.
// However, in modules like precisebank, it is common to borrow and repay integer balances
// from the module account to support fractional balance handling.
//
// As a result, even if a module account is marked as a BlockedAddress, a keeper-level SendCoins operation
// can emit an x/bank event in which the module account appears as a spender or receiver.
// If such events are parsed and used to invoke StateDB.AddBalance or StateDB.SubBalance, authorization errors can occur.
//
// To prevent this, balance changes from events involving blocked addresses are not applied to the StateDB.
// Instead, the state changes resulting from the precompile call are applied directly via the MultiStore.
func (bh *BalanceHandler) AfterBalanceChange(ctx sdk.Context, stateDB *statedb.StateDB) error {
	events := ctx.EventManager().Events()

	for i, event := range events[bh.prevEventsLen:] {
		eventIdx := bh.prevEventsLen + i

		// Skip events already processed by flushing before the precompile was called.
		if stateDB.IsEventProcessed(eventIdx) {
			continue
		}

		switch event.Type {
		case banktypes.EventTypeCoinSpent:
			spenderAddr, err := ParseAddress(event, banktypes.AttributeKeySpender)
			if err != nil {
				return fmt.Errorf("failed to parse spender address from event %q: %w", banktypes.EventTypeCoinSpent, err)
			}
			// EVM state only tracks 20-byte accounts.
			if len(spenderAddr.Bytes()) != common.AddressLength {
				continue
			}
			if bh.bankKeeper.BlockedAddr(spenderAddr) {
				// Bypass blocked addresses
				continue
			}

			amount, err := ParseAmount(event)
			if err != nil {
				return fmt.Errorf("failed to parse amount from event %q: %w", banktypes.EventTypeCoinSpent, err)
			}

			// A delegation from a vesting account spends locked tokens (tracked as
			// DelegatedVesting), which does NOT reduce the spendable balance. Since
			// the EVM statedb tracks only spendable, subtract just the spendable
			// portion (amount - locked). Without this, a within-spendable vesting
			// delegation would spuriously burn coins at commit, and a beyond-
			// spendable one would underflow. locked is zero for ordinary spends.
			locked, err := ParseLockedAmount(event)
			if err != nil {
				return fmt.Errorf("failed to parse locked amount from event %q: %w", banktypes.EventTypeCoinSpent, err)
			}
			// The locked (vesting) portion reported by bank can never exceed the
			// total spent amount (locked = min(LockedCoins, amount) by construction).
			// If it does, the event is inconsistent — fail loudly instead of
			// silently masking a bug in the emission.
			if locked.Gt(amount) {
				return fmt.Errorf(
					"inconsistent %s event: locked amount %s exceeds spent amount %s",
					banktypes.EventTypeCoinSpent, locked, amount,
				)
			}
			// Subtract only the spendable portion (amount - locked). For ordinary
			// (non-vesting) spends locked is zero, so this equals amount.
			sub := new(uint256.Int).Sub(amount, locked)

			stateDB.SubBalance(common.BytesToAddress(spenderAddr.Bytes()), sub, tracing.BalanceChangeUnspecified)

		case banktypes.EventTypeCoinReceived:
			receiverAddr, err := ParseAddress(event, banktypes.AttributeKeyReceiver)
			if err != nil {
				return fmt.Errorf("failed to parse receiver address from event %q: %w", banktypes.EventTypeCoinReceived, err)
			}
			// EVM state only tracks 20-byte accounts.
			if len(receiverAddr.Bytes()) != common.AddressLength {
				continue
			}
			if bh.bankKeeper.BlockedAddr(receiverAddr) {
				// Bypass blocked addresses
				continue
			}

			amount, err := ParseAmount(event)
			if err != nil {
				return fmt.Errorf("failed to parse amount from event %q: %w", banktypes.EventTypeCoinReceived, err)
			}

			stateDB.AddBalance(common.BytesToAddress(receiverAddr.Bytes()), amount, tracing.BalanceChangeUnspecified)

		case precisebanktypes.EventTypeFractionalBalanceChange:
			addr, err := ParseAddress(event, precisebanktypes.AttributeKeyAddress)
			if err != nil {
				return fmt.Errorf("failed to parse address from event %q: %w", precisebanktypes.EventTypeFractionalBalanceChange, err)
			}
			// EVM state only tracks 20-byte accounts.
			if len(addr.Bytes()) != common.AddressLength {
				continue
			}
			if bh.bankKeeper.BlockedAddr(addr) {
				// Bypass blocked addresses
				continue
			}

			delta, err := ParseFractionalAmount(event)
			if err != nil {
				return fmt.Errorf("failed to parse amount from event %q: %w", precisebanktypes.EventTypeFractionalBalanceChange, err)
			}

			deltaAbs, err := utils.Uint256FromBigInt(new(big.Int).Abs(delta))
			if err != nil {
				return fmt.Errorf("failed to convert delta to Uint256: %w", err)
			}

			if delta.Sign() == 1 {
				stateDB.AddBalance(common.BytesToAddress(addr.Bytes()), deltaAbs, tracing.BalanceChangeUnspecified)
			} else if delta.Sign() == -1 {
				stateDB.SubBalance(common.BytesToAddress(addr.Bytes()), deltaAbs, tracing.BalanceChangeUnspecified)
			}

		default:
			// Non-balance events are already marked as processed above
			continue
		}
	}

	return nil
}
