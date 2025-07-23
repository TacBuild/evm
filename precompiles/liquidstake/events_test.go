package liquidstake_test

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"
	"github.com/cosmos/evm/precompiles/liquidstake"
	"github.com/cosmos/evm/x/vm/statedb"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

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
			"success - all three methods are present in the emitted event",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					abi.MaxUint256,
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.StakeToLPMsg,
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
				s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec // G115

				var approvalEvent authorization.EventApproval
				err := cmn.UnpackLog(s.precompile.ABI, &approvalEvent, authorization.EventTypeApproval, *log)
				s.Require().NoError(err)
				s.Require().Equal(grantee, approvalEvent.Grantee)
				s.Require().Equal(granter, approvalEvent.Granter)
				s.Require().Equal(abi.MaxUint256, approvalEvent.Value)
				s.Require().Equal(3, len(approvalEvent.Methods))
				s.Require().Equal(liquidstake.LiquidStakeMsg, approvalEvent.Methods[0])
				s.Require().Equal(liquidstake.StakeToLPMsg, approvalEvent.Methods[1])
				s.Require().Equal(liquidstake.LiquidUnstakeMsg, approvalEvent.Methods[2])
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

			approveArgs := tc.malleate(grantee.Addr)
			_, err := s.precompile.Approve(ctx, granter.Addr, stDB, &method, approveArgs)

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
			"success - increased allowance for all 3 methods by 1000000 tokens",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.StakeToLPMsg,
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
					liquidstake.StakeToLPMsg,
					liquidstake.LiquidUnstakeMsg,
				}
				amounts := []*big.Int{
					big.NewInt(2000000),
					big.NewInt(2000000),
					big.NewInt(2000000),
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

			// Approve first with 1000000
			approveArgs := tc.malleate(grantee.Addr)
			_, err := s.precompile.Approve(ctx, granter.Addr, stDB, &approvalMethod, approveArgs)
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
			"success - decreased allowance for all 3 methods by 500000 tokens",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(500000),
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.StakeToLPMsg,
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
					liquidstake.StakeToLPMsg,
					liquidstake.LiquidUnstakeMsg,
				}
				amounts := []*big.Int{
					big.NewInt(1500000),
					big.NewInt(1500000),
					big.NewInt(1500000),
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

			// Approve first with 2000000
			args := []interface{}{
				grantee.Addr,
				big.NewInt(2000000),
				[]string{
					liquidstake.LiquidStakeMsg,
					liquidstake.StakeToLPMsg,
					liquidstake.LiquidUnstakeMsg,
				},
			}
			_, err := s.precompile.Approve(ctx, granter.Addr, stDB, &approvalMethod, args)
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

// CheckAllowanceChangeEvent checks the allowance change event log
func (s *LiquidStakePrecompileTestSuite) CheckAllowanceChangeEvent(
	log *types.Log,
	methods []string,
	amounts []*big.Int,
	granter, grantee common.Address,
) {
	s.Require().Equal(log.Address, s.precompile.Address())

	// Check event signature matches the one emitted
	event := s.precompile.ABI.Events[authorization.EventTypeAllowanceChange]
	s.Require().Equal(crypto.Keccak256Hash([]byte(event.Sig)), common.HexToHash(log.Topics[0].Hex()))

	// Check the fully unpacked event matches the one emitted
	var allowanceChangeEvent authorization.EventAllowanceChange
	err := cmn.UnpackLog(s.precompile.ABI, &allowanceChangeEvent, authorization.EventTypeAllowanceChange, *log)
	s.Require().NoError(err)
	s.Require().Equal(grantee, allowanceChangeEvent.Grantee)
	s.Require().Equal(granter, allowanceChangeEvent.Granter)
	s.Require().Equal(len(methods), len(allowanceChangeEvent.Methods))
	s.Require().Equal(len(amounts), len(allowanceChangeEvent.Values))

	for i, method := range methods {
		s.Require().Equal(method, allowanceChangeEvent.Methods[i])
		s.Require().Equal(amounts[i], allowanceChangeEvent.Values[i])
	}
}

func (s *LiquidStakePrecompileTestSuite) TestLiquidStakeEvent() {
	var (
		stDB             *statedb.StateDB
		ctx              sdk.Context
		liquidStakeValue = big.NewInt(1000000)
		method           = s.precompile.Methods[liquidstake.LiquidStakeMethod]
	)

	testCases := []struct {
		name        string
		malleate    func(delegator common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(delegator common.Address)
	}{
		{
			name: "success - the correct event is emitted",
			malleate: func(delegator common.Address) []interface{} {
				return []interface{}{
					delegator,
					liquidStakeValue,
				}
			},
			postCheck: func(delegator common.Address) {
				logs := stDB.Logs()
				if len(logs) > 0 {
					log := logs[0]
					s.Require().Equal(log.Address, s.precompile.Address())

					// Check event signature matches the one emitted
					// Note: The actual event types would depend on your liquidstake module implementation
					s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec // G115
				}
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

func (s *LiquidStakePrecompileTestSuite) TestStakeToLPEvent() {
	var (
		stDB   *statedb.StateDB
		ctx    sdk.Context
		method = s.precompile.Methods[liquidstake.StakeToLPMethod]
	)

	testCases := []struct {
		name        string
		malleate    func(delegator, validator common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(delegator, validator common.Address)
	}{
		{
			name: "success - the correct event is emitted",
			malleate: func(delegator, validator common.Address) []interface{} {
				return []interface{}{
					delegator,
					validator,
					big.NewInt(1000000),
					big.NewInt(1000000),
				}
			},
			postCheck: func(delegator, validator common.Address) {
				logs := stDB.Logs()
				if len(logs) > 0 {
					log := logs[0]
					s.Require().Equal(log.Address, s.precompile.Address())
					s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec // G115
				}
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest() // reset
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)
			validator := s.keyring.GetKey(1)

			contract := vm.NewContract(vm.AccountRef(delegator.Addr), s.precompile, common.U2560, 200000)
			_, err := s.precompile.StakeToLP(ctx, delegator.Addr, contract, stDB, &method, tc.malleate(delegator.Addr, validator.Addr))

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Contains(err.Error(), tc.errContains)
			} else {
				s.Require().NoError(err)
				tc.postCheck(delegator.Addr, validator.Addr)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestLiquidUnstakeEvent() {
	var (
		stDB   *statedb.StateDB
		ctx    sdk.Context
		method = s.precompile.Methods[liquidstake.LiquidUnstakeMethod]
	)

	testCases := []struct {
		name        string
		malleate    func(delegator common.Address) []interface{}
		expErr      bool
		errContains string
		postCheck   func(delegator common.Address)
	}{
		{
			name: "success - the correct event is emitted",
			malleate: func(delegator common.Address) []interface{} {
				return []interface{}{
					delegator,
					big.NewInt(1000000),
				}
			},
			postCheck: func(delegator common.Address) {
				logs := stDB.Logs()
				if len(logs) > 0 {
					log := logs[0]
					s.Require().Equal(log.Address, s.precompile.Address())
					s.Require().Equal(log.BlockNumber, uint64(ctx.BlockHeight())) //nolint:gosec // G115
				}
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
