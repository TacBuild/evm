package ed25519

import (
	"crypto/ed25519"
	"crypto/rand"

	"github.com/ethereum/go-ethereum/core/vm"

	edprecompile "github.com/cosmos/evm/precompiles/ed25519"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

func (s *PrecompileTestSuite) TestAddress() {
	s.Require().Equal(evmtypes.Ed25519PrecompileAddress, s.precompile.Address().String())
}

func (s *PrecompileTestSuite) TestRequiredGas() {
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
			"empty input",
			[]byte{},
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*2,
		},
		{
			"empty message (ABI-packed)",
			packCall(0),
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*2,
		},
		{
			"32 byte message",
			packCall(32),
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*3,
		},
		{
			"64 byte message",
			packCall(64),
			edprecompile.Ed25519VerifyBaseGas + edprecompile.Sha512BaseGas + edprecompile.Sha512PerWordGas*4,
		},
		{
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
				return []byte{0x01, 0x02, 0x03, 0x04}
			},
			true,
			false,
		},
		{
			"error - input too short",
			func() []byte {
				return []byte{0x01, 0x02}
			},
			true,
			false,
		},
		{
			"error - invalid ABI data",
			func() []byte {
				method := s.precompile.Methods[edprecompile.ED25519VerifyMethod]
				return append(method.ID, []byte{0x01, 0x02, 0x03}...)
			},
			true,
			false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			input := tc.input()

			if len(input) < 4 {
				defer func() {
					if r := recover(); r != nil {
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

			var result bool
			err = s.precompile.UnpackIntoInterface(&result, edprecompile.ED25519VerifyMethod, bz)
			s.Require().NoError(err)
			if tc.expPass {
				s.Require().True(result)
			} else {
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
