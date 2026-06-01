package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
)

func TestBlockedAddressesIncludesLiquidstakeDummyFeeAccount(t *testing.T) {
	blockedAddrs := BlockedAddresses()

	require.True(t, blockedAddrs[liquidstaketypes.DummyFeeAccountAcc.String()])
}
