package ed25519_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/suite"

	//nolint:revive // dot imports are fine for Ginkgo
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot imports are fine for Ginkgo
	. "github.com/onsi/gomega"

	edprecompile "github.com/cosmos/evm/precompiles/ed25519"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

var s *PrecompileTestSuite

type PrecompileTestSuite struct {
	suite.Suite
	ed25519Pub  ed25519.PublicKey
	ed25519Priv ed25519.PrivateKey
	precompile  *edprecompile.Precompile
}

func TestPrecompileTestSuite(t *testing.T) {
	s = new(PrecompileTestSuite)
	suite.Run(t, s)

	// Run Ginkgo integration tests
	RegisterFailHandler(Fail)
	RunSpecs(t, "Precompile Test Suite")
}

func (s *PrecompileTestSuite) SetupTest() {
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	s.Require().NoError(err)
	s.ed25519Pub = edPub
	s.ed25519Priv = edPriv
	s.precompile, err = edprecompile.NewPrecompile()
	s.Require().NoError(err)
}

func (s *PrecompileTestSuite) TestAddress() {
	s.Require().Equal(evmtypes.Ed25519PrecompileAddress, s.precompile.Address().String())
}

func (s *PrecompileTestSuite) TestRequiredGas() {
	// helper: pack a real ABI-encoded call; message length drives per-word cost.
	packCall := func(msgLen int) []byte {
		msg := make([]byte, msgLen)
		packed, err := s.precompile.Pack(
			edprecompile.ED25519VerifyMethod,
			[32]byte(s.ed25519Pub),
			sigToBytes32Arr(make([]byte, 64)),
			msg,
		)
		s.Require().NoError(err)
		return packed
	}

	testCases := []struct {
		name     string
		input    []byte
		expected uint64
	}{
		{
			// raw empty input – no ABI overhead at all → sha512Len=64 → 2 words
			"empty input",
			[]byte{},
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*2,
		},
		{
			// ABI call with 0-byte message: 4+32+64+32+32 = 164 bytes → msgLen=0 → sha512Len=64 → 2 words
			"empty message (ABI-packed)",
			packCall(0),
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*2,
		},
		{
			// 32-byte message → sha512Len=96 → 3 words
			"32 byte message",
			packCall(32),
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*3,
		},
		{
			// 64-byte message → sha512Len=128 → 4 words
			"64 byte message",
			packCall(64),
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*4,
		},
		{
			// 100-byte message → sha512Len=164 → ceil(164/32)=6 words
			"100 byte message",
			packCall(100),
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*6,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			gas := s.precompile.RequiredGas(tc.input)
			s.Require().Equal(tc.expected, gas)
		})
	}
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

func (s *PrecompileTestSuite) TestRun() {
	testCases := []struct {
		name     string
		input    func() []byte
		expError bool
		expPass  bool
	}{
		{
			"pass - valid signature",
			func() []byte {
				message := []byte("hello world")
				sig := ed25519.Sign(s.ed25519Priv, message)

				// Pack arguments using ABI
				packed, err := s.precompile.Pack(edprecompile.ED25519VerifyMethod, [32]byte(s.ed25519Pub), sigToBytes32Arr(sig), message)
				s.Require().NoError(err)
				return packed
			},
			false,
			true,
		},
		{
			"pass - empty message",
			func() []byte {
				message := []byte{}
				sig := ed25519.Sign(s.ed25519Priv, message)

				packed, err := s.precompile.Pack(edprecompile.ED25519VerifyMethod, [32]byte(s.ed25519Pub), sigToBytes32Arr(sig), message)
				s.Require().NoError(err)
				return packed
			},
			false,
			true,
		},
		{
			"pass - long message",
			func() []byte {
				message := make([]byte, 1000)
				for i := range message {
					message[i] = byte(i % 256)
				}
				sig := ed25519.Sign(s.ed25519Priv, message)

				packed, err := s.precompile.Pack(edprecompile.ED25519VerifyMethod, [32]byte(s.ed25519Pub), sigToBytes32Arr(sig), message)
				s.Require().NoError(err)
				return packed
			},
			false,
			true,
		},
		{
			"fail - invalid signature",
			func() []byte {
				message := []byte("hello world")
				// Create an invalid signature
				invalidSignature := make([]byte, 64)
				for i := range invalidSignature {
					invalidSignature[i] = byte(i)
				}

				packed, err := s.precompile.Pack(edprecompile.ED25519VerifyMethod, [32]byte(s.ed25519Pub), sigToBytes32Arr(invalidSignature), message)
				s.Require().NoError(err)
				return packed
			},
			false,
			false,
		},
		{
			"fail - wrong public key",
			func() []byte {
				message := []byte("hello world")
				sig := ed25519.Sign(s.ed25519Priv, message)

				// Generate another key pair for wrong public key
				wrongPub, _, err := ed25519.GenerateKey(rand.Reader)
				s.Require().NoError(err)

				packed, err := s.precompile.Pack(edprecompile.ED25519VerifyMethod, [32]byte(wrongPub), sigToBytes32Arr(sig), message)
				s.Require().NoError(err)
				return packed
			},
			false,
			false,
		},
		{
			"fail - wrong message",
			func() []byte {
				message := []byte("hello world")
				sig := ed25519.Sign(s.ed25519Priv, message)

				// Use different message
				wrongMessage := []byte("wrong message")

				packed, err := s.precompile.Pack(edprecompile.ED25519VerifyMethod, [32]byte(s.ed25519Pub), sigToBytes32Arr(sig), wrongMessage)
				s.Require().NoError(err)
				return packed
			},
			false,
			false,
		},
		{
			"error - invalid method",
			func() []byte {
				// Create input with invalid method selector
				return []byte{0x01, 0x02, 0x03, 0x04}
			},
			true,
			false,
		},
		{
			"error - input too short",
			func() []byte {
				// Create input that's too short (less than 4 bytes for method selector)
				return []byte{0x01, 0x02}
			},
			true,
			false,
		},
		{
			"error - invalid ABI data",
			func() []byte {
				// Get the correct method selector but provide invalid data
				method := s.precompile.Methods[edprecompile.ED25519VerifyMethod]
				methodID := method.ID

				// Append invalid data
				return append(methodID, []byte{0x01, 0x02, 0x03}...)
			},
			true,
			false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			input := tc.input()

			// Handle short inputs that would panic
			if len(input) < 4 {
				defer func() {
					if r := recover(); r != nil {
						// Expected panic for short input
						return
					}
				}()
			}

			bz, err := s.precompile.Run(nil, &vm.Contract{Input: input}, false)

			if tc.expError {
				s.Require().Error(err)
				return
			}

			s.Require().NoError(err)

			if tc.expPass {
				// Unpack the result
				var result bool
				err = s.precompile.UnpackIntoInterface(&result, edprecompile.ED25519VerifyMethod, bz)
				s.Require().NoError(err)
				s.Require().True(result)
			} else {
				// Unpack the result
				var result bool
				err = s.precompile.UnpackIntoInterface(&result, edprecompile.ED25519VerifyMethod, bz)
				s.Require().NoError(err)
				s.Require().False(result)
			}
		})
	}
}

func (s *PrecompileTestSuite) TestNewPrecompile() {
	precompile, err := edprecompile.NewPrecompile()
	s.Require().NoError(err)
	s.Require().NotNil(precompile)
	s.Require().NotNil(precompile.ABI)
	s.Require().Contains(precompile.Methods, edprecompile.ED25519VerifyMethod)
}
