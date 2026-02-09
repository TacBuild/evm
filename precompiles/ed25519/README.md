# ED25519 Signature Verification Precompile

## Overview

The ED25519 precompile provides native support for verifying ED25519 digital signatures within the EVM. This precompile enables efficient cryptographic signature verification using the ED25519 elliptic curve, which is widely used in blockchain and cryptographic applications.

## Contract Address

The ED25519 precompile is deployed at a fixed address:

```
0x00000000000000000000000000000000000008f3
```

## Interface

### Methods

#### `ed25519Verify`

Verifies an ED25519 signature against a public key and message.

```solidity
function ed25519Verify(
    bytes32 publicKey,
    bytes32[2] signature,
    bytes message
) returns (bool isValid)
```

**Parameters:**
- `publicKey` (bytes32): The ED25519 public key (32 bytes)
- `signature` (bytes32[2]): The ED25519 signature split into two 32-byte parts:
  - `signature[0]`: R component (first 32 bytes)
  - `signature[1]`: S component (last 32 bytes)
- `message` (bytes): The message that was signed (variable length)

**Returns:**
- `isValid` (bool): `true` if the signature is valid, `false` otherwise

## Gas Costs

The gas cost for the `ed25519Verify` function is calculated dynamically based on the message length:

```
gas = ED25519_VERIFY_BASE_GAS + SHA512_BASE_GAS + SHA512_PER_WORD_GAS * ((msgLen + 31) / 32)
```

Where:
- `ED25519_VERIFY_BASE_GAS = 2000`: Base cost for ED25519 signature verification
- `SHA512_BASE_GAS = 60`: Base cost for SHA512 hashing
- `SHA512_PER_WORD_GAS = 12`: Cost per 32-byte word for SHA512 hashing
- `msgLen`: Length of the message (excluding the 36 bytes for method selector and signature.S)


## Usage Example

### Solidity

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

interface IED25519 {
    function ed25519Verify(
        bytes32 publicKey,
        bytes32[2] calldata signature,
        bytes calldata message
    ) external returns (bool isValid);
}

contract ED25519Example {
    IED25519 constant ED25519_PRECOMPILE = IED25519(0x00000000000000000000000000000000000008f3);

    function verifySignature(
        bytes32 publicKey,
        bytes32[2] calldata signature,
        bytes calldata message
    ) public returns (bool) {
        return ED25519_PRECOMPILE.ed25519Verify(publicKey, signature, message);
    }
}
```

### JavaScript/TypeScript (ethers.js)

```typescript
import { ethers } from 'ethers';

const ED25519_ADDRESS = '0x00000000000000000000000000000000000008f3';

const ed25519ABI = [
  {
    "inputs": [
      { "internalType": "bytes32", "name": "publicKey", "type": "bytes32" },
      { "internalType": "bytes32[2]", "name": "signature", "type": "bytes32[2]" },
      { "internalType": "bytes", "name": "message", "type": "bytes" }
    ],
    "name": "ed25519Verify",
    "outputs": [
      { "internalType": "bool", "name": "isValid", "type": "bool" }
    ],
    "stateMutability": "nonpayable",
    "type": "function"
  }
];

async function verifyED25519Signature(
  provider: ethers.Provider,
  publicKey: string,
  signature: [string, string],
  message: string
): Promise<boolean> {
  const ed25519Contract = new ethers.Contract(ED25519_ADDRESS, ed25519ABI, provider);
  const isValid = await ed25519Contract.ed25519Verify(publicKey, signature, message);
  return isValid;
}
```

## Technical Details

### ED25519 Algorithm

ED25519 is a public-key signature system that uses:
- Curve25519 elliptic curve
- SHA-512 hash function
- Schnorr signature scheme

### Signature Format

The ED25519 signature is 64 bytes long and is split into two components:
- **R component** (32 bytes): The first half of the signature
- **S component** (32 bytes): The second half of the signature

### Implementation

The precompile uses Go's standard `crypto/ed25519` package for signature verification, ensuring compatibility with standard ED25519 implementations.

## Security Considerations

1. **Public Key Validation**: The precompile expects a valid 32-byte ED25519 public key. Invalid keys will result in verification failure.

2. **Signature Validation**: The signature must be exactly 64 bytes (provided as two 32-byte arrays). Invalid signature lengths or formats will result in an error.

3. **Message Integrity**: The message should be provided exactly as it was when the signature was created. Any modification will cause verification to fail.

4. **Gas Limits**: When verifying large messages, ensure sufficient gas is provided based on the dynamic gas calculation formula.

## Use Cases

- **Cross-chain Communication**: Verify signatures from chains that use ED25519 (e.g., Cosmos SDK chains, Solana)
- **Identity Verification**: Validate ED25519-based digital identities
- **Secure Messaging**: Verify signed messages in decentralized applications
- **Multi-signature Wallets**: Implement multi-sig wallets that support ED25519
- **Oracle Data Verification**: Verify signed data from oracles using ED25519 keys

## Testing

The precompile includes comprehensive test coverage. Run tests with:

```bash
go test ./precompiles/ed25519/...
```

## References

- [ED25519 Specification (RFC 8032)](https://datatracker.ietf.org/doc/html/rfc8032)
- [Go crypto/ed25519 Package](https://pkg.go.dev/crypto/ed25519)
- [Curve25519 and ED25519](https://ed25519.cr.yp.to/)

## License

This precompile is part of the Cosmos EVM project and is licensed under the project's license terms.
