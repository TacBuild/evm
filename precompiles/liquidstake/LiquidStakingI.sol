// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "../common/Types.sol";

/// @dev The LiquidStakingI contract's address.
address constant LIQUIDSTAKING_PRECOMPILE_ADDRESS = 0x0000000000000000000000000000000000001600;

/// @dev The LiquidStakingI contract's instance.
LiquidStakingI constant LIQUIDSTAKING_CONTRACT = LiquidStakingI(LIQUIDSTAKING_PRECOMPILE_ADDRESS);

/// @dev Define all the available liquidstake methods.
string constant MSG_LIQUID_STAKE = "/tac.liquidstake.v1beta1.MsgLiquidStake";
string constant MSG_LIQUID_UNSTAKE = "/tac.liquidstake.v1beta1.MsgLiquidUnstake";
string constant MSG_STAKE_TO_LP = "/tac.liquidstake.v1beta1.MsgStakeToLP";
string constant MSG_UPDATE_PARAMS = "/tac.liquidstake.v1beta1.MsgUpdateParams";
string constant MSG_UPDATE_WHITELISTED_VALIDATORS = "/tac.liquidstake.v1beta1.MsgUpdateWhitelistedValidators";
string constant MSG_SET_MODULE_PAUSED = "/tac.liquidstake.v1beta1.MsgSetModulePaused";


struct WhitelistedValidator {
    address     validatorAddress;
    int256      targetWeight;
}

struct LiquidStakeParams {
    string                  liquidBondDenom;
    WhitelistedValidator[]  whitelistedValidators;
    int256                  unstakeFeeRate;
    bool                    lsmDisabled;
    int256                  minLiquidStakeAmount;
    address                 cwLockedPoolAddress;
    address                 feeAccountAddress;
    int256                  autocompoundFeeRate;
    address                 whitelistAdminAddress;
    bool                    modulePaused;
}

struct LiquidStakeUpdatableParams {
    int256                  unstakeFeeRate;
    bool                    lsmDisabled;
    int256                  minLiquidStakeAmount;
    address                 cwLockedPoolAddress;
    address                 feeAccountAddress;
    int256                  autocompoundFeeRate;
    address                 whitelistAdminAddress;
}

enum ValidatorStatus {
    Unspecified,
    Active,
    Inactive
}

struct LiquidValidatorState {
    address                 operatorAddress;
    int256                  weight;
    ValidatorStatus         status;
    int256                  delShares;
    int256                  liquidTokens;
}

struct NetAmountState {
    int256                      mintRate;
    int256                      stkTACTotalSupply;
    int256                      netAmount;
    int256                      totalDelShares;
    int256                      totalLiquidTokens;
    int256                      totalRemainingRewards;
    int256                      totalUnbondingBalance;
    int256                      proxyAccBalance;
}

/// @dev The interface through which solidity contracts will interact with liquidstaking.
interface LiquidStakingI {

    // functions definitions start
    function liquidStake(
        address         delegatorAddress,
        uint256         amount
    ) external returns (bool success);
    // dev notes: bool success corresponds to "empty" responce in message server

    function stakeToLP(
        address         delegatorAddress,
        address         validatorAddress,
        uint256         stakedAmount,
        uint256         liquidAmount
    ) external returns (bool success);

    function liquidUnstake(
        address         delegatorAddress,
        uint256         Amount
    ) external returns (int64 completionTime);


    // admin transactions
    function updateParams(
        LiquidStakeUpdatableParams calldata  params
    ) external returns (bool success);

    function updateWhitelistedValidators(
        WhitelistedValidator[] calldata whitelistedValidators
    ) external returns (bool success);

    function setModulePaused(
        bool    isPaused
    ) external returns (bool success);
    // functions definitions end

    // view functions/query definitions start
    function params() external view returns(LiquidStakeParams calldata);

    function liquidValidators() external view returns(LiquidValidatorState[] calldata);

    function states() external view returns(NetAmountState calldata);
    // view functions/query definitions end


    // events definitions start
    event LiquidStake(
        address indexed     delegatorAddress,
        uint256             amount
    );

    event StakeToLP(
        address indexed     delegatorAddress,
        address indexed     validatorAddress,
        uint256             stakedAmount,
        uint256             liquidAmount
    );

    event LiquidUnstake(
        address indexed     delegatorAddress,
        uint256             amount
    );

    event UpdateParams(
        LiquidStakeUpdatableParams  params
    );

    event UpdateWhitelistedValidator(
        WhitelistedValidator[] whitelistedValidators
    );

    event SetModulePaused(
        bool                isPaused
    );
    // events definitions end
}
