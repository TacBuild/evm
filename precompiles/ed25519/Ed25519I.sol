// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @dev The Ed25519I contract's address.
address constant ED25519_PRECOMPILE_ADDRESS = 0x00000000000000000000000000000000000008f3;

/// @dev The Ed25519I contract's instance.
Ed25519I constant ED25519_CONTRACT = Ed25519I(ED25519_PRECOMPILE_ADDRESS);

/// @author TAC Team
/// @title Ed25519 Precompiled Contract
/// @dev The interface through which solidity contracts can verify Ed25519 signatures.
/// @custom:address 0x00000000000000000000000000000000000008f3
interface Ed25519I {
    /// @dev Defines a method for verifying an Ed25519 signature.
    /// @param publicKey The public key (32 bytes).
    /// @param signature The Ed25519 signature (64 bytes) R||S.
    /// @param message The signed message.
    /// @return isValid True if the signature is valid, false otherwise.
    function ed25519Verify(
        bytes32 publicKey,
        bytes32[2] calldata signature,
        bytes calldata message
    ) external returns (bool isValid);
}