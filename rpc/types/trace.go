package types

import (
	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/evm/x/vm/overrides"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// TraceCallConfig holds the configuration for debug_traceCall.
// It embeds the standard TraceConfig and extends it with state and block overrides,
// mirroring geth's ethapi.TraceCallConfig.
type TraceCallConfig struct {
	// TraceConfig contains the standard tracing options (tracer, timeout, etc.)
	*evmtypes.TraceConfig
	// StateOverrides allows overriding account state (balance, nonce, code, storage)
	// before executing the call. Same semantics as eth_call stateOverride.
	StateOverrides overrides.StateOverride `json:"stateOverrides,omitempty"`
	// BlockOverrides allows overriding block context fields (number, timestamp, etc.)
	// before executing the call.
	BlockOverrides *overrides.BlockOverrides `json:"blockOverrides,omitempty"`
	// TxIndex is the index of the transaction within the block at which the call
	// should be simulated. If set, all preceding transactions in the block will be
	// replayed to build up the correct state before the call.
	TxIndex *uint `json:"txIndex,omitempty"`
}

// TraceCallResult is the result of a debug_traceCall invocation.
// The Result field is tracer-dependent (StructLogger JSON by default).
type TraceCallResult struct {
	// Result contains the tracer output — its shape depends on the tracer used.
	Result interface{} `json:"result"`
	// ReturnValue is the raw return data of the call (hex-encoded).
	ReturnValue string `json:"returnValue,omitempty"`
	// Failed indicates whether the call ended in a VM error or revert.
	Failed bool `json:"failed"`
	// Error contains the VM error string if the call failed.
	Error string `json:"error,omitempty"`
}

// TraceCallRequest is the internal request passed to the backend.
type TraceCallRequest struct {
	Args            evmtypes.TransactionArgs
	BlockNr         BlockNumber
	Config          *TraceCallConfig
	ProposerAddress []byte
	BlockHash       common.Hash
	BlockMaxGas     int64
	ChainID         int64
}
