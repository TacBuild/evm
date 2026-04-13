package ed25519

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/evm/evmd/tests/integration"
	"github.com/cosmos/evm/tests/integration/precompiles/ed25519"
)

func TestEd25519PrecompileTestSuite(t *testing.T) {
	s := ed25519.NewPrecompileTestSuite(integration.CreateEvmd)
	suite.Run(t, s)
}
