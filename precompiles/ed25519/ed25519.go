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

// Embed abi json file to the executable binary. Needed when importing as dependency.
//
//go:embed abi.json
var f embed.FS

const ED25519_VERIFY_BASE_GAS = 1500
const SHA512_BASE_GAS = 60
const SHA512_PER_WORD_GAS = 8

const ED25519VerifyMethod = "ed25519Verify"

type Precompile struct {
	abi.ABI
}

func NewPrecompile() (*Precompile, error) {
	abi, err := cmn.LoadABI(f, "abi.json")
	if err != nil {
		return nil, err
	}

	return &Precompile{
		ABI: abi,
	}, nil
}

func (Precompile) Address() common.Address {
	return common.HexToAddress(evmtypes.Ed25519PrecompileAddress)
}

func (p Precompile) RequiredGas(input []byte) uint64 {
	// Challenge for ed22519 uses sha512 of sig.R, pubkey, msg
	// So exclude 32 bytes of Z from signature
	msgLen := max(len(input)-32, 0)
	return ED25519_VERIFY_BASE_GAS + SHA512_BASE_GAS + SHA512_PER_WORD_GAS*((uint64(msgLen)+31)/32)
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
