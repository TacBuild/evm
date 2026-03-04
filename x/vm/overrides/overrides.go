package overrides

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/cosmos/evm/x/vm/statedb"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// StateOverride is the collection of overridden accounts.
type StateOverride map[common.Address]OverrideAccount

// OverrideAccount indicates the overriding fields of account during the execution of
// a message call.
// Note, state and stateDiff can't be specified at the same time. If state is
// set, message execution will only use the data in the given state. Otherwise
// if statDiff is set, all diff will be applied first and then execute the call
// message.
type OverrideAccount struct {
	Nonce     *hexutil.Uint64             `json:"nonce"`
	Code      *hexutil.Bytes              `json:"code"`
	Balance   *hexutil.Big                `json:"balance"`
	State     map[common.Hash]common.Hash `json:"state"`
	StateDiff map[common.Hash]common.Hash `json:"stateDiff"`
}

// Apply overrides the fields of specified accounts into the given state.
func (diff StateOverride) Apply(statedb *statedb.StateDB, activePrecompiles []common.Address) error {
	if diff == nil {
		return nil
	}
	for addr, account := range diff {
		// check if the account is a precompile, if so, return error since state override is not allowed for precompiles
		if slices.Index(activePrecompiles, addr) != -1 {
			return fmt.Errorf("account %s is a precompile, state override is not allowed", addr.Hex())
		}
		// Override account nonce.
		if account.Nonce != nil {
			statedb.SetNonce(addr, uint64(*account.Nonce))
		}
		// Override account(contract) code.
		if account.Code != nil {
			statedb.SetCode(addr, *account.Code)
		}
		// Override account balance.
		if account.Balance != nil {
			u256Balance, _ := uint256.FromBig((*big.Int)(account.Balance))
			statedb.SetBalance(addr, u256Balance)
		}
		if account.State != nil && account.StateDiff != nil {
			return fmt.Errorf("account %s has both 'state' and 'stateDiff'", addr.Hex())
		}
		// Replace entire state if caller requires.
		if account.State != nil {
			statedb.SetStorage(addr, account.State)
		}
		// Apply state diff into specified accounts.
		if account.StateDiff != nil {
			for key, value := range account.StateDiff {
				statedb.SetState(addr, key, value)
			}
		}
	}
	return nil
}

// BlockOverrides is a set of header fields to override for tracing/call simulation.
// This mirrors go-ethereum's internal/ethapi/override.BlockOverrides.
type BlockOverrides struct {
	Number        *hexutil.Big    `json:"number,omitempty"`
	Difficulty    *hexutil.Big    `json:"difficulty,omitempty"`
	Time          *hexutil.Uint64 `json:"time,omitempty"`
	GasLimit      *hexutil.Uint64 `json:"gasLimit,omitempty"`
	FeeRecipient  *common.Address `json:"feeRecipient,omitempty"`
	PrevRandao    *common.Hash    `json:"prevRandao,omitempty"`
	BaseFeePerGas *hexutil.Big    `json:"baseFeePerGas,omitempty"`
	BlobBaseFee   *hexutil.Big    `json:"blobBaseFee,omitempty"`
}

// Apply overrides the given header fields into the given block context.
func (o *BlockOverrides) Apply(blockCtx *vm.BlockContext) {
	if o == nil {
		return
	}
	if o.Number != nil {
		blockCtx.BlockNumber = o.Number.ToInt()
	}
	if o.Difficulty != nil {
		blockCtx.Difficulty = o.Difficulty.ToInt()
	}
	if o.Time != nil {
		blockCtx.Time = uint64(*o.Time)
	}
	if o.GasLimit != nil {
		blockCtx.GasLimit = uint64(*o.GasLimit)
	}
	if o.FeeRecipient != nil {
		blockCtx.Coinbase = *o.FeeRecipient
	}
	if o.PrevRandao != nil {
		blockCtx.Random = o.PrevRandao
	}
	if o.BaseFeePerGas != nil {
		blockCtx.BaseFee = o.BaseFeePerGas.ToInt()
	}
	if o.BlobBaseFee != nil {
		blockCtx.BlobBaseFee = o.BlobBaseFee.ToInt()
	}
}

// GetBlockNumber returns the overridden block number or nil if not set.
func (o *BlockOverrides) GetBlockNumber() *big.Int {
	if o == nil || o.Number == nil {
		return nil
	}
	return o.Number.ToInt()
}
