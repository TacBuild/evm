package liquidstake_test

import (
	"math/big"
	"time"
	sdkmath "cosmossdk.io/math"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"
	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	"github.com/cosmos/evm/x/vm/statedb"

	sdk "github.com/cosmos/cosmos-sdk/types"

	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
)

func (s *LiquidStakePrecompileTestSuite) CreateAuthorization(ctx sdk.Context, granter, grantee sdk.AccAddress, authzType liquidstaketypes.AuthorizationType, coin *sdk.Coin) error {
	stakingAuthz, err := liquidstaketypes.NewStakeAuthorization(authzType, coin, nil, nil)
	if err != nil {
		return err
	}

	expiration := time.Now().Add(cmn.DefaultExpirationDuration).UTC()
	err = s.nw.App.AuthzKeeper.SaveGrant(ctx, grantee, granter, stakingAuthz, &expiration)
	if err != nil {
		return err
	}

	return nil
}

// CheckAllowanceChangeEvent checks the AllowanceChange event matches the expected values
func (s *LiquidStakePrecompileTestSuite) CheckAllowanceChangeEvent(log *ethtypes.Log, methods []string, amounts []*big.Int, granter, grantee common.Address) {
	s.Require().Equal(log.Address, s.precompile.Address())
	
	// Check event signature matches the one emitted
	event := s.precompile.ABI.Events[authorization.EventTypeAllowanceChange]
	s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))

	var allowanceEvent authorization.EventAllowanceChange
	err := cmn.UnpackLog(s.precompile.ABI, &allowanceEvent, authorization.EventTypeAllowanceChange, *log)
	s.Require().NoError(err)
	s.Require().Equal(grantee, allowanceEvent.Grantee)
	s.Require().Equal(granter, allowanceEvent.Granter)
	s.Require().Equal(len(methods), len(allowanceEvent.Methods))
	s.Require().Equal(len(amounts), len(allowanceEvent.Values))
	
	for i, method := range methods {
		s.Require().Equal(method, allowanceEvent.Methods[i])
		s.Require().Equal(amounts[i], allowanceEvent.Values[i])
	}
}

func (s *LiquidStakePrecompileTestSuite) TestLiquidStakeEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	method := s.precompile.Methods[liquidstake.LiquidStakeMethod]
	testCases := []struct {
		name        string
		malleate    func(delegator common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(delegator common.Address)
	}{
		{
			"success - LiquidStake event emitted correctly",
			func(delegator common.Address) []interface{} {
				return []interface{}{
					delegator,
					big.NewInt(1000000000000000000),
				}
			},
			false,
			"",
			func(delegator common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				// Check event signature matches the one emitted
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
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)

			contract := vm.NewContract(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, 200000)

			_, err := s.precompile.LiquidStake(ctx, delegator.Addr, contract, stDB, &method, tc.malleate(delegator.Addr))

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

func (s *LiquidStakePrecompileTestSuite) TestUpdateParamsEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	method := s.precompile.Methods[liquidstake.UpdateParams]
	testCases := []struct {
		name        string
		malleate    func(admin common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(admin common.Address)
	}{
		{
			"success - UpdateParams event emitted correctly",
			func(admin common.Address) []interface{} {
				// Create test params with all required fields
				params := liquidstake.LiquidStakeParams{
					LiquidBondDenom:       "stkTAC",
					WhitelistedValidators: []liquidstake.WhitelistedValidator{},
					UnstakeFeeRate:        big.NewInt(1000),
					LsmDisabled:           false,
					MinLiquidStakeAmount:  big.NewInt(1000000),
					CwLockedPoolAddress:   common.HexToAddress("0x1"),
					FeeAccountAddress:     common.HexToAddress("0x2"),
					AutocompoundFeeRate:   big.NewInt(500),
					WhitelistAdminAddress: admin,
					ModulePaused:          false,
				}
				return []interface{}{params}
			},
			false,
			"",
			func(admin common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				// Check event signature matches the one emitted
				event := s.precompile.ABI.Events[liquidstake.EventTypeUpdateParams]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var updateParamsEvent liquidstake.EventUpdateParams
				err := cmn.UnpackLog(s.precompile.ABI, &updateParamsEvent, liquidstake.EventTypeUpdateParams, *log)
				s.Require().NoError(err)
				s.Require().Equal("stkTAC", updateParamsEvent.Params.LiquidBondDenom)
				s.Require().Equal(admin, updateParamsEvent.Params.WhitelistAdminAddress)
				s.Require().Equal(false, updateParamsEvent.Params.ModulePaused)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			contract := vm.NewContract(vm.AccountRef(s.admin.Addr), s.precompile, common.U2560, 200000)
			contract.CallerAddress = s.admin.Addr

			_, err := s.precompile.UpdateParams(ctx, s.admin.Addr, contract, stDB, &method, tc.malleate(s.admin.Addr))

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

func (s *LiquidStakePrecompileTestSuite) TestUpdateWhitelistedValidatorsEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	method := s.precompile.Methods[liquidstake.UpdateWhitelistedValidators]
	testCases := []struct {
		name        string
		malleate    func(admin common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(admin common.Address)
	}{
		{
			"success - UpdateWhitelistedValidators event emitted correctly",
			func(admin common.Address) []interface{} {
				// Create test whitelisted validators
				validator1 := liquidstake.WhitelistedValidator{
					ValidatorAddress: s.ValidatorAddr,
					TargetWeight:     big.NewInt(10000),
				}
				whitelistedValidators := []liquidstake.WhitelistedValidator{validator1}
				return []interface{}{whitelistedValidators}
			},
			false,
			"",
			func(admin common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				// Check event signature matches the one emitted
				event := s.precompile.ABI.Events[liquidstake.EventTypeUpdateWhitelistedValidator]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var updateWhitelistEvent liquidstake.EventUpdateWhitelistedValidator
				err := cmn.UnpackLog(s.precompile.ABI, &updateWhitelistEvent, liquidstake.EventTypeUpdateWhitelistedValidator, *log)
				s.Require().NoError(err)
				s.Require().Equal(1, len(updateWhitelistEvent.WhitelistedValidators))
				s.Require().Equal(s.ValidatorAddr, updateWhitelistEvent.WhitelistedValidators[0].ValidatorAddress)
				s.Require().Equal(big.NewInt(10000), updateWhitelistEvent.WhitelistedValidators[0].TargetWeight)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			contract := vm.NewContract(vm.AccountRef(s.admin.Addr), s.precompile, common.U2560, 200000)
			contract.CallerAddress = s.admin.Addr

			_, err := s.precompile.UpdateWhitelistedValidators(ctx, s.admin.Addr, contract, stDB, &method, tc.malleate(s.admin.Addr))

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

func (s *LiquidStakePrecompileTestSuite) TestSetModulePausedEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	method := s.precompile.Methods[liquidstake.SetModulePaused]
	testCases := []struct {
		name        string
		malleate    func(admin common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(admin common.Address)
	}{
		{
			"success - SetModulePaused event emitted correctly (pause module)",
			func(admin common.Address) []interface{} {
				return []interface{}{true}
			},
			false,
			"",
			func(admin common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				// Check event signature matches the one emitted
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
			func(admin common.Address) []interface{} {
				return []interface{}{false}
			},
			false,
			"",
			func(admin common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				// Check event signature matches the one emitted
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
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			contract := vm.NewContract(vm.AccountRef(s.admin.Addr), s.precompile, common.U2560, 200000)
			contract.CallerAddress = s.admin.Addr

			_, err := s.precompile.SetModulePaused(ctx, s.admin.Addr, contract, stDB, &method, tc.malleate(s.admin.Addr))

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
//func (s *LiquidStakePrecompileTestSuite) TestStakeToLPEvent() {
//	var (
//		stDB *statedb.StateDB
//		ctx  sdk.Context
//	)
//	method := s.precompile.Methods[liquidstake.StakeToLPMethod]
//	testCases := []struct {
//		name        string
//		malleate    func(delegator common.Address) []interface{}
//		expErr      bool
//		errContains string
//		postCheck   func(delegator common.Address)
//	}{
//		{
//			"success - StakeToLP event emitted correctly",
//			func(delegator common.Address) []interface{} {
//				validator := s.validatorAdr
//				return []interface{}{
//					delegator,
//					validator,
//					big.NewInt(1000000000000000000),
//					big.NewInt(1000000000000000000),
//				}
//			},
//			false,
//			"",
//			func(delegator common.Address) {
//				s.SetupTest() 
//				log := stDB.Logs()[0]
//				s.Require().Equal(log.Address, s.precompile.Address())
//
//				// Check event signature matches the one emitted
//				event := s.precompile.ABI.Events[liquidstake.EventTypeStakeToLP]
//				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
//				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec
//
//				var stakeToLPEvent liquidstake.EventStakeToLP
//				err := cmn.UnpackLog(s.precompile.ABI, &stakeToLPEvent, liquidstake.EventTypeStakeToLP, *log)
//				s.Require().NoError(err)
//				s.Require().Equal(delegator, stakeToLPEvent.DelegatorAddress)
//				s.Require().Equal(big.NewInt(1000000000000000000), stakeToLPEvent.StakedAmount)
//				s.Require().Equal(big.NewInt(500000000000000000), stakeToLPEvent.LiquidAmount)
//			},
//		},
//	}
//
//	for _, tc := range testCases {
//		s.Run(tc.name, func() {
//			s.SetupTest() // reset
//			ctx = s.nw.GetContext()
//			stDB = s.nw.GetStateDB()
//
//			delegator := s.keyring.GetKey(0)
//
//			contract := vm.NewContract(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, 200000)
//
//			_, err := s.precompile.StakeToLP(ctx, delegator.Addr, contract, stDB, &method, tc.malleate(delegator.Addr))
//
//			if tc.expErr {
//				s.Require().Error(err)
//				s.Require().Contains(err.Error(), tc.errContains)
//			} else {
//				s.Require().NoError(err)
//				tc.postCheck(delegator.Addr)
//			}
//		})
//	}
//}

func (s *LiquidStakePrecompileTestSuite) TestLiquidUnstakeEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	method := s.precompile.Methods[liquidstake.LiquidUnstakeMethod]
	testCases := []struct {
		name        string
		malleate    func(delegator common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(delegator common.Address)
	}{
		{
			"success - LiquidUnstake event emitted correctly",
			func(delegator common.Address) []interface{} {
				_, err := s.nw.App.LiquidStakeKeeper.LiquidStake(ctx, liquidstaketypes.LiquidStakeProxyAcc, sdk.AccAddress(delegator.Bytes()), sdk.NewCoin(s.bondDenom, sdkmath.NewInt(1000000000000000000)))
				s.Require().NoError(err)

				s.Require().NoError(err, "failed to pack input")

				return []interface{}{
					delegator,
					big.NewInt(1000000000000000000),
				}
			},
			false,
			"",
			func(delegator common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())

				// Check event signature matches the one emitted
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
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)

			contract := vm.NewContract(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, 200000)

			_, err := s.precompile.LiquidUnstake(ctx, delegator.Addr, contract, stDB, &method, tc.malleate(delegator.Addr))

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

func (s *LiquidStakePrecompileTestSuite) TestApprovalEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	method := s.precompile.Methods[authorization.ApproveMethod]
	testCases := []struct {
		name        string
		malleate    func(grantee common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(granter, grantee common.Address)
	}{
		{
			"success - both liquidstake methods are present in the emitted event",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					abi.MaxUint256,
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.LiquidUnstakeMsg,
					},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				log := stDB.Logs()[0]
				s.Require().Equal(log.Address, s.precompile.Address())
				// Check event signature matches the one emitted
				event := s.precompile.ABI.Events[authorization.EventTypeApproval]
				s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec

				var approvalEvent authorization.EventApproval
				err := cmn.UnpackLog(s.precompile.ABI, &approvalEvent, authorization.EventTypeApproval, *log)
				s.Require().NoError(err)
				s.Require().Equal(grantee, approvalEvent.Grantee)
				s.Require().Equal(granter, approvalEvent.Granter)
				s.Require().Equal(abi.MaxUint256, approvalEvent.Value)
				s.Require().Equal(2, len(approvalEvent.Methods))
				s.Require().Equal(liquidstake.LiquidStakeMsg, approvalEvent.Methods[0])
				s.Require().Equal(liquidstake.LiquidUnstakeMsg, approvalEvent.Methods[1])
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			err := s.CreateAuthorization(ctx, granter.AccAddr, grantee.AccAddr, liquidstaketypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE, nil)
			s.Require().NoError(err)

			approveArgs := tc.malleate(grantee.Addr)
			_, err = s.precompile.Approve(ctx, granter.Addr, stDB, &method, approveArgs)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck(granter.Addr, grantee.Addr)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestIncreaseAllowanceEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	approvalMethod := s.precompile.Methods[authorization.ApproveMethod]
	method := s.precompile.Methods[authorization.IncreaseAllowanceMethod]
	testCases := []struct {
		name        string
		malleate    func(grantee common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(granter, grantee common.Address)
	}{
		{
			"success - increased allowance for liquidstake methods by 1 unit",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(1),
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.LiquidUnstakeMsg,
					},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				log := stDB.Logs()[1]
				methods := []string{
					liquidstake.LiquidStakeMsg,
					liquidstake.LiquidUnstakeMsg,
				}
				amounts := []*big.Int{
					big.NewInt(2),
					big.NewInt(2),
				}
				s.CheckAllowanceChangeEvent(log, methods, amounts, granter, grantee)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			err := s.CreateAuthorization(ctx, granter.AccAddr, grantee.AccAddr, liquidstaketypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE, nil)
			s.Require().NoError(err)

			// Approve first
			approveArgs := tc.malleate(grantee.Addr)
			_, err = s.precompile.Approve(ctx, granter.Addr, stDB, &approvalMethod, approveArgs)
			s.Require().NoError(err)

			// Increase allowance after approval
			_, err = s.precompile.IncreaseAllowance(ctx, granter.Addr, stDB, &method, approveArgs)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck(granter.Addr, grantee.Addr)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestDecreaseAllowanceEvent() {
	var (
		stDB *statedb.StateDB
		ctx  sdk.Context
	)
	approvalMethod := s.precompile.Methods[authorization.ApproveMethod]
	method := s.precompile.Methods[authorization.DecreaseAllowanceMethod]
	testCases := []struct {
		name        string
		malleate    func(grantee common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(granter, grantee common.Address)
	}{
		{
			"success - decreased allowance for liquidstake methods by 1 unit",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(1),
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.LiquidUnstakeMsg,
					},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				log := stDB.Logs()[1]
				methods := []string{
					liquidstake.LiquidStakeMsg,
					liquidstake.LiquidUnstakeMsg,
				}
				amounts := []*big.Int{
					big.NewInt(1),
					big.NewInt(1),
				}
				s.CheckAllowanceChangeEvent(log, methods, amounts, granter, grantee)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			err := s.CreateAuthorization(ctx, granter.AccAddr, grantee.AccAddr, liquidstaketypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE, nil)
			s.Require().NoError(err)

			// Approve first with 2 units
			args := []interface{}{
				grantee.Addr,
				big.NewInt(2),
				[]string{
					liquidstake.LiquidStakeMsg,
					liquidstake.LiquidUnstakeMsg,
				},
			}
			_, err = s.precompile.Approve(ctx, granter.Addr, stDB, &approvalMethod, args)
			s.Require().NoError(err)

			// Decrease allowance after approval
			_, err = s.precompile.DecreaseAllowance(ctx, granter.Addr, stDB, &method, tc.malleate(grantee.Addr))

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck(granter.Addr, grantee.Addr)
			}
		})
	}
}

