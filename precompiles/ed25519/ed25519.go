package ed25519

import (
	"crypto/ed25519"
	"embed"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

var _ vm.PrecompiledContract = &Precompile{}

var (
	// Embed abi json file to the executable binary. Needed when importing as dependency.
	//
	//go:embed abi.json
	f   embed.FS
	ABI abi.ABI
)

func init() {
	var err error
	ABI, err = cmn.LoadABI(f, "abi.json")
	if err != nil {
		panic(err)
	}
}

const (
	Ed25519VerifyBaseGas = 2000
	Sha512BaseGas        = 60
	Sha512PerWordGas     = 12
)

const ED25519VerifyMethod = "ed25519Verify"

type Precompile struct {
	abi.ABI
}

func NewPrecompile() *Precompile {
	return &Precompile{
		ABI: ABI,
	}
}

func (Precompile) Address() common.Address {
	return common.HexToAddress(evmtypes.Ed25519PrecompileAddress)
}

func (p Precompile) RequiredGas(input []byte) uint64 {
	// ed25519 challenge hashes via SHA-512: sig.R (32) ++ pubkey (32) ++ msg.
	// ABI-encoded calldata layout for ed25519Verify(bytes32, bytes32[2], bytes):
	//   4  (selector)
	//   32 (pubkey  – static bytes32)
	//   64 (sig     – static bytes32[2])
	//   32 (offset  – pointer to dynamic `bytes`)
	//   32 (length  – byte-length of msg)
	// = 164 bytes of fixed overhead; everything beyond is the message payload.
	sha512Len := uint64(64 + max(len(input)-164, 0)) //nolint:gosec
	return Ed25519VerifyBaseGas + Sha512BaseGas + Sha512PerWordGas*((sha512Len+31)/32)
}

func (p Precompile) Run(_ *vm.EVM, contract *vm.Contract, _ bool) (bz []byte, err error) {
	method, err := p.MethodById(contract.Input[:4])
	if err != nil {
		return nil, err
	}

	var result bool
	switch method.Name {
	case ED25519VerifyMethod:

		args, err := method.Inputs.Unpack(contract.Input[4:])
		if err != nil {
			return nil, err
		}

		pubKey, ok := args[0].([32]byte)
		if !ok {
			return nil, fmt.Errorf("invalid public key")
		}

		signature, ok := args[1].([2][32]byte)
		if !ok {
			return nil, fmt.Errorf("invalid signature")
		}
		sig := make([]byte, 64)
		copy(sig[:32], signature[0][:])
		copy(sig[32:], signature[1][:])

		message, ok := args[2].([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid message")
		}

		result = ed25519.Verify(pubKey[:], message, sig)
		return method.Outputs.Pack(result)
	default:
		return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
	}
}
