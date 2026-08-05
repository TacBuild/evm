package types_test

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"

	rpc "github.com/cosmos/evm/rpc/types"
	"github.com/cosmos/evm/x/vm/statedb"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/cosmos/evm/x/vm/types/mocks"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type precompileContract struct{}

func (p *precompileContract) Address() common.Address { return common.Address{} }

func (p *precompileContract) RequiredGas(input []byte) uint64 { return 0 }

func (p *precompileContract) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return nil, nil
}

func TestApply(t *testing.T) {
	emptyTxConfig := statedb.NewEmptyTxConfig()
	db := statedb.New(sdk.Context{}.WithEventManager(sdk.NewEventManager()), mocks.NewEVMKeeper(), emptyTxConfig)
	precompiles := map[common.Address]vm.PrecompiledContract{
		common.BytesToAddress([]byte{0x1}): &precompileContract{},
		common.BytesToAddress([]byte{0x2}): &precompileContract{},
	}
	bytes2Addr := func(b []byte) *common.Address {
		a := common.BytesToAddress(b)
		return &a
	}
	testCases := map[string]struct {
		overrides           *rpc.StateOverride
		expectedPrecompiles map[common.Address]struct{}
		fail                bool
	}{
		"move to already touched precompile": {
			overrides: &rpc.StateOverride{
				common.BytesToAddress([]byte{0x1}): {
					Code:             &hexutil.Bytes{0xff},
					MovePrecompileTo: bytes2Addr([]byte{0x2}),
				},
				common.BytesToAddress([]byte{0x2}): {
					Code: &hexutil.Bytes{0x00},
				},
			},
			fail: true,
		},
		"move non-precompile": {
			overrides: &rpc.StateOverride{
				common.BytesToAddress([]byte{0x1}): {
					Code:             &hexutil.Bytes{0xff},
					MovePrecompileTo: bytes2Addr([]byte{0xff}),
				},
				common.BytesToAddress([]byte{0x3}): {
					Code:             &hexutil.Bytes{0x00},
					MovePrecompileTo: bytes2Addr([]byte{0xfe}),
				},
			},
			fail: true,
		},
		"move two precompiles": {
			overrides: &rpc.StateOverride{
				common.BytesToAddress([]byte{0x1}): {
					Code:             &hexutil.Bytes{0xff},
					MovePrecompileTo: bytes2Addr([]byte{0xff}),
				},
				common.BytesToAddress([]byte{0x2}): {
					Code:             &hexutil.Bytes{0x00},
					MovePrecompileTo: bytes2Addr([]byte{0xfe}),
				},
			},
			expectedPrecompiles: map[common.Address]struct{}{
				common.BytesToAddress([]byte{0xfe}): {},
				common.BytesToAddress([]byte{0xff}): {},
			},
			fail: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cpy := maps.Clone(precompiles)
			err := tc.overrides.Apply(db, cpy)
			if tc.fail {
				if err == nil {
					t.Errorf("%s: want error, have nothing", name)
				}
				return
			}
			if err != nil {
				t.Errorf("%s: want no error, have %v", name, err)
				return
			}
			if len(cpy) != len(tc.expectedPrecompiles) {
				t.Errorf("%s: precompile mismatch, want %d, have %d", name, len(tc.expectedPrecompiles), len(cpy))
			}
			for k := range tc.expectedPrecompiles {
				if _, ok := cpy[k]; !ok {
					t.Errorf("%s: precompile not found: %s", name, k.String())
				}
			}
		})
	}
}

func TestTacSimulateResultJSON(t *testing.T) {
	logs := []*evmtypes.Log{
		{
			Address:     common.HexToAddress("0x1234567890123456789012345678901234567890").Hex(),
			Topics:      []string{common.HexToHash("0x01").Hex()},
			Data:        []byte{0x2a},
			BlockNumber: 7,
			TxHash:      common.HexToHash("0x02").Hex(),
			TxIndex:     1,
			BlockHash:   common.HexToHash("0x03").Hex(),
			Index:       2,
		},
	}

	testCases := []struct {
		name    string
		result  rpc.TacSimulateResult
		expJSON string
	}{
		{
			name: "successful execution",
			result: rpc.TacSimulateResult{
				Status:       true,
				Output:       hexutil.Bytes{0x01, 0x02},
				Logs:         rpc.ToRPCTypeLogs(logs),
				GasEstimated: hexutil.Uint64(21000),
			},
			expJSON: `{"success":true,"output":"0x0102","logs":[{"address":"0x1234567890123456789012345678901234567890",` +
				`"topics":["0x0000000000000000000000000000000000000000000000000000000000000001"],"data":"0x2a",` +
				`"blockNumber":"0x7","transactionHash":"0x0000000000000000000000000000000000000000000000000000000000000002",` +
				`"transactionIndex":"0x1","blockHash":"0x0000000000000000000000000000000000000000000000000000000000000003",` +
				`"blockTimestamp":"0x0","logIndex":"0x2","removed":false}],"gasEstimated":"0x5208"}`,
		},
		{
			// a revert keeps the output and reports the reason, without logs
			name: "reverted execution",
			result: rpc.TacSimulateResult{
				Status:  false,
				Output:  hexutil.Bytes{0xde, 0xad},
				VmError: vm.ErrExecutionReverted.Error(),
			},
			expJSON: `{"success":false,"output":"0xdead","vmError":"execution reverted","gasEstimated":"0x0"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bz, err := json.Marshal(tc.result)
			require.NoError(t, err)
			require.JSONEq(t, tc.expJSON, string(bz))
		})
	}
}
