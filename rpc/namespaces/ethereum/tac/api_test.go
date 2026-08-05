package tac

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	rpctypes "github.com/cosmos/evm/rpc/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// TestSimulateParams pins down the JSON-RPC parameters of tac_simulate.
// go-ethereum derives the accepted parameters from the method signature, so any
// change here silently changes the public API. Block overrides are deliberately
// not part of it.
func TestSimulateParams(t *testing.T) {
	method, ok := reflect.TypeOf(&TacAPI{}).MethodByName("Simulate")
	require.True(t, ok, "tac_simulate is not exposed")

	params := make([]reflect.Type, 0, method.Type.NumIn()-1)
	for i := 1; i < method.Type.NumIn(); i++ {
		params = append(params, method.Type.In(i))
	}

	require.Equal(t, []reflect.Type{
		reflect.TypeOf(evmtypes.TransactionArgs{}),
		reflect.TypeOf(rpctypes.BlockNumberOrHash{}),
		reflect.TypeOf((*json.RawMessage)(nil)),
	}, params)
}
