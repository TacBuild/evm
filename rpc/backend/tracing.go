package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	tmrpcclient "github.com/cometbft/cometbft/rpc/client"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"

	rpctypes "github.com/cosmos/evm/rpc/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// TraceTransaction returns the structured logs created during the execution of EVM
// and returns them as a JSON object.
func (b *Backend) TraceTransaction(hash common.Hash, config *evmtypes.TraceConfig) (interface{}, error) {
	// Get transaction by hash
	transaction, err := b.GetTxByEthHash(hash)
	if err != nil {
		b.logger.Debug("tx not found", "hash", hash)
		return nil, err
	}

	// check if block number is 0
	if transaction.Height == 0 {
		return nil, errors.New("genesis is not traceable")
	}

	blk, err := b.TendermintBlockByNumber(rpctypes.BlockNumber(transaction.Height))
	if err != nil {
		b.logger.Debug("block not found", "height", transaction.Height)
		return nil, err
	}

	// check tx index is not out of bound
	if len(blk.Block.Txs) > math.MaxUint32 {
		return nil, fmt.Errorf("tx count %d is overfloing", len(blk.Block.Txs))
	}
	txsLen := uint32(len(blk.Block.Txs)) // #nosec G115 -- checked for int overflow already
	if txsLen < transaction.TxIndex {
		b.logger.Debug("tx index out of bounds", "index", transaction.TxIndex, "hash", hash.String(), "height", blk.Block.Height)
		return nil, fmt.Errorf("transaction not included in block %v", blk.Block.Height)
	}

	var predecessors []*evmtypes.MsgEthereumTx
	for _, txBz := range blk.Block.Txs[:transaction.TxIndex] {
		tx, err := b.clientCtx.TxConfig.TxDecoder()(txBz)
		if err != nil {
			b.logger.Debug("failed to decode transaction in block", "height", blk.Block.Height, "error", err.Error())
			continue
		}
		for _, msg := range tx.GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				continue
			}

			predecessors = append(predecessors, ethMsg)
		}
	}

	tx, err := b.clientCtx.TxConfig.TxDecoder()(blk.Block.Txs[transaction.TxIndex])
	if err != nil {
		b.logger.Debug("tx not found", "hash", hash)
		return nil, err
	}

	// add predecessor messages in current cosmos tx
	index := int(transaction.MsgIndex) // #nosec G115
	for i := 0; i < index; i++ {
		ethMsg, ok := tx.GetMsgs()[i].(*evmtypes.MsgEthereumTx)
		if !ok {
			continue
		}
		predecessors = append(predecessors, ethMsg)
	}

	ethMessage, ok := tx.GetMsgs()[transaction.MsgIndex].(*evmtypes.MsgEthereumTx)
	if !ok {
		b.logger.Debug("invalid transaction type", "type", fmt.Sprintf("%T", tx))
		return nil, fmt.Errorf("invalid transaction type %T", tx)
	}

	nc, ok := b.clientCtx.Client.(tmrpcclient.NetworkClient)
	if !ok {
		return nil, errors.New("invalid rpc client")
	}

	cp, err := nc.ConsensusParams(b.ctx, &blk.Block.Height)
	if err != nil {
		return nil, err
	}

	traceTxRequest := evmtypes.QueryTraceTxRequest{
		Msg:             ethMessage,
		Predecessors:    predecessors,
		BlockNumber:     blk.Block.Height,
		BlockTime:       blk.Block.Time,
		BlockHash:       common.Bytes2Hex(blk.BlockID.Hash),
		ProposerAddress: sdk.ConsAddress(blk.Block.ProposerAddress),
		ChainId:         b.chainID.Int64(),
		BlockMaxGas:     cp.ConsensusParams.Block.MaxGas,
	}

	if config != nil {
		traceTxRequest.TraceConfig = config
	}

	// minus one to get the context of block beginning
	contextHeight := transaction.Height - 1
	if contextHeight < 1 {
		// 0 is a special value in `ContextWithHeight`
		contextHeight = 1
	}
	traceResult, err := b.queryClient.TraceTx(rpctypes.ContextWithHeight(contextHeight), &traceTxRequest)
	if err != nil {
		return nil, err
	}

	// Response format is unknown due to custom tracer config param
	// More information can be found here https://geth.ethereum.org/docs/dapp/tracing-filtered
	var decodedResult interface{}
	err = json.Unmarshal(traceResult.Data, &decodedResult)
	if err != nil {
		return nil, err
	}

	return decodedResult, nil
}

// TraceBlock configures a new tracer according to the provided configuration, and
// executes all the transactions contained within. The return value will be one item
// per transaction, dependent on the requested tracer.
func (b *Backend) TraceBlock(height rpctypes.BlockNumber,
	config *evmtypes.TraceConfig,
	block *tmrpctypes.ResultBlock,
) ([]*evmtypes.TxTraceResult, error) {
	txs := block.Block.Txs
	txsLength := len(txs)

	if txsLength == 0 {
		// If there are no transactions return empty array
		return []*evmtypes.TxTraceResult{}, nil
	}

	txDecoder := b.clientCtx.TxConfig.TxDecoder()

	var txsMessages []*evmtypes.MsgEthereumTx
	for i, tx := range txs {
		decodedTx, err := txDecoder(tx)
		if err != nil {
			b.logger.Error("failed to decode transaction", "hash", txs[i].Hash(), "error", err.Error())
			continue
		}

		for _, msg := range decodedTx.GetMsgs() {
			ethMessage, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				// Just considers Ethereum transactions
				continue
			}
			txsMessages = append(txsMessages, ethMessage)
		}
	}

	// minus one to get the context at the beginning of the block
	contextHeight := height - 1
	if contextHeight < 1 {
		// 0 is a special value for `ContextWithHeight`.
		contextHeight = 1
	}
	ctxWithHeight := rpctypes.ContextWithHeight(int64(contextHeight))

	nc, ok := b.clientCtx.Client.(tmrpcclient.NetworkClient)
	if !ok {
		return nil, errors.New("invalid rpc client")
	}

	cp, err := nc.ConsensusParams(b.ctx, &block.Block.Height)
	if err != nil {
		return nil, err
	}

	traceBlockRequest := &evmtypes.QueryTraceBlockRequest{
		Txs:             txsMessages,
		TraceConfig:     config,
		BlockNumber:     block.Block.Height,
		BlockTime:       block.Block.Time,
		BlockHash:       common.Bytes2Hex(block.BlockID.Hash),
		ProposerAddress: sdk.ConsAddress(block.Block.ProposerAddress),
		ChainId:         b.chainID.Int64(),
		BlockMaxGas:     cp.ConsensusParams.Block.MaxGas,
	}

	res, err := b.queryClient.TraceBlock(ctxWithHeight, traceBlockRequest)
	if err != nil {
		return nil, err
	}

	decodedResults := make([]*evmtypes.TxTraceResult, txsLength)
	if err := json.Unmarshal(res.Data, &decodedResults); err != nil {
		return nil, err
	}

	return decodedResults, nil
}

// TraceCall performs a simulated call with tracing at the given block number/hash.
// It mirrors geth's debug_traceCall: optional stateOverrides and blockOverrides are applied
// before executing the call, and the call itself is traced with the provided TraceConfig.
func (b *Backend) TraceCall(
	args evmtypes.TransactionArgs,
	blockNrOrHash rpctypes.BlockNumberOrHash,
	config *rpctypes.TraceCallConfig,
) (interface{}, error) {
	// Resolve block number
	blockNr, err := b.BlockNumberFromTendermint(blockNrOrHash)
	if err != nil {
		return nil, err
	}

	header, err := b.TendermintBlockByNumber(blockNr)
	if err != nil {
		return nil, errors.New("header not found")
	}

	// Build base TraceConfig
	var traceConfig *evmtypes.TraceConfig
	if config != nil {
		traceConfig = config.TraceConfig
	}

	// Marshal call args
	argsBz, err := json.Marshal(&args)
	if err != nil {
		return nil, err
	}

	// Marshal state overrides (nil-safe)
	var stateOverrideBz []byte
	if config != nil && config.StateOverrides != nil {
		stateOverrideBz, err = json.Marshal(config.StateOverrides)
		if err != nil {
			return nil, err
		}
	}

	// Marshal block overrides (nil-safe)
	var blockOverridesBz []byte
	if config != nil && config.BlockOverrides != nil {
		blockOverridesBz, err = json.Marshal(config.BlockOverrides)
		if err != nil {
			return nil, err
		}
	}

	req := evmtypes.QueryTraceCallRequest{
		Args:            argsBz,
		GasCap:          b.RPCGasCap(),
		ProposerAddress: sdk.ConsAddress(header.Block.ProposerAddress),
		ChainId:         b.chainID.Int64(),
		TraceConfig:     traceConfig,
		StateOverride:   stateOverrideBz,
		BlockOverrides:  blockOverridesBz,
	}

	// Use the block itself as the context height: we are simulating a call on top
	// of the committed state at this height (not replaying a past tx).
	ctx := rpctypes.ContextWithHeight(blockNr.Int64())
	timeout := b.RPCEVMTimeout()

	var cancel func()
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	res, err := b.queryClient.TraceCall(ctx, &req)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(res.Data, &result); err != nil {
		return nil, err
	}

	return result, nil
}
