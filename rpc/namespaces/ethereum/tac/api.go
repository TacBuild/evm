package tac

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/evm/rpc/backend"
	rpctypes "github.com/cosmos/evm/rpc/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	"cosmossdk.io/log"
)

// TacAPI is the tac_ prefixed set of custom TAC APIs in the Web3 JSON-RPC spec.
type TacAPI struct {
	logger  log.Logger
	backend backend.EVMBackend
}

// NewTacAPI creates an instance of the TAC Web3 API.
func NewTacAPI(logger log.Logger, backend backend.EVMBackend) *TacAPI {
	return &TacAPI{
		logger:  logger.With("client", "json-rpc"),
		backend: backend,
	}
}

// Simulate implements the custom `tac_simulate` rpc api. On top of eth_call it
// supports state overrides and reports the emitted event logs and the gas
// estimation of the call. A reverting execution is reported through the result
// (success false, vmError, output) instead of a JSON-RPC error.
func (api *TacAPI) Simulate(
	args evmtypes.TransactionArgs,
	blockNrOrHash rpctypes.BlockNumberOrHash,
	overrides *json.RawMessage,
) (*rpctypes.TacSimulateResult, error) {
	api.logger.Debug("tac_simulate", "args", args, "block number or hash", blockNrOrHash)

	blockNum, err := api.backend.BlockNumberFromComet(blockNrOrHash)
	if err != nil {
		return nil, err
	}

	data, err := api.backend.DoTacSimulate(args, blockNum, overrides)
	if err != nil {
		return nil, err
	}

	return &rpctypes.TacSimulateResult{
		Status:       len(data.VmError) == 0,
		Output:       hexutil.Bytes(data.Ret),
		VmError:      data.VmError,
		Logs:         rpctypes.ToRPCTypeLogs(data.Logs),
		GasEstimated: hexutil.Uint64(data.GasEstimated),
	}, nil
}
