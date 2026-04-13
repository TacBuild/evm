package ed25519

import (
	"crypto/ed25519"
	"crypto/rand"

	"github.com/stretchr/testify/suite"

	edprecompile "github.com/cosmos/evm/precompiles/ed25519"
	"github.com/cosmos/evm/testutil/integration/evm/network"
)

type PrecompileTestSuite struct {
	suite.Suite

	create      network.CreateEvmApp
	ed25519Pub  ed25519.PublicKey
	ed25519Priv ed25519.PrivateKey
	precompile  *edprecompile.Precompile
}

func NewPrecompileTestSuite(create network.CreateEvmApp) *PrecompileTestSuite {
	precompile, err := edprecompile.NewPrecompile()
	if err != nil {
		panic(err)
	}
	return &PrecompileTestSuite{
		create:     create,
		precompile: precompile,
	}
}

func (s *PrecompileTestSuite) SetupTest() {
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	s.Require().NoError(err)
	s.ed25519Pub = edPub
	s.ed25519Priv = edPriv

	s.precompile, err = edprecompile.NewPrecompile()
	s.Require().NoError(err)
}

func sigToBytes32Arr(sig []byte) [2][32]byte {
	var arr [2][32]byte
	if len(sig) != 64 {
		return arr
	}
	copy(arr[0][:], sig[:32])
	copy(arr[1][:], sig[32:])
	return arr
}
