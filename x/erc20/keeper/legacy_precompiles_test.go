package keeper

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestLegacyBlobContains(t *testing.T) {
	a := "0x1111111111111111111111111111111111111111"
	b := "0x2222222222222222222222222222222222222222"
	c := "0x3333333333333333333333333333333333333333"

	blob := []byte(a + b)

	t.Run("present", func(t *testing.T) {
		if !legacyBlobContains(blob, common.HexToAddress(a)) {
			t.Fatal("expected first address to be found")
		}
		if !legacyBlobContains(blob, common.HexToAddress(b)) {
			t.Fatal("expected second address to be found")
		}
	})

	t.Run("absent", func(t *testing.T) {
		if legacyBlobContains(blob, common.HexToAddress(c)) {
			t.Fatal("expected absent address to be reported missing")
		}
	})

	t.Run("case-insensitive", func(t *testing.T) {
		// old node may have stored a differently-cased (non-EIP-55) hex string
		mixed := []byte("0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa")
		if !legacyBlobContains(mixed, common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")) {
			t.Fatal("expected case-insensitive address match")
		}
	})

	t.Run("empty blob", func(t *testing.T) {
		if legacyBlobContains(nil, common.HexToAddress(a)) {
			t.Fatal("expected false for empty blob")
		}
	})

	t.Run("malformed length", func(t *testing.T) {
		if legacyBlobContains([]byte(a+"0x12"), common.HexToAddress(a)) {
			t.Fatal("expected false for blob length not a multiple of address width")
		}
	})

	t.Run("non-hex chunk is skipped", func(t *testing.T) {
		garbage := []byte(strings.Repeat("z", legacyPrecompileAddrLen))
		if legacyBlobContains(garbage, common.HexToAddress(a)) {
			t.Fatal("expected false for non-hex chunk")
		}
	})
}
