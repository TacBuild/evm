package types_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	
	"github.com/cosmos/evm/x/liquidstake/types"
	ibctypes "github.com/cosmos/ibc-go/v8/modules/core/23-commitment/types" // Import the conflicting module
)

func TestErrorCodeRegistration(t *testing.T) {
	// This test simply imports both modules with error registrations
	// If there's a conflict, it will panic during initialization
	// The test passes if it doesn't panic
	assert.NotNil(t, types.ErrActiveLiquidValidatorsNotExists, "liquidstake error should be registered")
	assert.NotNil(t, ibctypes.ErrInvalidProof, "IBC error should be registered")
}
