package liquidstake

import (
	"math/big"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"

	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	chainutil "github.com/cosmos/evm/testutil"
	testkeyring "github.com/cosmos/evm/testutil/keyring"
	"github.com/cosmos/evm/x/liquidstake/types"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
	"github.com/cosmos/evm/x/vm/statedb"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

func (s *PrecompileTestSuite) TestIsTransaction() {
	testCases := []struct {
		name   string
		method abi.Method
		isTx   bool
	}{
		{
			liquidstake.LiquidStakeMethod,
			s.precompile.Methods[liquidstake.LiquidStakeMethod],
			true,
		},
		{
			liquidstake.StakeToLPMethod,
			s.precompile.Methods[liquidstake.StakeToLPMethod],
			true,
		},
		{
			liquidstake.LiquidUnstakeMethod,
			s.precompile.Methods[liquidstake.LiquidUnstakeMethod],
			true,
		},
		{
			liquidstake.UpdateParamsMethod,
			s.precompile.Methods[liquidstake.UpdateParamsMethod],
			true,
		},
		{
			liquidstake.UpdateWhitelistedValidatorsMethod,
			s.precompile.Methods[liquidstake.UpdateWhitelistedValidatorsMethod],
			true,
		},
		{
			liquidstake.SetModulePausedMethod,
			s.precompile.Methods[liquidstake.SetModulePausedMethod],
			true,
		},
		{
			liquidstake.ParamsMethod,
			s.precompile.Methods[liquidstake.ParamsMethod],
			false,
		},
		{
			"invalid",
			abi.Method{},
			false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.Require().Equal(s.precompile.IsTransaction(&tc.method), tc.isTx)
		})
	}
}

func (s *PrecompileTestSuite) TestRequiredGas() {
	testcases := []struct {
		name     string
		malleate func() []byte
		expGas   uint64
	}{
		{
			"success - liquidStake transaction with correct gas estimation",
			func() []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidStakeMethod,
					s.keyring.GetAddr(0),
					big.NewInt(1000000),
				)
				s.Require().NoError(err)
				return input
			},
			4040,
		},
		{
			"success - stakeToLP transaction with correct gas estimation",
			func() []byte {
				input, err := s.precompile.Pack(
					liquidstake.StakeToLPMethod,
					s.keyring.GetAddr(0),
					s.keyring.GetAddr(1),
					big.NewInt(1000000),
					big.NewInt(1000000),
				)
				s.Require().NoError(err)
				return input
			},
			5960,
		},
		{
			"success - liquidUnstake transaction with correct gas estimation",
			func() []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidUnstakeMethod,
					s.keyring.GetAddr(0),
					big.NewInt(1000000),
				)
				s.Require().NoError(err)
				return input
			},
			4040,
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			s.SetupTest()
			input := tc.malleate()
			gas := s.precompile.RequiredGas(input)
			s.Require().Equal(tc.expGas, gas)
		})
	}
}

// TestRun tests the precompile's Run method.
func (s *PrecompileTestSuite) TestRun() {
	var ctx sdk.Context
	testcases := []struct {
		name        string
		malleate    func(delegator testkeyring.Key) []byte
		gas         uint64
		readOnly    bool
		expPass     bool
		expOutOfGas bool
		errContains string
	}{
		{
			name: "fail - contract gas limit is < gas cost to run a query / tx",
			malleate: func(delegator testkeyring.Key) []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidStakeMethod,
					delegator.Addr,
					big.NewInt(1000000),
				)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			gas:         8000,
			readOnly:    false,
			expPass:     false,
			expOutOfGas: true,
			errContains: "out of gas",
		},
		{
			name: "pass - liquidStake transaction",
			malleate: func(delegator testkeyring.Key) []byte {
				input, err := s.precompile.Pack(
					liquidstake.LiquidStakeMethod,
					delegator.Addr,
					big.NewInt(1000000),
				)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			gas:     1000000,
			expPass: true,
		},
		{
			name: "pass - stakeToLP transaction",
			malleate: func(delegator testkeyring.Key) []byte {
				delAmount := sdkmath.NewInt(1000000000000000000)
				_, err := s.nw.App.GetStakingKeeper().Delegate(ctx, sdk.AccAddress(delegator.Addr.Bytes()), delAmount, stakingtypes.Bonded, s.liquidValidator, false)
				if err != nil {
					panic(err)
				}

				_, err = s.nw.App.GetStakingKeeper().GetDelegation(ctx, delegator.AccAddr, sdk.ValAddress(s.liquidValidatorAddr.Bytes()))
				if err != nil {
					panic(err)
				}

				tokenizeAmount := big.NewInt(1000000000000000000)
				input, err := s.precompile.Pack(
					liquidstake.StakeToLPMethod,
					delegator.Addr,
					s.liquidValidatorAddr,
					tokenizeAmount,
					tokenizeAmount,
				)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			gas:     1000000000000000000,
			expPass: true,
		},
		{
			name: "pass - liquidUnstake transaction",
			malleate: func(delegator testkeyring.Key) []byte {
				lsKeeper := s.nw.App.GetLiquidStakeKeeper()
				_, err := lsKeeper.LiquidStake(ctx, liquidstaketypes.LiquidStakeProxyAcc, delegator.AccAddr, sdk.NewCoin(s.bondDenom, sdkmath.NewInt(1000000000000000000)))
				s.Require().NoError(err)

				input, err := s.precompile.Pack(
					liquidstake.LiquidUnstakeMethod,
					delegator.Addr,
					big.NewInt(1000000000000000000),
				)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			gas:     1000000,
			expPass: true,
		},
		{
			name: "fail - invalid method",
			malleate: func(_ testkeyring.Key) []byte {
				return []byte("invalid")
			},
			gas:         100000,
			expPass:     false,
			errContains: "no method with id",
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext().WithBlockTime(time.Now())

			baseFee := s.nw.App.GetEVMKeeper().GetBaseFee(ctx)

			delegator := s.keyring.GetKey(0)

			contract := vm.NewPrecompile(delegator.Addr, s.precompile.Address(), uint256.NewInt(0), tc.gas)
			contractAddr := contract.Address()

			contract.Input = tc.malleate(delegator)

			txArgs := evmtypes.EvmTxArgs{
				ChainID:   evmtypes.GetEthChainConfig().ChainID,
				Nonce:     0,
				To:        &contractAddr,
				Amount:    nil,
				GasLimit:  tc.gas,
				GasPrice:  chainutil.ExampleMinGasPrices,
				GasFeeCap: baseFee,
				GasTipCap: big.NewInt(1),
				Accesses:  &ethtypes.AccessList{},
			}

			msg, err := s.factory.GenerateGethCoreMsg(delegator.Priv, txArgs)
			s.Require().NoError(err)

			proposerAddress := ctx.BlockHeader().ProposerAddress
			cfg, err := s.nw.App.GetEVMKeeper().EVMConfig(ctx, proposerAddress)
			s.Require().NoError(err, "failed to instantiate EVM config")

			stDB := statedb.New(
				ctx,
				s.nw.App.GetEVMKeeper(),
				statedb.NewEmptyTxConfig(),
			)
			evm := s.nw.App.GetEVMKeeper().NewEVM(
				ctx, *msg, cfg, nil, stDB,
			)

			precompiles, found, err := s.nw.App.GetEVMKeeper().GetPrecompileInstance(ctx, contractAddr)
			s.Require().NoError(err, "failed to instantiate precompile")
			s.Require().True(found, "not found precompile")
			evm.WithPrecompiles(precompiles.Map)

			bz, err := s.precompile.Run(evm, contract, tc.readOnly)

			if tc.expPass {
				s.Require().NoError(err, "expected no error when running the precompile")
				s.Require().NotNil(bz, "expected returned bytes not to be nil")
			} else {
				s.Require().Error(err, "expected error to be returned when running the precompile")
				s.Require().NotNil(bz, "expected returned revert bytes not to be nil")
				execErr := evmtypes.NewExecErrorWithReason(bz)
				s.Require().ErrorContains(execErr, tc.errContains)
				if tc.expOutOfGas {
					consumed := ctx.GasMeter().GasConsumed()
					s.Require().LessOrEqual(tc.gas, consumed, "expected gas consumed to be equal or less to gas limit")
				}
			}
		})
	}
}

func (s *PrecompileTestSuite) TestPrecompileInitialization() {
	s.Require().NotNil(s.precompile)
}

func (s *PrecompileTestSuite) TestPrecompileMethodSignatures() {
	methods := s.precompile.Methods

	s.Require().Contains(methods, liquidstake.LiquidStakeMethod)
	s.Require().Contains(methods, liquidstake.StakeToLPMethod)
	s.Require().Contains(methods, liquidstake.LiquidUnstakeMethod)
	s.Require().Contains(methods, liquidstake.UpdateParamsMethod)
	s.Require().Contains(methods, liquidstake.UpdateWhitelistedValidatorsMethod)
	s.Require().Contains(methods, liquidstake.SetModulePausedMethod)
	// Test query methods
	s.Require().Contains(methods, liquidstake.ParamsMethod)
	s.Require().Contains(methods, liquidstake.LiquidValidatorsMethod)
	s.Require().Contains(methods, liquidstake.StatesMethod)
}

// TestQueryMethods tests the precompile's query methods.
func (s *PrecompileTestSuite) TestQueryMethods() {
	var ctx sdk.Context
	testcases := []struct {
		name        string
		malleate    func() []byte
		gas         uint64
		readOnly    bool
		expPass     bool
		errContains string
	}{
		{
			"pass - params query",
			func() []byte {
				input, err := s.precompile.Pack(liquidstake.ParamsMethod)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			100000,
			true,
			true,
			"",
		},
		{
			"pass - liquidValidators query",
			func() []byte {
				input, err := s.precompile.Pack(liquidstake.LiquidValidatorsMethod)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			100000,
			true,
			true,
			"",
		},
		{
			"pass - states query",
			func() []byte {
				input, err := s.precompile.Pack(liquidstake.StatesMethod)
				s.Require().NoError(err, "failed to pack input")
				return input
			},
			100000,
			true,
			true,
			"",
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext().WithBlockTime(time.Now())

			baseFee := s.nw.App.GetEVMKeeper().GetBaseFee(ctx)

			delegator := s.keyring.GetKey(0)

			contract := vm.NewPrecompile(delegator.Addr, s.precompile.Address(), uint256.NewInt(0), tc.gas)
			contractAddr := contract.Address()

			contract.Input = tc.malleate()

			txArgs := evmtypes.EvmTxArgs{
				ChainID:   evmtypes.GetEthChainConfig().ChainID,
				Nonce:     0,
				To:        &contractAddr,
				Amount:    nil,
				GasLimit:  tc.gas,
				GasPrice:  chainutil.ExampleMinGasPrices,
				GasFeeCap: baseFee,
				GasTipCap: big.NewInt(1),
				Accesses:  &ethtypes.AccessList{},
			}

			msg, err := s.factory.GenerateGethCoreMsg(delegator.Priv, txArgs)
			s.Require().NoError(err)

			proposerAddress := ctx.BlockHeader().ProposerAddress
			cfg, err := s.nw.App.GetEVMKeeper().EVMConfig(ctx, proposerAddress)
			s.Require().NoError(err, "failed to instantiate EVM config")

			stDB := statedb.New(
				ctx,
				s.nw.App.GetEVMKeeper(),
				statedb.NewEmptyTxConfig(),
			)
			evm := s.nw.App.GetEVMKeeper().NewEVM(
				ctx, *msg, cfg, nil, stDB,
			)

			precompiles, found, err := s.nw.App.GetEVMKeeper().GetPrecompileInstance(ctx, contractAddr)
			s.Require().NoError(err, "failed to instantiate precompile")
			s.Require().True(found, "not found precompile")
			evm.WithPrecompiles(precompiles.Map)

			bz, err := s.precompile.Run(evm, contract, tc.readOnly)

			if tc.expPass {
				s.Require().NoError(err, "expected no error when running the precompile")
				s.Require().NotNil(bz, "expected returned bytes not to be nil")
				s.Require().Greater(len(bz), 0, "expected non-empty response")
			} else {
				s.Require().Error(err, "expected error to be returned when running the precompile")
				s.Require().NotNil(bz, "expected returned revert bytes not to be nil")
				execErr := evmtypes.NewExecErrorWithReason(bz)
				s.Require().ErrorContains(execErr, tc.errContains)
			}
		})
	}
}

// TestAdminMethods tests the precompile's admin methods.
func (s *PrecompileTestSuite) TestAdminMethods() {
	var ctx sdk.Context
	testcases := []struct {
		name        string
		malleate    func() ([]byte, testkeyring.Key)
		gas         uint64
		expPass     bool
		errContains string
	}{
		{
			"UpdateParams_Basic_Positive",
			func() ([]byte, testkeyring.Key) {
				lsKeeper := s.nw.App.GetLiquidStakeKeeper()
				params := lsKeeper.GetParams(ctx)
				updatableParams := types.UpdatableParams{
					UnstakeFeeRate:        params.UnstakeFeeRate,
					LsmDisabled:           true,
					MinLiquidStakeAmount:  params.MinLiquidStakeAmount,
					CwLockedPoolAddress:   params.CwLockedPoolAddress,
					FeeAccountAddress:     params.FeeAccountAddress,
					AutocompoundFeeRate:   params.AutocompoundFeeRate,
					WhitelistAdminAddress: params.WhitelistAdminAddress,
				}

				paramsAfter := liquidstake.NewLiquidStakeUpdatableParamsOutput(&updatableParams)

				input, err := s.precompile.Pack(
					liquidstake.UpdateParamsMethod,
					paramsAfter,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			true,
			"",
		},
		{
			"UpdateWhitelistedValidators_Basic_Positive",
			func() ([]byte, testkeyring.Key) {
				lsKeeper := s.nw.App.GetLiquidStakeKeeper()
				paramsBeforeInternal := lsKeeper.GetParams(ctx)
				whitelisted := liquidstake.NewLiquidStakeWhitelistedValidatorsOutput(&paramsBeforeInternal)

				whitelisted[0].TargetWeight = big.NewInt(8000)

				whitelisted = append(whitelisted, liquidstake.WhitelistedValidator{
					ValidatorAddress: s.ValidatorAddr,
					TargetWeight:     big.NewInt(2000),
				})

				input, err := s.precompile.Pack(
					liquidstake.UpdateWhitelistedValidatorsMethod,
					whitelisted,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			true,
			"",
		},
		{
			"SetModulePaused_Basic_Positive",
			func() ([]byte, testkeyring.Key) {
				input, err := s.precompile.Pack(
					liquidstake.SetModulePausedMethod,
					false,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			true,
			"",
		},
		{
			"UpdateParams_Unauthorized_Caller",
			func() ([]byte, testkeyring.Key) {
				lsKeeper := s.nw.App.GetLiquidStakeKeeper()
				params := lsKeeper.GetParams(ctx)
				updatableParams := types.UpdatableParams{
					UnstakeFeeRate:        params.UnstakeFeeRate,
					LsmDisabled:           true,
					MinLiquidStakeAmount:  params.MinLiquidStakeAmount,
					CwLockedPoolAddress:   params.CwLockedPoolAddress,
					FeeAccountAddress:     params.FeeAccountAddress,
					AutocompoundFeeRate:   params.AutocompoundFeeRate,
					WhitelistAdminAddress: params.WhitelistAdminAddress,
				}

				paramsAfter := liquidstake.NewLiquidStakeUpdatableParamsOutput(&updatableParams)

				nonAdmin := s.keyring.GetKey(1)

				input, err := s.precompile.Pack(
					liquidstake.UpdateParamsMethod,
					paramsAfter,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, nonAdmin
			},
			800000,
			false,
			"invalid authority",
		},
		{
			"UpdateWhitelistedValidators_Unauthorized_Caller",
			func() ([]byte, testkeyring.Key) {
				lsKeeper := s.nw.App.GetLiquidStakeKeeper()
				paramsBeforeInternal := lsKeeper.GetParams(ctx)
				whitelisted := liquidstake.NewLiquidStakeWhitelistedValidatorsOutput(&paramsBeforeInternal)

				nonAdmin := s.keyring.GetKey(2)

				input, err := s.precompile.Pack(
					liquidstake.UpdateWhitelistedValidatorsMethod,
					whitelisted,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, nonAdmin
			},
			800000,
			false,
			"invalid authority",
		},
		{
			"SetModulePaused_Unauthorized_Caller",
			func() ([]byte, testkeyring.Key) {
				nonAdmin := s.keyring.GetKey(3)

				input, err := s.precompile.Pack(
					liquidstake.SetModulePausedMethod,
					true,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, nonAdmin
			},
			800000,
			false,
			"invalid authority",
		},
		{
			"UpdateWhitelistedValidators_Empty_Validators_List",
			func() ([]byte, testkeyring.Key) {
				emptyValidators := []liquidstake.WhitelistedValidator{}

				input, err := s.precompile.Pack(
					liquidstake.UpdateWhitelistedValidatorsMethod,
					emptyValidators,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			800000,
			false,
			"whitelisted validators list cannot be empty",
		},
		{
			"UpdateParams_Out_Of_Gas",
			func() ([]byte, testkeyring.Key) {
				lsKeeper := s.nw.App.GetLiquidStakeKeeper()
				params := lsKeeper.GetParams(ctx)
				updatableParams := types.UpdatableParams{
					UnstakeFeeRate:        params.UnstakeFeeRate,
					LsmDisabled:           true,
					MinLiquidStakeAmount:  params.MinLiquidStakeAmount,
					CwLockedPoolAddress:   params.CwLockedPoolAddress,
					FeeAccountAddress:     params.FeeAccountAddress,
					AutocompoundFeeRate:   params.AutocompoundFeeRate,
					WhitelistAdminAddress: params.WhitelistAdminAddress,
				}

				paramsAfter := liquidstake.NewLiquidStakeUpdatableParamsOutput(&updatableParams)

				input, err := s.precompile.Pack(
					liquidstake.UpdateParamsMethod,
					paramsAfter,
				)
				s.Require().NoError(err, "failed to pack input")

				return input, s.admin
			},
			10000,
			false,
			"out of gas",
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext().WithBlockTime(time.Now())

			baseFee := s.nw.App.GetEVMKeeper().GetBaseFee(ctx)

			input, sender := tc.malleate()

			contract := vm.NewPrecompile(sender.Addr, s.precompile.Address(), uint256.NewInt(0), tc.gas)
			contractAddr := contract.Address()

			contract.Input = input

			txArgs := evmtypes.EvmTxArgs{
				ChainID:   evmtypes.GetEthChainConfig().ChainID,
				Nonce:     0,
				To:        &contractAddr,
				Amount:    nil,
				GasLimit:  tc.gas,
				GasPrice:  chainutil.ExampleMinGasPrices,
				GasFeeCap: baseFee,
				GasTipCap: big.NewInt(1),
				Accesses:  &ethtypes.AccessList{},
			}

			msg, err := s.factory.GenerateGethCoreMsg(sender.Priv, txArgs)
			s.Require().NoError(err)

			proposerAddress := ctx.BlockHeader().ProposerAddress
			cfg, err := s.nw.App.GetEVMKeeper().EVMConfig(ctx, proposerAddress)
			s.Require().NoError(err, "failed to instantiate EVM config")

			stDB := statedb.New(
				ctx,
				s.nw.App.GetEVMKeeper(),
				statedb.NewEmptyTxConfig(),
			)
			evm := s.nw.App.GetEVMKeeper().NewEVM(
				ctx, *msg, cfg, nil, stDB,
			)

			precompiles, found, err := s.nw.App.GetEVMKeeper().GetPrecompileInstance(ctx, contractAddr)
			s.Require().NoError(err, "failed to instantiate precompile")
			s.Require().True(found, "not found precompile")
			evm.WithPrecompiles(precompiles.Map)

			bz, err := s.precompile.Run(evm, contract, false)

			if tc.expPass {
				s.Require().NoError(err, "expected no error when running the precompile")
				s.Require().NotNil(bz, "expected returned bytes not to be nil")
			} else {
				s.Require().Error(err, "expected error to be returned when running the precompile")
				s.Require().NotNil(bz, "expected returned revert bytes not to be nil")
				execErr := evmtypes.NewExecErrorWithReason(bz)
				s.Require().ErrorContains(execErr, tc.errContains)
			}
		})
	}
}
