package liquidstake

import (
	"cosmossdk.io/math"
	"fmt"
	"math/big"

	cmn "github.com/cosmos/evm/precompiles/common"

	"github.com/cosmos/evm/x/liquidstake/types"

	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// !!WARNING!!, from PixelPlex dev Team:
// Adding new precompiled contract introduces few implicit conflicts with dependency to cosmos/evm module
// Particulary here: adding new binded address that potentially can conflict with already exzisted
const LiquidStakingPrecompileAddress = "0x00000000000000000000000000000000000001600"

type WhitelistedValidator = struct {
	ValidatorAddress string   "json:\"ValidatorAddress\""
	TargetWeight     *big.Int "json:\"TargetWeight\""
}

type Description = struct {
	Moniker         string "json:\"moniker\""
	Identity        string "json:\"identity\""
	Website         string "json:\"website\""
	SecurityContact string "json:\"securityContact\""
	Details         string "json:\"details\""
}

type LiquidStakeParams = struct {
	LiquidBondDenom       string                 "json:\"LiquidBondDenom\""
	WhiteListedValidators []WhitelistedValidator "json:\"WhiteListedValidators\""
	UnstakeFeeRate        *big.Int               "json:\"UnstakeFeeRate\""
	LsmDisabled           bool                   "json:\"LsmDisabled\""
	MinLiquidStakeAmount  *big.Int               "json:\"MinLiquidStakeAmount\""
	CwLockedPoolAddress   string                 "json:\"CwLockedPoolAddress\""
	FeeAcountAddress      string                 "json:\"FeeAcountAddress\""
	AutocompoundFeeRate   *big.Int               "json:\"AutocompoundFeeRate\""
	WhitelistAdminAddress string                 "json:\"WhitelistAdminAddress\""
	ModulePaused          bool                   "json:\"ModulePuased\""
}

func NewMsgLiquidStake(args []interface{}, denom string) (*types.MsgLiquidStake, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 6, len(args))
	}

	delegatorAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidDelegator, args[0])
	}

	amount, ok := args[1].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidAmount, args[1])
	}

	msg := types.MsgLiquidStake{
		DelegatorAddress: sdk.AccAddress(delegatorAddress.Bytes()).String(),
		Amount:           sdk.NewCoin(denom, math.NewIntFromBigInt(amount)),
	}

	return &msg, nil
}

func NewMsgStakeToLP(args []interface{}, liquidDenom string, denom string) (*types.MsgStakeToLP, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 6, len(args))
	}

	delegatorAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidDelegator, args[0])
	}

	validatorAddress, ok := args[1].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidValidator, args[1])
	}

	stakedAmount, ok := args[2].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidAmount, args[2])
	}

	liquidAmount, ok := args[3].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidAmount, args[3])
	}

	msg := types.MsgStakeToLP{
		DelegatorAddress: sdk.AccAddress(delegatorAddress.Bytes()).String(),
		ValidatorAddress: sdk.ValAddress(validatorAddress.Bytes()).String(),
		StakedAmount:     sdk.NewCoin(denom, math.NewIntFromBigInt(stakedAmount)),
		LiquidAmount:     sdk.NewCoin(liquidDenom, math.NewIntFromBigInt(liquidAmount)),
	}

	return &msg, nil
}

func NewMsgLiquidUnstake(args []interface{}, denom string) (*types.MsgLiquidUnstake, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 6, len(args))
	}

	delegatorAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidDelegator, args[0])
	}

	amount, ok := args[1].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidAmount, args[1])
	}

	msg := types.MsgLiquidUnstake{
		DelegatorAddress: sdk.AccAddress(delegatorAddress.Bytes()).String(),
		Amount:           sdk.NewCoin(denom, math.NewIntFromBigInt(amount)),
	}

	return &msg, nil
}

func NewMsgUpdateParams(args []interface{}, denom string) (*types.MsgUpdateParams, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 6, len(args))
	}

	authorityAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "common.Address", "received", args[0])
	}

	params, ok := args[1].(LiquidStakeParams)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidAmount, args[1])
	}

	Params := types.Params{
		// I have no idea what im doing
		LiquidBondDenom:       sdk.AccAddress(params.LiquidBondDenom).String(),
		WhitelistAdminAddress: sdk.AccAddress(params.WhitelistAdminAddress).String(),
		UnstakeFeeRate:        math.LegacyNewDecFromBigInt(params.UnstakeFeeRate),
		LsmDisabled:           params.LsmDisabled,
		MinLiquidStakeAmount:  math.NewIntFromBigInt(params.MinLiquidStakeAmount),
		CwLockedPoolAddress:   sdk.AccAddress(params.CwLockedPoolAddress).String(),
		FeeAccountAddress:     sdk.AccAddress(params.FeeAcountAddress).String(),
		AutocompoundFeeRate:   math.LegacyNewDecFromBigInt(params.AutocompoundFeeRate),
		ModulePaused:          params.ModulePaused,
		WhitelistedValidators: make([]types.WhitelistedValidator, len(params.WhiteListedValidators)),
	}

	for i, whitelisted := range params.WhiteListedValidators {
		Params.WhitelistedValidators[i].ValidatorAddress = whitelisted.ValidatorAddress
		Params.WhitelistedValidators[i].TargetWeight = math.NewIntFromBigInt(whitelisted.TargetWeight)
	}

	msg := types.MsgUpdateParams{
		Authority: sdk.AccAddress(authorityAddress.Bytes()).String(),
		Params:    Params,
	}

	return &msg, nil
}

func NewMsgUpdateWhitelistedValidators(args []interface{}, denom string) (*types.MsgUpdateWhitelistedValidators, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 6, len(args))
	}

	authorityAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "common.Address", "received", args[0])
	}

	whitelistedValidators, ok := args[1].([]WhitelistedValidator)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "[]WhitelistedValidator", "received", args[1])
	}

	WhitelistedValidatorsEncoded := make([]types.WhitelistedValidator, len(whitelistedValidators))

	for i, whitelisted := range whitelistedValidators {
		WhitelistedValidatorsEncoded[i].ValidatorAddress = whitelisted.ValidatorAddress
		WhitelistedValidatorsEncoded[i].TargetWeight = math.NewIntFromBigInt(whitelisted.TargetWeight)
	}

	msg := types.MsgUpdateWhitelistedValidators{
		Authority:             sdk.AccAddress(authorityAddress.Bytes()).String(),
		WhitelistedValidators: WhitelistedValidatorsEncoded,
	}

	return &msg, nil
}

func NewMsgSetModulePaused(args []interface{}, denom string) (*types.MsgSetModulePaused, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 6, len(args))
	}

	authorityAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "common.Address", "received", args[0])
	}

	isPaused, ok := args[1].(bool)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "bool", "received", args[1])
	}

	msg := types.MsgSetModulePaused{
		Authority: sdk.AccAddress(authorityAddress.Bytes()).String(),
		IsPaused:  isPaused,
	}

	return &msg, nil
}
