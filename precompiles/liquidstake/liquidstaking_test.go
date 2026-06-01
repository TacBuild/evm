package liquidstake

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/evm/x/liquidstake/keeper"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

func TestNewPrecompileUsesCanonicalAddress(t *testing.T) {
	require.True(t, common.IsHexAddress(evmtypes.LiquidStakePrecompileAddress))

	precompile := NewPrecompile(keeper.Keeper{}, nil, nil)
	require.Equal(t, common.HexToAddress(evmtypes.LiquidStakePrecompileAddress), precompile.Address())
}
