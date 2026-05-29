package liquidstake

import (
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	cmn "github.com/cosmos/evm/precompiles/common"
	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	"github.com/cosmos/evm/x/vm/statedb"

	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
)

func (s *PrecompileTestSuite) TestLiquidStakeEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	testCases := []struct {
		name        string
		malleate    func(delegator common.Address) *liquidstaketypes.MsgLiquidStake
		expErr      bool
		errContains string
		postCheck   func(delegator common.Address)
	}{
		{
			"success - LiquidStake event emitted correctly",
			func(delegator common.Address) *liquidstaketypes.MsgLiquidStake {
				return &liquidstaketypes.MsgLiquidStake{
					DelegatorAddress: sdk.AccAddress(delegator.Bytes()).String(),
					Amount:           sdk.NewCoin(s.bondDenom, sdkmath.NewInt(1000000000000000000)),
				}
			},
			false,
			"",
			func(delegator common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				event := s.precompile.ABI.Events[liquidstake.EventTypeLiquidStake]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var liquidStakeEvent liquidstake.EventLiquidStake
				err := cmn.UnpackLog(s.precompile.ABI, &liquidStakeEvent, liquidstake.EventTypeLiquidStake, *log)
				s.Require().NoError(err)
				s.Require().Equal(delegator, liquidStakeEvent.DelegatorAddress)
				s.Require().Equal(big.NewInt(1000000000000000000), liquidStakeEvent.Amount)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)
			msg := tc.malleate(delegator.Addr)

			err := s.precompile.EmitLiquidStakeEvent(ctx, stDB, msg, delegator.Addr)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck(delegator.Addr)
			}
		})
	}
}

func (s *PrecompileTestSuite) TestUpdateParamsEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	testCases := []struct {
		name        string
		malleate    func(admin common.Address) *liquidstaketypes.MsgUpdateParams
		expErr      bool
		errContains string
		postCheck   func(admin common.Address)
	}{
		{
			"success - UpdateParams event emitted correctly",
			func(admin common.Address) *liquidstaketypes.MsgUpdateParams {
				return &liquidstaketypes.MsgUpdateParams{
					Authority: sdk.AccAddress(admin.Bytes()).String(),
					Params: liquidstaketypes.UpdatableParams{
						UnstakeFeeRate:        sdkmath.LegacyNewDecWithPrec(1, 3),
						MinLiquidStakeAmount:  sdkmath.NewInt(1000000),
						CwLockedPoolAddress:   sdk.AccAddress(common.HexToAddress("0x1").Bytes()).String(),
						FeeAccountAddress:     sdk.AccAddress(common.HexToAddress("0x2").Bytes()).String(),
						AutocompoundFeeRate:   sdkmath.LegacyNewDecWithPrec(5, 4),
						WhitelistAdminAddress: sdk.AccAddress(admin.Bytes()).String(),
					},
				}
			},
			false,
			"",
			func(admin common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				event := s.precompile.ABI.Events[liquidstake.EventTypeUpdateParams]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var updateParamsEvent liquidstake.EventUpdateParams
				err := cmn.UnpackLog(s.precompile.ABI, &updateParamsEvent, liquidstake.EventTypeUpdateParams, *log)
				s.Require().NoError(err)
				s.Require().Equal(admin, updateParamsEvent.Params.WhitelistAdminAddress)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			msg := tc.malleate(s.admin.Addr)

			err := s.precompile.EmitUpdateParamsEvent(ctx, stDB, msg)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck(s.admin.Addr)
			}
		})
	}
}

func (s *PrecompileTestSuite) TestUpdateWhitelistedValidatorsEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	testCases := []struct {
		name        string
		malleate    func() *liquidstaketypes.MsgUpdateWhitelistedValidators
		expErr      bool
		errContains string
		postCheck   func()
	}{
		{
			"success - UpdateWhitelistedValidators event emitted correctly",
			func() *liquidstaketypes.MsgUpdateWhitelistedValidators {
				return &liquidstaketypes.MsgUpdateWhitelistedValidators{
					Authority: sdk.AccAddress(s.admin.Addr.Bytes()).String(),
					WhitelistedValidators: []liquidstaketypes.WhitelistedValidator{
						{
							ValidatorAddress: s.liquidValidator.OperatorAddress,
							TargetWeight:     sdkmath.NewInt(10000),
						},
					},
				}
			},
			false,
			"",
			func() {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				event := s.precompile.ABI.Events[liquidstake.EventTypeUpdateWhitelistedValidator]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var updateWhitelistEvent liquidstake.EventUpdateWhitelistedValidator
				err := cmn.UnpackLog(s.precompile.ABI, &updateWhitelistEvent, liquidstake.EventTypeUpdateWhitelistedValidator, *log)
				s.Require().NoError(err)
				s.Require().Equal(1, len(updateWhitelistEvent.WhitelistedValidators))
				s.Require().Equal(s.liquidValidatorAddr, updateWhitelistEvent.WhitelistedValidators[0].ValidatorAddress)
				s.Require().Equal(big.NewInt(10000), updateWhitelistEvent.WhitelistedValidators[0].TargetWeight)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			msg := tc.malleate()

			err := s.precompile.EmitUpdateWhitelistedValidatorEvent(ctx, stDB, msg)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck()
			}
		})
	}
}

func (s *PrecompileTestSuite) TestSetModulePausedEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	testCases := []struct {
		name        string
		malleate    func() *liquidstaketypes.MsgSetModulePaused
		expErr      bool
		errContains string
		postCheck   func()
	}{
		{
			"success - SetModulePaused event emitted correctly (pause module)",
			func() *liquidstaketypes.MsgSetModulePaused {
				return &liquidstaketypes.MsgSetModulePaused{
					Authority: sdk.AccAddress(s.admin.Addr.Bytes()).String(),
					IsPaused:  true,
				}
			},
			false,
			"",
			func() {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				event := s.precompile.ABI.Events[liquidstake.EventTypeSetModulePaused]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var setModulePausedEvent liquidstake.EventSetModulePaused
				err := cmn.UnpackLog(s.precompile.ABI, &setModulePausedEvent, liquidstake.EventTypeSetModulePaused, *log)
				s.Require().NoError(err)
				s.Require().Equal(true, setModulePausedEvent.IsPaused)
			},
		},
		{
			"success - SetModulePaused event emitted correctly (unpause module)",
			func() *liquidstaketypes.MsgSetModulePaused {
				return &liquidstaketypes.MsgSetModulePaused{
					Authority: sdk.AccAddress(s.admin.Addr.Bytes()).String(),
					IsPaused:  false,
				}
			},
			false,
			"",
			func() {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				event := s.precompile.ABI.Events[liquidstake.EventTypeSetModulePaused]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var setModulePausedEvent liquidstake.EventSetModulePaused
				err := cmn.UnpackLog(s.precompile.ABI, &setModulePausedEvent, liquidstake.EventTypeSetModulePaused, *log)
				s.Require().NoError(err)
				s.Require().Equal(false, setModulePausedEvent.IsPaused)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			msg := tc.malleate()

			err := s.precompile.EmitSetModulePausedEvent(ctx, stDB, msg)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck()
			}
		})
	}
}

func (s *PrecompileTestSuite) TestLiquidUnstakeEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	testCases := []struct {
		name        string
		malleate    func(delegator common.Address) *liquidstaketypes.MsgLiquidUnstake
		expErr      bool
		errContains string
		postCheck   func(delegator common.Address)
	}{
		{
			"success - LiquidUnstake event emitted correctly",
			func(delegator common.Address) *liquidstaketypes.MsgLiquidUnstake {
				return &liquidstaketypes.MsgLiquidUnstake{
					DelegatorAddress: sdk.AccAddress(delegator.Bytes()).String(),
					Amount:           sdk.NewCoin(s.bondDenom, sdkmath.NewInt(1000000000000000000)),
				}
			},
			false,
			"",
			func(delegator common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				event := s.precompile.ABI.Events[liquidstake.EventTypeLiquidUnstake]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var liquidUnstakeEvent liquidstake.EventLiquidUnstake
				err := cmn.UnpackLog(s.precompile.ABI, &liquidUnstakeEvent, liquidstake.EventTypeLiquidUnstake, *log)
				s.Require().NoError(err)
				s.Require().Equal(delegator, liquidUnstakeEvent.DelegatorAddress)
				s.Require().Equal(big.NewInt(1000000000000000000), liquidUnstakeEvent.Amount)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)
			msg := tc.malleate(delegator.Addr)

			err := s.precompile.EmitLiquidUnstakeEvent(ctx, stDB, msg, delegator.Addr)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck(delegator.Addr)
			}
		})
	}
}
