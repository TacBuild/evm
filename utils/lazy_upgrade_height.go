package utils

import (
	"sync/atomic"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// LazyUpgradeHeight lazily resolves and caches the block height at which a
// one-off store migration was applied on this chain. It is used on read paths
// that must decode pre-migration state differently below that height (e.g.
// historical eth_call against state written before an upgrade re-encoded it).
//
// resolve is expected to read the applied x/upgrade done-height, so it only
// returns a positive value once the upgrade has executed; before then it stays 0
// and is re-read cheaply. The height is monotonic (0 until applied, then a fixed
// positive value), so the first non-zero result is memoized and hot paths pay
// only an atomic load afterwards.
type LazyUpgradeHeight struct {
	resolve func(ctx sdk.Context) int64
	cached  atomic.Int64
}

// NewLazyUpgradeHeight returns a resolver gate, or nil if resolve is nil (which
// callers treat as "feature disabled", preserving pre-migration-agnostic
// behavior for chains that never went through the migration).
func NewLazyUpgradeHeight(resolve func(ctx sdk.Context) int64) *LazyUpgradeHeight {
	if resolve == nil {
		return nil
	}
	return &LazyUpgradeHeight{resolve: resolve}
}

// Height returns the resolved migration height, or 0 when disabled or not yet
// applied. Safe to call on a nil receiver.
func (l *LazyUpgradeHeight) Height(ctx sdk.Context) int64 {
	if l == nil {
		return 0
	}
	if h := l.cached.Load(); h != 0 {
		return h
	}
	h := l.resolve(ctx)
	if h > 0 {
		l.cached.Store(h)
	}
	return h
}

// Below reports whether the context is at a positive height strictly below the
// resolved migration height, i.e. reading pre-migration state. Safe to call on a
// nil receiver (returns false).
func (l *LazyUpgradeHeight) Below(ctx sdk.Context) bool {
	if l == nil {
		return false
	}
	height := ctx.BlockHeight()
	if height <= 0 {
		return false
	}
	migrationHeight := l.Height(ctx)
	return migrationHeight > 0 && height < migrationHeight
}
