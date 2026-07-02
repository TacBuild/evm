package utils_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/evm/utils"
)

func TestLazyUpgradeHeight(t *testing.T) {
	const migrationHeight = int64(100)

	t.Run("nil gate is never below", func(t *testing.T) {
		var gate *utils.LazyUpgradeHeight // disabled (resolve was nil)
		if gate.Below(sdk.Context{}.WithBlockHeight(50)) {
			t.Fatal("expected false for nil gate")
		}
		if gate.Height(sdk.Context{}.WithBlockHeight(50)) != 0 {
			t.Fatal("expected height 0 for nil gate")
		}
	})

	t.Run("nil resolve yields nil gate", func(t *testing.T) {
		if utils.NewLazyUpgradeHeight(nil) != nil {
			t.Fatal("expected nil gate when resolve is nil")
		}
	})

	t.Run("gates on height and caches", func(t *testing.T) {
		calls := 0
		gate := utils.NewLazyUpgradeHeight(func(sdk.Context) int64 {
			calls++
			return migrationHeight
		})

		if !gate.Below(sdk.Context{}.WithBlockHeight(migrationHeight - 1)) {
			t.Fatal("expected true below migration height")
		}
		if gate.Below(sdk.Context{}.WithBlockHeight(migrationHeight)) {
			t.Fatal("expected false at migration height")
		}
		if gate.Below(sdk.Context{}.WithBlockHeight(migrationHeight + 10)) {
			t.Fatal("expected false above migration height")
		}
		if calls != 1 {
			t.Fatalf("expected resolver cached after first non-zero result, got %d calls", calls)
		}
	})

	t.Run("non-positive height is never below", func(t *testing.T) {
		gate := utils.NewLazyUpgradeHeight(func(sdk.Context) int64 { return migrationHeight })
		if gate.Below(sdk.Context{}.WithBlockHeight(0)) {
			t.Fatal("expected false at height 0")
		}
	})

	t.Run("re-reads until upgrade applied, then caches", func(t *testing.T) {
		calls := 0
		resolved := int64(0)
		gate := utils.NewLazyUpgradeHeight(func(sdk.Context) int64 {
			calls++
			return resolved
		})

		// before the upgrade the resolver returns 0 and must be re-read
		if gate.Below(sdk.Context{}.WithBlockHeight(50)) {
			t.Fatal("expected false before upgrade applied")
		}
		if gate.Below(sdk.Context{}.WithBlockHeight(50)) {
			t.Fatal("expected false before upgrade applied")
		}
		if calls != 2 {
			t.Fatalf("expected resolver re-read while returning 0, got %d calls", calls)
		}

		// once applied, gating engages and the value is memoized
		resolved = migrationHeight
		if !gate.Below(sdk.Context{}.WithBlockHeight(50)) {
			t.Fatal("expected true once upgrade applied and height below it")
		}
		gate.Below(sdk.Context{}.WithBlockHeight(50))
		if calls != 3 {
			t.Fatalf("expected resolver cached after applied, got %d calls", calls)
		}
	})
}
