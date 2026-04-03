package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"

	liquidstaketest "github.com/cosmos/evm/tests/integration/x/liquidstake"
)

func TestLiquidStakeKeeperTestSuite(t *testing.T) {
	suite.Run(t, liquidstaketest.NewKeeperTestSuite(CreateEvmd))
}
