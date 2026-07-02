package types

import (
	"reflect"
	"strings"
	"testing"

	proto "github.com/cosmos/gogoproto/proto"
)

// marshalLegacyParams encodes Params using the pre-migration field numbers, i.e.
// exactly the bytes a pre-v1.6.0 node wrote to historical state.
func marshalLegacyParams(t *testing.T, old *legacyParams) []byte {
	t.Helper()
	raw, err := proto.Marshal(old)
	if err != nil {
		t.Fatalf("marshal legacy params: %v", err)
	}
	return raw
}

// TestCurrentSchemaFailsOnLegacyParams reproduces the mainnet failure: decoding
// pre-migration Params bytes with the current generated schema trips over old
// field 10 (active_static_precompiles, length-delimited) landing on new field 10
// (history_serve_window, varint).
func TestCurrentSchemaFailsOnLegacyParams(t *testing.T) {
	// evm_channels intentionally empty so decoding reaches field 10 and fails
	// there, matching the reported "-32000 ... wrong wireType = 2 for field
	// HistoryServeWindow".
	raw := marshalLegacyParams(t, &legacyParams{
		EvmDenom:  "atac",
		ExtraEIPs: []int64{3855},
		ActiveStaticPrecompiles: []string{
			"0x0000000000000000000000000000000000000800",
			"0x0000000000000000000000000000000000000801",
		},
	})

	var p Params
	err := proto.Unmarshal(raw, &p)
	if err == nil {
		t.Fatal("expected decode of legacy params with current schema to fail, got nil error")
	}
	if !strings.Contains(err.Error(), "HistoryServeWindow") {
		t.Fatalf("expected HistoryServeWindow wireType error, got: %v", err)
	}
}

// TestDecodeLegacyParams verifies the shifted fields are mapped to their current
// positions and the new-only fields are left at zero.
func TestDecodeLegacyParams(t *testing.T) {
	want := legacyParams{
		EvmDenom:    "atac",
		ExtraEIPs:   []int64{2929, 3855},
		EVMChannels: []string{"channel-0", "channel-3"},
		AccessControl: AccessControl{
			Create: AccessControlType{AccessType: AccessTypePermissionless},
			Call: AccessControlType{
				AccessType:        AccessTypeRestricted,
				AccessControlList: []string{"0x1111111111111111111111111111111111111111"},
			},
		},
		ActiveStaticPrecompiles: []string{
			"0x0000000000000000000000000000000000000800",
			"0x0000000000000000000000000000000000000801",
		},
	}

	raw := marshalLegacyParams(t, &want)

	got, err := DecodeLegacyParams(raw)
	if err != nil {
		t.Fatalf("DecodeLegacyParams: %v", err)
	}

	if got.EvmDenom != want.EvmDenom {
		t.Errorf("EvmDenom: got %q, want %q", got.EvmDenom, want.EvmDenom)
	}
	if !reflect.DeepEqual(got.ExtraEIPs, want.ExtraEIPs) {
		t.Errorf("ExtraEIPs: got %v, want %v", got.ExtraEIPs, want.ExtraEIPs)
	}
	if !reflect.DeepEqual(got.EVMChannels, want.EVMChannels) {
		t.Errorf("EVMChannels: got %v, want %v", got.EVMChannels, want.EVMChannels)
	}
	if !reflect.DeepEqual(got.AccessControl, want.AccessControl) {
		t.Errorf("AccessControl: got %+v, want %+v", got.AccessControl, want.AccessControl)
	}
	if !reflect.DeepEqual(got.ActiveStaticPrecompiles, want.ActiveStaticPrecompiles) {
		t.Errorf("ActiveStaticPrecompiles: got %v, want %v", got.ActiveStaticPrecompiles, want.ActiveStaticPrecompiles)
	}

	// new-only fields have no legacy counterpart and must stay zero
	if got.HistoryServeWindow != 0 {
		t.Errorf("HistoryServeWindow: got %d, want 0", got.HistoryServeWindow)
	}
	if got.ExtendedDenomOptions != nil {
		t.Errorf("ExtendedDenomOptions: got %+v, want nil", got.ExtendedDenomOptions)
	}
}
