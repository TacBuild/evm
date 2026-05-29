package liquidstake

import (
	"cosmossdk.io/math"
	"fmt"
	"math/big"

	cmn "github.com/cosmos/evm/precompiles/common"

	"github.com/cosmos/evm/x/liquidstake/types"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// !!WARNING!!, from PixelPlex dev Team:
// Adding new precompiled contract introduces few implicit conflicts with dependency to cosmos/evm module
// Particulary here: adding new binded address that potentially can conflict with already exzisted
const LiquidStakingPrecompileAddress = "0x00000000000000000000000000000000000001600"

type WhitelistedValidator = struct {
	ValidatorAddress common.Address `json:"validatorAddress"`
	TargetWeight     *big.Int       `json:"targetWeight"`
}

type LiquidStakeParams = struct {
	LiquidBondDenom       string                 `json:"liquidBondDenom"`
	WhitelistedValidators []WhitelistedValidator `json:"whitelistedValidators"`
	UnstakeFeeRate        *big.Int               `json:"unstakeFeeRate"`
	MinLiquidStakeAmount  *big.Int               `json:"minLiquidStakeAmount"`
	CwLockedPoolAddress   common.Address         `json:"cwLockedPoolAddress"`
	FeeAccountAddress     common.Address         `json:"feeAccountAddress"`
	AutocompoundFeeRate   *big.Int               `json:"autocompoundFeeRate"`
	WhitelistAdminAddress common.Address         `json:"whitelistAdminAddress"`
	ModulePaused          bool                   `json:"modulePaused"`
}

type LiquidStakeUpdatableParams = struct {
	UnstakeFeeRate        *big.Int       `json:"unstakeFeeRate"`
	MinLiquidStakeAmount  *big.Int       `json:"minLiquidStakeAmount"`
	CwLockedPoolAddress   common.Address `json:"cwLockedPoolAddress"`
	FeeAccountAddress     common.Address `json:"feeAccountAddress"`
	AutocompoundFeeRate   *big.Int       `json:"autocompoundFeeRate"`
	WhitelistAdminAddress common.Address `json:"whitelistAdminAddress"`
}

type LiquidValidatorState struct {
	OperatorAddress common.Address `json:"operatorAddress"`
	Weight          *big.Int       `json:"weight"`
	Status          uint8          `json:"status"`
	DelShares       *big.Int       `json:"delShares"`
	LiquidTokens    *big.Int       `json:"liquidTokens"`
}

type NetAmount struct {
	MintRate              *big.Int `json:"mintRate"`
	StkTACTotalSupply     *big.Int `json:"stkTACTotalSupply"`
	NetAmount             *big.Int `json:"netAmount"`
	TotalDelShares        *big.Int `json:"totalDelShares"`
	TotalLiquidTokens     *big.Int `json:"totalLiquidTokens"`
	TotalRemainingRewards *big.Int `json:"totalRemainingRewards"`
	TotalUnbondingBalance *big.Int `json:"totalUnbondingBalance"`
	ProxyAccBalance       *big.Int `json:"proxyAccBalance"`
}

// EventLiquidStake represents the LiquidStake event data
type EventLiquidStake struct {
	DelegatorAddress common.Address `json:"delegatorAddress"`
	Amount           *big.Int       `json:"amount"`
}

// EventLiquidUnstake represents the LiquidUnstake event data
type EventLiquidUnstake struct {
	DelegatorAddress common.Address `json:"delegatorAddress"`
	Amount           *big.Int       `json:"amount"`
}

// EventUpdateParams represents the UpdateParams event data
type EventUpdateParams struct {
	Params LiquidStakeUpdatableParams `json:"params"`
}

// EventUpdateWhitelistedValidator represents the UpdateWhitelistedValidator event data
type EventUpdateWhitelistedValidator struct {
	WhitelistedValidators []WhitelistedValidator `json:"whitelistedValidators"`
}

// EventSetModulePaused represents the SetModulePaused event data
type EventSetModulePaused struct {
	IsPaused bool `json:"isPaused"`
}

func NewLiquidValidatorOutput(lvs *types.LiquidValidatorState) LiquidValidatorState {
	valAddr, err := sdk.ValAddressFromBech32(lvs.OperatorAddress)
	var validatorAddr common.Address
	if err == nil {
		validatorAddr = common.BytesToAddress(valAddr.Bytes())
	}

	return LiquidValidatorState{
		OperatorAddress: validatorAddr,
		Weight:          lvs.Weight.BigInt(),
		Status:          uint8(lvs.Status),
		DelShares:       lvs.DelShares.BigInt(),
		LiquidTokens:    lvs.LiquidTokens.BigInt(),
	}
}

func PackLiquidValidatorOutputs(lvs []types.LiquidValidatorState, args abi.Arguments) ([]byte, error) {
	outputs := make([]LiquidValidatorState, len(lvs))
	for i, state := range lvs {
		outputs[i] = NewLiquidValidatorOutput(&state)
	}
	return args.Pack(outputs)
}

func NewNetAmount(nas *types.NetAmountState) NetAmount {
	return NetAmount{
		MintRate:              nas.MintRate.BigInt(),
		StkTACTotalSupply:     nas.GtacTotalSupply.BigInt(),
		NetAmount:             nas.NetAmount.BigInt(),
		TotalDelShares:        nas.TotalDelShares.BigInt(),
		TotalLiquidTokens:     nas.TotalLiquidTokens.BigInt(),
		TotalRemainingRewards: nas.TotalRemainingRewards.BigInt(),
		TotalUnbondingBalance: nas.TotalUnbondingBalance.BigInt(),
		ProxyAccBalance:       nas.ProxyAccBalance.BigInt(),
	}
}

func NewLiquidStakeWhitelistedValidatorsOutput(params *types.Params) []WhitelistedValidator {
	whitelistedValidators := make([]WhitelistedValidator, len(params.WhitelistedValidators))
	for i, wv := range params.WhitelistedValidators {
		// Convert bech32 validator address to common.Address
		// this shouldnt ever fail
		valAddr, _ := sdk.ValAddressFromBech32(wv.ValidatorAddress)
		validatorAddr := common.BytesToAddress(valAddr.Bytes())

		whitelistedValidators[i] = WhitelistedValidator{
			ValidatorAddress: validatorAddr,
			TargetWeight:     wv.TargetWeight.BigInt(),
		}
	}

	return whitelistedValidators
}

func NewLiquidStakeParamsOutput(params *types.Params) LiquidStakeParams {
	// Convert bech32 address strings to common.Address for ABI compatibility
	var cwLockedPoolAddr, feeAccountAddr, whitelistAdminAddr common.Address

	if params.CwLockedPoolAddress != "" {
		if accAddr, err := sdk.AccAddressFromBech32(params.CwLockedPoolAddress); err == nil {
			cwLockedPoolAddr = common.BytesToAddress(accAddr.Bytes())
		}
	}

	if params.FeeAccountAddress != "" {
		if accAddr, err := sdk.AccAddressFromBech32(params.FeeAccountAddress); err == nil {
			feeAccountAddr = common.BytesToAddress(accAddr.Bytes())
		}
	}

	if params.WhitelistAdminAddress != "" {
		if accAddr, err := sdk.AccAddressFromBech32(params.WhitelistAdminAddress); err == nil {
			whitelistAdminAddr = common.BytesToAddress(accAddr.Bytes())
		}
	}

	whitelistedValidators := NewLiquidStakeWhitelistedValidatorsOutput(params)

	return LiquidStakeParams{
		LiquidBondDenom:       params.LiquidBondDenom,
		WhitelistedValidators: whitelistedValidators,
		UnstakeFeeRate:        params.UnstakeFeeRate.BigInt(),
		MinLiquidStakeAmount:  params.MinLiquidStakeAmount.BigInt(),
		CwLockedPoolAddress:   cwLockedPoolAddr,
		FeeAccountAddress:     feeAccountAddr,
		AutocompoundFeeRate:   params.AutocompoundFeeRate.BigInt(),
		WhitelistAdminAddress: whitelistAdminAddr,
		ModulePaused:          params.ModulePaused,
	}
}

func NewLiquidStakeUpdatableParamsOutput(params *types.UpdatableParams) LiquidStakeUpdatableParams {
	// Convert bech32 address strings to common.Address for ABI compatibility
	var cwLockedPoolAddr, feeAccountAddr, whitelistAdminAddr common.Address

	if params.CwLockedPoolAddress != "" {
		if accAddr, err := sdk.AccAddressFromBech32(params.CwLockedPoolAddress); err == nil {
			cwLockedPoolAddr = common.BytesToAddress(accAddr.Bytes())
		}
	}

	if params.FeeAccountAddress != "" {
		if accAddr, err := sdk.AccAddressFromBech32(params.FeeAccountAddress); err == nil {
			feeAccountAddr = common.BytesToAddress(accAddr.Bytes())
		}
	}

	if params.WhitelistAdminAddress != "" {
		if accAddr, err := sdk.AccAddressFromBech32(params.WhitelistAdminAddress); err == nil {
			whitelistAdminAddr = common.BytesToAddress(accAddr.Bytes())
		}
	}

	return LiquidStakeUpdatableParams{
		UnstakeFeeRate:        params.UnstakeFeeRate.BigInt(),
		MinLiquidStakeAmount:  params.MinLiquidStakeAmount.BigInt(),
		CwLockedPoolAddress:   cwLockedPoolAddr,
		FeeAccountAddress:     feeAccountAddr,
		AutocompoundFeeRate:   params.AutocompoundFeeRate.BigInt(),
		WhitelistAdminAddress: whitelistAdminAddr,
	}
}

func NewMsgLiquidStake(args []interface{}, denom string) (*common.Address, *types.MsgLiquidStake, error) {
	if len(args) != 2 {
		return nil, nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}

	delegatorAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf(cmn.ErrInvalidDelegator, args[0])
	}

	amount, ok := args[1].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf(cmn.ErrInvalidAmount, args[1])
	}

	msg := types.MsgLiquidStake{
		DelegatorAddress: sdk.AccAddress(delegatorAddress.Bytes()).String(),
		Amount:           sdk.NewCoin(denom, math.NewIntFromBigInt(amount)),
	}

	return &delegatorAddress, &msg, nil
}

func NewMsgLiquidUnstake(args []interface{}, denom string) (*common.Address, *types.MsgLiquidUnstake, error) {
	if len(args) != 2 {
		return nil, nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}

	delegatorAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf(cmn.ErrInvalidDelegator, args[0])
	}

	amount, ok := args[1].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf(cmn.ErrInvalidAmount, args[1])
	}

	msg := types.MsgLiquidUnstake{
		DelegatorAddress: sdk.AccAddress(delegatorAddress.Bytes()).String(),
		Amount:           sdk.NewCoin(denom, math.NewIntFromBigInt(amount)),
	}

	return &delegatorAddress, &msg, nil
}

func NewMsgUpdateParams(args []interface{}, denom string, authorityAddress common.Address) (*types.MsgUpdateParams, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}

	params, ok := args[0].(LiquidStakeUpdatableParams)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "LiquidStakeParams", "received", args[0])
	}

	Params := types.UpdatableParams{
		WhitelistAdminAddress: sdk.AccAddress(params.WhitelistAdminAddress.Bytes()).String(),
		UnstakeFeeRate:        math.LegacyNewDecFromBigIntWithPrec(params.UnstakeFeeRate, math.LegacyPrecision),
		MinLiquidStakeAmount:  math.NewIntFromBigInt(params.MinLiquidStakeAmount),
		CwLockedPoolAddress:   sdk.AccAddress(params.CwLockedPoolAddress.Bytes()).String(),
		FeeAccountAddress:     sdk.AccAddress(params.FeeAccountAddress.Bytes()).String(),
		AutocompoundFeeRate:   math.LegacyNewDecFromBigIntWithPrec(params.AutocompoundFeeRate, math.LegacyPrecision),
	}

	msg := types.MsgUpdateParams{
		Authority: sdk.AccAddress(authorityAddress.Bytes()).String(),
		Params:    Params,
	}

	return &msg, nil
}

func NewMsgUpdateWhitelistedValidators(args []interface{}, denom string, authorityAddress common.Address) (*types.MsgUpdateWhitelistedValidators, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}

	whitelistedValidators, ok := args[0].([]WhitelistedValidator)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "[]WhitelistedValidator", "received", args[0])
	}

	WhitelistedValidatorsEncoded := make([]types.WhitelistedValidator, len(whitelistedValidators))

	for i, whitelisted := range whitelistedValidators {
		WhitelistedValidatorsEncoded[i].ValidatorAddress = sdk.ValAddress(whitelisted.ValidatorAddress.Bytes()).String()
		WhitelistedValidatorsEncoded[i].TargetWeight = math.NewIntFromBigInt(whitelisted.TargetWeight)
	}

	msg := types.MsgUpdateWhitelistedValidators{
		Authority:             sdk.AccAddress(authorityAddress.Bytes()).String(),
		WhitelistedValidators: WhitelistedValidatorsEncoded,
	}

	return &msg, nil
}

func NewMsgSetModulePaused(args []interface{}, denom string, authorityAddress common.Address) (*types.MsgSetModulePaused, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}

	isPaused, ok := args[0].(bool)
	if !ok {
		return nil, fmt.Errorf(cmn.ErrInvalidType, "bool", "received", args[0])
	}

	msg := types.MsgSetModulePaused{
		Authority: sdk.AccAddress(authorityAddress.Bytes()).String(),
		IsPaused:  isPaused,
	}

	return &msg, nil
}
