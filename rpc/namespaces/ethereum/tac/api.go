package tac

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/evm/rpc/backend"
	rpctypes "github.com/cosmos/evm/rpc/types"
	"github.com/cosmos/evm/x/vm/overrides"
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

// Simulate implements the custom `tac_simulate` rpc api which supports state override and event logs as result.
func (api *TacAPI) Simulate(args evmtypes.TransactionArgs,
	blockNrOrHash rpctypes.BlockNumberOrHash,
	stateOverride overrides.StateOverride,
) (hexutil.Bytes, error) {
	api.logger.Debug("tac_simulate", "args", args.String(), "block number or hash", blockNrOrHash, "state override", stateOverride)

	blockNum, err := api.backend.BlockNumberFromTendermint(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	data, err := api.backend.DoTacSimulate(args, blockNum, stateOverride)
	if err != nil {
		return []byte{}, err
	}

	// convert evmtypes.Log to ethtypes.Log
	rpcLogs := rpctypes.ToRPCTypeLogs(data.Logs)

	tacSimulateResult := rpctypes.TacSimulateResult{
		Status:       len(data.VmError) == 0,
		Output:       hexutil.Bytes(data.Ret),
		VmError:      data.VmError,
		Logs:         rpcLogs,
		GasEstimated: hexutil.Uint64(data.GasEstimated),
	}

	ret, err := json.Marshal(tacSimulateResult)
	if err != nil {
		return []byte{}, err
	}

	return ret, nil
}
