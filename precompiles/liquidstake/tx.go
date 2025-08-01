package liquidstake

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/authorization"

	sdk "github.com/cosmos/cosmos-sdk/types"
	keeper "github.com/cosmos/evm/x/liquidstake/keeper"
	types "github.com/cosmos/evm/x/liquidstake/types"

	evmtypes "github.com/cosmos/evm/x/vm/types"
	cmn "github.com/cosmos/evm/precompiles/common"
	"github.com/cosmos/evm/utils"

)

const (
	LiquidStakeMethod                 = "liquidStake"
	StakeToLPMethod                   = "stakeToLP"
	LiquidUnstakeMethod               = "liquidUnstake"
)

// Ensure imports are used (compiler workaround)
var (
	_ *types.MsgLiquidStake
	_ *types.LiquidStakeAuthorization
)

// Authorization types for liquid staking operations
const (
	// LiquidStakeAuthz defines the authorization type for the liquidstake Stake
	LiquidStakeAuthz = types.AuthorizationType_AUTHORIZATION_TYPE_STAKE
	// LiquidUnstakeAuthz defines the authorization type for the liquidstake Unstake
	LiquidUnstakeAuthz = types.AuthorizationType_AUTHORIZATION_TYPE_UNSTAKE
	// StakeToLPAuthz defines the authorization type for the liquidstake StakeToLP
	StakeToLPAuthz = types.AuthorizationType_AUTHORIZATION_TYPE_STAKE_TO_LP
)

//	var (
//		// stakeAuthz is the authorization grant for the caller and the delegator address
//		stakeAuthz *stakingtypes.StakeAuthorization
//		// expiration is the expiration time of the authorization grant
//		expiration *time.Time
//
//		// isCallerOrigin is true when the delegator is the same as the origin and is the caller address
//		isCallerOrigin = delegatorHexAddr == origin && contract.CallerAddress == origin
//		// isSCDelegator is true when the contract caller is the same as the delegator
//		// and is not origin (it is a SmartContract)
//		isSCDelegator = contract.CallerAddress == delegatorHexAddr && origin != contract.CallerAddress
//	)
//
//	// 3 possible cases:
//	// 1. Delegator is EOA and submits tx to stake its own funds (origin == contract_caller_addr) -> no auth needed
//	// 2. Delegator is SC and submits tx to stake its own funds -> no auth needed (should be handled at SC level)
//	// 3. Delegator is EOA and SC makes call to stake the EOA's funds -> auth needed
//
//	// no need to have authorization when the delegator is the owner of the funds
//	if !isCallerOrigin && !isSCDelegator {
//		// Check if the authorization grant exists for the caller and the origin
//		stakeAuthz, expiration, err = authorization.CheckAuthzAndAllowanceForGranter(ctx, p.AuthzKeeper, contract.CallerAddress, delegatorHexAddr, &msg.Amount, DelegateMsg)
//		if err != nil {
//			return nil, err
//		}
//	}
//
//	// Execute the transaction using the message server
//	msgSrv := stakingkeeper.NewMsgServerImpl(&p.stakingKeeper)
//	if _, err = msgSrv.Delegate(ctx, msg); err != nil {
//		return nil, err
//	}
//
//	// Only update the authorization if the contract caller is different from owner of the funds
//	if !isCallerOrigin && !isSCDelegator {
//		if err := p.UpdateStakingAuthorization(ctx, contract.CallerAddress, delegatorHexAddr, stakeAuthz, expiration, DelegateMsg, msg); err != nil {
//			return nil, err
//		}
//	}

func (p Precompile) LiquidStake(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)

	if err != nil {
		return nil, err
	}

	delegatorHexAddr, msg, err := NewMsgLiquidStake(args, bondDenom)
	if err != nil {
		return nil, err
	}

	var (
		// stakeAuthz is the authorization grant for the caller and the delegator address
		liquidAuthz *types.LiquidStakeAuthorization
		// expiration is the expiration time of the authorization grant
		expiration *time.Time

		// isCallerOrigin is true when the delegator is the same as the origin and is the caller address
		isCallerOrigin = *delegatorHexAddr == origin && contract.CallerAddress == origin
		// isSCDelegator is true when the contract caller is the same as the delegator
		// and is not origin (it is a SmartContract)
		isSCDelegator = contract.CallerAddress == *delegatorHexAddr && origin != contract.CallerAddress
	)

	// 3 possible cases:
	// 1. Delegator is EOA and submits tx to stake its own funds (origin == contract_caller_addr) -> no auth needed
	// 2. Delegator is SC and submits tx to stake its own funds -> no auth needed (should be handled at SC level)
	// 3. Delegator is EOA and SC makes call to stake the EOA's funds -> auth needed

	// no need to have authorization when the delegator is the owner of the funds
	if !isCallerOrigin && !isSCDelegator {
		// Check if the authorization grant exists for the caller and the origin

		liquidAuthz, expiration, err = authorization.CheckAuthzAndAllowanceForLiquidStake(ctx, p.AuthzKeeper, contract.CallerAddress, *delegatorHexAddr, &msg.Amount, LiquidStakeMsg)
		if err != nil {
			return nil, err
		}
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	if _, err = msgSrv.LiquidStake(ctx, msg); err != nil {
		return nil, err
	}

	// Only update the authorization if the contract caller is different from owner of the funds
	if !isCallerOrigin && !isSCDelegator {
		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, *delegatorHexAddr, liquidAuthz, expiration, LiquidStakeMsg, msg); err != nil {
			return nil, err
		}
	}

	if !isCallerOrigin && msg.Amount.Denom == evmtypes.GetEVMCoinDenom() {
		// get the delegator address from the message
		delAccAddr := sdk.MustAccAddressFromBech32(msg.DelegatorAddress)
		delHexAddr := common.BytesToAddress(delAccAddr)
		// NOTE: This ensures that the changes in the bank keeper are correctly mirrored to the EVM stateDB
		// when calling the precompile from a smart contract
		// This prevents the stateDB from overwriting the changed balance in the bank keeper when committing the EVM state.

		// Need to scale the amount to 18 decimals for the EVM balance change entry
		scaledAmt, err := utils.Uint256FromBigInt(evmtypes.ConvertAmountTo18DecimalsBigInt(msg.Amount.Amount.BigInt()))
		if err != nil {
			return nil, err
		}

		p.SetBalanceChangeEntries(cmn.NewBalanceChangeEntry(delHexAddr, scaledAmt, cmn.Sub))
	}

// Emit event after successful transaction
if err := p.EmitLiquidStakeEvent(ctx, stateDB, msg, *delegatorHexAddr); err != nil {
    return nil, err
}

return method.Outputs.Pack(true)
}

func (p Precompile) StakeToLP(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	liquidBondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)
	bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	delegatorHexAddr, msg, err := NewMsgStakeToLP(args, liquidBondDenom, bondDenom)
	if err != nil {
		return nil, err
	}

	var (
		// isCallerOrigin is true when the delegator is the same as the origin and is the caller address
		isCallerOrigin = *delegatorHexAddr == origin && contract.CallerAddress == origin
		// isSCDelegator is true when the contract caller is the same as the delegator
		// and is not origin (it is a SmartContract)
		isSCDelegator = contract.CallerAddress == *delegatorHexAddr && origin != contract.CallerAddress
	)

	// 2 possible cases:
	// 1. Delegator is EOA and submits tx to stake its own funds (origin == contract_caller_addr) -> no auth needed
	// 2. Delegator is SC and submits tx to stake its own funds -> no auth needed (should be handled at SC level)

	if !isCallerOrigin && !isSCDelegator {
		return nil, fmt.Errorf("Delegator is not Origin nor caller, Delegation through precompile for StakeToLp is not possible")
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	if _, err = msgSrv.StakeToLP(ctx, msg); err != nil {
		return nil, err
	}

// Emit event after successful transaction
if err := p.EmitStakeToLPEvent(ctx, stateDB, msg, *delegatorHexAddr); err != nil {
    return nil, err
}

return method.Outputs.Pack(true)
}

func (p Precompile) LiquidUnstake(
	ctx sdk.Context,
	origin common.Address,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom := p.liquidStakeKeeper.LiquidBondDenom(ctx)

	delegatorHexAddr, msg, err := NewMsgLiquidUnstake(args, bondDenom)
	if err != nil {
		return nil, err
	}

	var (
		// stakeAuthz is the authorization grant for the caller and the delegator address
		liquidAuthz *types.LiquidStakeAuthorization
		// expiration is the expiration time of the authorization grant
		expiration *time.Time

		// isCallerOrigin is true when the delegator is the same as the origin and is the caller address
		isCallerOrigin = *delegatorHexAddr == origin && contract.CallerAddress == origin
		// isSCDelegator is true when the contract caller is the same as the delegator
		// and is not origin (it is a SmartContract)
		isSCDelegator = contract.CallerAddress == *delegatorHexAddr && origin != contract.CallerAddress
	)

	// 3 possible cases:
	// 1. Delegator is EOA and submits tx to stake its own funds (origin == contract_caller_addr) -> no auth needed
	// 2. Delegator is SC and submits tx to stake its own funds -> no auth needed (should be handled at SC level)
	// 3. Delegator is EOA and SC makes call to stake the EOA's funds -> auth needed

	// no need to have authorization when the delegator is the owner of the funds
	if !isCallerOrigin && !isSCDelegator {
		// Check if the authorization grant exists for the caller and the origin

		liquidAuthz, expiration, err = authorization.CheckAuthzAndAllowanceForLiquidStake(ctx, p.AuthzKeeper, contract.CallerAddress, *delegatorHexAddr, &msg.Amount, LiquidUnstakeMsg)
		if err != nil {
			return nil, err
		}
	}

	msgSrv := keeper.NewMsgServerImpl(p.liquidStakeKeeper)

	// Execute the transaction using the message server
	responce, err := msgSrv.LiquidUnstake(ctx, msg)
	if err != nil {
		return nil, err
	}

	// Only update the authorization if the contract caller is different from owner of the funds
	if !isCallerOrigin && !isSCDelegator {
		if err := p.UpdateLiquidStakeAuthorization(ctx, contract.CallerAddress, *delegatorHexAddr, liquidAuthz, expiration, LiquidUnstakeMsg, msg); err != nil {
			return nil, err
		}
	}

	if !isCallerOrigin && msg.Amount.Denom == evmtypes.GetEVMCoinDenom() {
		// get the delegator address from the message
		delAccAddr := sdk.MustAccAddressFromBech32(msg.DelegatorAddress)
		delHexAddr := common.BytesToAddress(delAccAddr)
		// NOTE: This ensures that the changes in the bank keeper are correctly mirrored to the EVM stateDB
		// when calling the precompile from a smart contract
		// This prevents the stateDB from overwriting the changed balance in the bank keeper when committing the EVM state.

		// Need to scale the amount to 18 decimals for the EVM balance change entry
		scaledAmt, err := utils.Uint256FromBigInt(evmtypes.ConvertAmountTo18DecimalsBigInt(msg.Amount.Amount.BigInt()))
		if err != nil {
			return nil, err
		}

		p.SetBalanceChangeEntries(cmn.NewBalanceChangeEntry(delHexAddr, scaledAmt, cmn.Sub))
	}


// Emit event after successful transaction
if err := p.EmitLiquidUnstakeEvent(ctx, stateDB, msg, *delegatorHexAddr); err != nil {
    return nil, err
}

return method.Outputs.Pack(responce.CompletionTime.Unix())
}

