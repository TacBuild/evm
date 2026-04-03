package liquidstake

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/evm/evmd/tests/integration"
	liquidstaketest "github.com/cosmos/evm/tests/integration/precompiles/liquidstake"
)

func TestLiquidStakePrecompileTestSuite(t *testing.T) {
	s := liquidstaketest.NewPrecompileTestSuite(integration.CreateEvmd)
	suite.Run(t, s)
}
