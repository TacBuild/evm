package types

// legacy_params.go supports decoding x/vm Params that were written to state by
// the evmos-based cosmos/evm layout (@ b1c973f) used before chains migrated to
// the current Params proto. The field numbers shifted between the two layouts,
// so historical state (immutable IAVL below the migration height) cannot be
// decoded with the generated Params.Unmarshal — most visibly it panics with
// "wrong wireType = 2 for field HistoryServeWindow", because old field 10
// (active_static_precompiles, length-delimited) collides with new field 10
// (history_serve_window, varint).
//
// This is only used on read paths for historical heights (e.g. eth_call); the
// on-chain migration already re-encoded the live Params at the upgrade height.
//
//	Old → New
//	1  evm_denom                 → 1
//	4  extra_eips                → 4
//	5  chain_config              → removed
//	6  allow_unprotected_txs     → removed
//	8  evm_channels              → 7
//	9  access_control            → 8
//	10 active_static_precompiles → 9

import (
	proto "github.com/cosmos/gogoproto/proto"
)

// legacyParams mirrors the pre-migration x/vm Params binary layout. Fields 5
// (chain_config) and 6 (allow_unprotected_txs) are intentionally omitted —
// proto.Unmarshal skips them as unknown fields. It is only ever read, never
// written back to the store.
type legacyParams struct {
	EvmDenom                string        `protobuf:"bytes,1,opt,name=evm_denom,json=evmDenom,proto3"`
	ExtraEIPs               []int64       `protobuf:"varint,4,rep,packed,name=extra_eips,json=extraEips,proto3"`
	EVMChannels             []string      `protobuf:"bytes,8,rep,name=evm_channels,json=evmChannels,proto3"`
	AccessControl           AccessControl `protobuf:"bytes,9,opt,name=access_control,json=accessControl,proto3"`
	ActiveStaticPrecompiles []string      `protobuf:"bytes,10,rep,name=active_static_precompiles,json=activeStaticPrecompiles,proto3"`
}

func (m *legacyParams) Reset()         { *m = legacyParams{} }
func (m *legacyParams) String() string { return proto.CompactTextString(m) }
func (m *legacyParams) ProtoMessage()  {}

// DecodeLegacyParams decodes pre-migration x/vm Params bytes and translates them
// into the current Params layout.
//
// HistoryServeWindow and ExtendedDenomOptions have no counterpart in the old
// schema and are left at their zero values. A zero HistoryServeWindow is
// harmless on read paths: keeper helpers fall back to DefaultHistoryServeWindow
// when the parameter is zero.
func DecodeLegacyParams(raw []byte) (Params, error) {
	var old legacyParams
	if err := proto.Unmarshal(raw, &old); err != nil {
		return Params{}, err
	}

	return Params{
		EvmDenom:                old.EvmDenom,
		ExtraEIPs:               old.ExtraEIPs,
		EVMChannels:             old.EVMChannels,
		AccessControl:           old.AccessControl,
		ActiveStaticPrecompiles: old.ActiveStaticPrecompiles,
	}, nil
}
