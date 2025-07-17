package types

import "cosmossdk.io/errors"

// Sentinel errors for the liquidstake module.
var (
	ErrActiveLiquidValidatorsNotExists              = errors.Register(ModuleName, 1000, "active liquid validators not exists")
	ErrInvalidDenom                                 = errors.Register(ModuleName, 1001, "invalid denom")
	ErrInvalidBondDenom                             = errors.Register(ModuleName, 1002, "invalid bond denom")
	ErrInvalidLiquidBondDenom                       = errors.Register(ModuleName, 1003, "invalid liquid bond denom")
	ErrNotImplementedYet                            = errors.Register(ModuleName, 1004, "not implemented yet")
	ErrLessThanMinLiquidStakeAmount                 = errors.Register(ModuleName, 1005, "staking amount should be over params.min_liquid_stake_amount")
	ErrInvalidStkXPRTSupply                         = errors.Register(ModuleName, 1006, "invalid liquid bond denom supply")
	ErrInvalidActiveLiquidValidators                = errors.Register(ModuleName, 1007, "invalid active liquid validators")
	ErrLiquidValidatorsNotExists                    = errors.Register(ModuleName, 1008, "liquid validators not exists")
	ErrInsufficientProxyAccBalance                  = errors.Register(ModuleName, 1009, "insufficient liquid tokens or balance of proxy account, need to wait for new liquid validator to be added or unbonding of proxy account to be completed")
	ErrTooSmallLiquidStakeAmount                    = errors.Register(ModuleName, 1010, "liquid stake amount is too small, the result becomes zero")
	ErrTooSmallLiquidUnstakingAmount                = errors.Register(ModuleName, 1011, "liquid unstaking amount is too small, the result becomes zero")
	ErrNoLPContractAddress                          = errors.Register(ModuleName, 1012, "CW address of an LP contract is not set")
	ErrDisabledLSM                                  = errors.Register(ModuleName, 1013, "LSM delegation is disabled")
	ErrLSMTokenizeFailed                            = errors.Register(ModuleName, 1014, "LSM tokenization failed")
	ErrLSMRedeemFailed                              = errors.Register(ModuleName, 1015, "LSM redemption failed")
	ErrLPContract                                   = errors.Register(ModuleName, 1016, "CW contract execution failed")
	ErrWhitelistedValidatorsList                    = errors.Register(ModuleName, 1017, "whitelisted validators list incorrect")
	ErrActiveLiquidValidatorsWeightQuorumNotReached = errors.Register(ModuleName, 1018, "active liquid validators weight quorum not reached")
	ErrModulePaused                                 = errors.Register(ModuleName, 1019, "module functions have been paused")
	ErrDelegationFailed                             = errors.Register(ModuleName, 1020, "delegation failed")
	ErrUnbondFailed                                 = errors.Register(ModuleName, 1021, "unbond failed")
	ErrInvalidResponse                              = errors.Register(ModuleName, 1022, "invalid response")
	ErrUnstakeFailed                                = errors.Register(ModuleName, 1023, "Unstaking failed")
	ErrRedelegateFailed                             = errors.Register(ModuleName, 1024, "Redelegate failed")
)
