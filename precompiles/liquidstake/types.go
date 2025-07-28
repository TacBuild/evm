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
	LiquidBondDenom            string                 `json:"liquidBondDenom"`
	WhitelistedValidators      []WhitelistedValidator `json:"whitelistedValidators"`
	UnstakeFeeRate             *big.Int               `json:"unstakeFeeRate"`
	LsmDisabled                bool                   `json:"lsmDisabled"`
	MinLiquidStakeAmount       *big.Int               `json:"minLiquidStakeAmount"`
	CwLockedPoolAddress        common.Address         `json:"cwLockedPoolAddress"`
	FeeAccountAddress          common.Address         `json:"feeAccountAddress"`
	AutocompoundFeeRate        *big.Int               `json:"autocompoundFeeRate"`
	WhitelistAdminAddress      common.Address         `json:"whitelistAdminAddress"`
	ModulePaused               bool                   `json:"modulePaused"`
}

type LiquidValidatorState struct {
	OperatorAddress  common.Address   `json:"operatorAddress"`
	Weight           *big.Int         `json:"weight"`
	Status           uint8            `json:"status"`
	DelShares        *big.Int         `json:"delShares"`
	LiquidTokens     *big.Int         `json:"liquidTokens"`
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


func NewLiquidValidatorOutput(lvs *types.LiquidValidatorState) LiquidValidatorState {
	valAddr, err := sdk.ValAddressFromBech32(lvs.OperatorAddress)
	var validatorAddr common.Address
	if err == nil {
		validatorAddr = common.BytesToAddress(valAddr.Bytes())
	}

	return LiquidValidatorState{
		OperatorAddress: validatorAddr,
		Weight:           lvs.Weight.BigInt(),
		Status:           uint8(lvs.Status),
		DelShares:        lvs.DelShares.BigInt(),
		LiquidTokens:     lvs.LiquidTokens.BigInt(),
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
		StkTACTotalSupply:     nas.StkxprtTotalSupply.BigInt(),
		NetAmount:             nas.NetAmount.BigInt(),
		TotalDelShares:        nas.TotalDelShares.BigInt(),
		TotalLiquidTokens:     nas.TotalLiquidTokens.BigInt(),
		TotalRemainingRewards: nas.TotalRemainingRewards.BigInt(),
		TotalUnbondingBalance: nas.TotalUnbondingBalance.BigInt(),
		ProxyAccBalance:       nas.ProxyAccBalance.BigInt(),
	}
}


func NewLiquidStakeParamsOutput(params *types.Params) LiquidStakeParams {
	whitelistedValidators := make([]WhitelistedValidator, len(params.WhitelistedValidators))
	for i, wv := range params.WhitelistedValidators {
		// Convert bech32 validator address to common.Address
		valAddr, err := sdk.ValAddressFromBech32(wv.ValidatorAddress)
		var validatorAddr common.Address
		if err == nil {
			validatorAddr = common.BytesToAddress(valAddr.Bytes())
		} else {
			// Fallback: try as AccAddress if ValAddress fails
			accAddr, accErr := sdk.AccAddressFromBech32(wv.ValidatorAddress)
			if accErr == nil {
				validatorAddr = common.BytesToAddress(accAddr.Bytes())
			}
		}
		whitelistedValidators[i] = WhitelistedValidator{
			ValidatorAddress: validatorAddr,
			TargetWeight:     wv.TargetWeight.BigInt(),
		}
	}

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

	return LiquidStakeParams{
		LiquidBondDenom:       params.LiquidBondDenom,
		WhitelistedValidators: whitelistedValidators,
		UnstakeFeeRate:        params.UnstakeFeeRate.BigInt(),
		LsmDisabled:           params.LsmDisabled,
		MinLiquidStakeAmount:  params.MinLiquidStakeAmount.BigInt(),
		CwLockedPoolAddress:   cwLockedPoolAddr,
		FeeAccountAddress:     feeAccountAddr,
		AutocompoundFeeRate:   params.AutocompoundFeeRate.BigInt(),
		WhitelistAdminAddress: whitelistAdminAddr,
		ModulePaused:          params.ModulePaused,
	}
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
		LiquidBondDenom:       params.LiquidBondDenom,
		WhitelistAdminAddress: sdk.AccAddress(params.WhitelistAdminAddress.Bytes()).String(),
		UnstakeFeeRate:        math.LegacyNewDecFromBigInt(params.UnstakeFeeRate),
		LsmDisabled:           params.LsmDisabled,
		MinLiquidStakeAmount:  math.NewIntFromBigInt(params.MinLiquidStakeAmount),
		CwLockedPoolAddress:   sdk.AccAddress(params.CwLockedPoolAddress.Bytes()).String(),
		FeeAccountAddress:     sdk.AccAddress(params.FeeAccountAddress.Bytes()).String(),
		AutocompoundFeeRate:   math.LegacyNewDecFromBigInt(params.AutocompoundFeeRate),
		ModulePaused:          params.ModulePaused,
		WhitelistedValidators: make([]types.WhitelistedValidator, len(params.WhitelistedValidators)),
	}

	for i, whitelisted := range params.WhitelistedValidators {
		Params.WhitelistedValidators[i].ValidatorAddress = sdk.ValAddress(whitelisted.ValidatorAddress.Bytes()).String()
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
		WhitelistedValidatorsEncoded[i].ValidatorAddress = sdk.ValAddress(whitelisted.ValidatorAddress.Bytes()).String()
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
