package liquidstake_test

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"
	"github.com/cosmos/evm/precompiles/liquidstake"
	"github.com/cosmos/evm/precompiles/testutil"
	testkeyring "github.com/cosmos/evm/testutil/integration/os/keyring"
	cosmosevmutiltx "github.com/cosmos/evm/testutil/tx"
	"github.com/cosmos/evm/x/vm/statedb"
)

func (s *LiquidStakePrecompileTestSuite) TestLiquidStake() {
	var (
		stDB   *statedb.StateDB
		method = s.precompile.Methods[liquidstake.LiquidStakeMethod]
	)

	testCases := []struct {
		name          string
		malleate      func(delegator testkeyring.Key) []interface{}
		gas           uint64
		callerAddress *common.Address
		postCheck     func(data []byte)
		expError      bool
		errContains   string
	}{
		{
			"fail - empty input args",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
		},
		{
			"fail - invalid delegator address",
			func(_ testkeyring.Key) []interface{} {
				return []interface{}{
					"invalid",
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid delegator",
		},
		{
			"fail - invalid amount",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					"invalid",
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid amount",
		},
		{
			"fail - zero amount",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					big.NewInt(0),
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"amount must be positive",
		},
		{
			"fail - cannot be called from different address without authorization",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					big.NewInt(1000000),
				}
			},
			200000,
			func() *common.Address {
				addr, _ := cosmosevmutiltx.NewAddrKey()
				return &addr
			}(),
			func([]byte) {},
			true,
			"does not exist or is expired",
		},
		{
			"success - liquid stake with sufficient funds",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func(data []byte) {
				success, err := s.precompile.Unpack(liquidstake.LiquidStakeMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)

				// Check that liquid tokens were minted
				logs := stDB.Logs()
				s.Require().Greater(len(logs), 0)
			},
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()

			ctx := s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)

			var contract *vm.Contract
			contract, ctx = testutil.NewPrecompileContract(s.T(), ctx, delegator.Addr, s.precompile, tc.gas)
			if tc.callerAddress != nil {
				contract.CallerAddress = *tc.callerAddress
			}

			bz, err := s.precompile.LiquidStake(ctx, delegator.Addr, contract, stDB, &method, tc.malleate(delegator))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				tc.postCheck(bz)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestStakeToLP() {
	var (
		stDB   *statedb.StateDB
		method = s.precompile.Methods[liquidstake.StakeToLPMethod]
	)

	testCases := []struct {
		name          string
		malleate      func(delegator, validator testkeyring.Key) []interface{}
		gas           uint64
		callerAddress *common.Address
		postCheck     func(data []byte)
		expError      bool
		errContains   string
	}{
		{
			"fail - empty input args",
			func(delegator, validator testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 4, 0),
		},
		{
			"fail - invalid delegator address",
			func(_, validator testkeyring.Key) []interface{} {
				return []interface{}{
					"invalid",
					validator.Addr,
					big.NewInt(1000000),
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid delegator",
		},
		{
			"fail - invalid validator address",
			func(delegator, _ testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					"invalid",
					big.NewInt(1000000),
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid validator",
		},
		{
			"fail - invalid staked amount",
			func(delegator, validator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					validator.Addr,
					"invalid",
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid amount",
		},
		{
			"fail - invalid liquid amount",
			func(delegator, validator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					validator.Addr,
					big.NewInt(1000000),
					"invalid",
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid amount",
		},
		{
			"success - stake to LP with valid parameters",
			func(delegator, validator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					validator.Addr,
					big.NewInt(1000000),
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func(data []byte) {
				success, err := s.precompile.Unpack(liquidstake.StakeToLPMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)
			},
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()

			ctx := s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)
			validator := s.keyring.GetKey(1)

			var contract *vm.Contract
			contract, ctx = testutil.NewPrecompileContract(s.T(), ctx, delegator.Addr, s.precompile, tc.gas)
			if tc.callerAddress != nil {
				contract.CallerAddress = *tc.callerAddress
			}

			bz, err := s.precompile.StakeToLP(ctx, delegator.Addr, contract, stDB, &method, tc.malleate(delegator, validator))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				tc.postCheck(bz)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestLiquidUnstake() {
	var (
		stDB   *statedb.StateDB
		method = s.precompile.Methods[liquidstake.LiquidUnstakeMethod]
	)

	testCases := []struct {
		name          string
		malleate      func(delegator testkeyring.Key) []interface{}
		gas           uint64
		callerAddress *common.Address
		postCheck     func(data []byte)
		expError      bool
		errContains   string
	}{
		{
			"fail - empty input args",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
		},
		{
			"fail - invalid delegator address",
			func(_ testkeyring.Key) []interface{} {
				return []interface{}{
					"invalid",
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid delegator",
		},
		{
			"fail - invalid amount",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					"invalid",
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid amount",
		},
		{
			"fail - zero amount",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					big.NewInt(0),
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"amount must be positive",
		},
		{
			"success - liquid unstake with sufficient liquid tokens",
			func(delegator testkeyring.Key) []interface{} {
				return []interface{}{
					delegator.Addr,
					big.NewInt(1000000),
				}
			},
			200000,
			nil,
			func(data []byte) {
				success, err := s.precompile.Unpack(liquidstake.LiquidUnstakeMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)
			},
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()

			ctx := s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			delegator := s.keyring.GetKey(0)

			var contract *vm.Contract
			contract, ctx = testutil.NewPrecompileContract(s.T(), ctx, delegator.Addr, s.precompile, tc.gas)
			if tc.callerAddress != nil {
				contract.CallerAddress = *tc.callerAddress
			}

			bz, err := s.precompile.LiquidUnstake(ctx, delegator.Addr, contract, stDB, &method, tc.malleate(delegator))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				tc.postCheck(bz)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestUpdateParams() {
	var (
		stDB   *statedb.StateDB
		method = s.precompile.Methods[liquidstake.UpdateParamsMethod]
	)

	testCases := []struct {
		name          string
		malleate      func(authority testkeyring.Key) []interface{}
		gas           uint64
		callerAddress *common.Address
		postCheck     func(data []byte)
		expError      bool
		errContains   string
	}{
		{
			"fail - empty input args",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
		},
		{
			"fail - invalid authority address",
			func(_ testkeyring.Key) []interface{} {
				return []interface{}{
					"invalid",
					liquidstake.LiquidStakeParams{
						LiquidBondDenom:      "agatom",
						UnstakeFeeRate:       big.NewInt(100),
						MinLiquidStakeAmount: big.NewInt(1000),
						AutocompoundFeeRate:  big.NewInt(50),
						ModulePaused:         false,
						LsmDisabled:          false,
					},
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid type",
		},
		{
			"success - update params with valid authority",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{
					authority.Addr,
					liquidstake.LiquidStakeParams{
						LiquidBondDenom:       "agatom",
						UnstakeFeeRate:        big.NewInt(100),
						MinLiquidStakeAmount:  big.NewInt(1000),
						AutocompoundFeeRate:   big.NewInt(50),
						ModulePaused:          false,
						LsmDisabled:           false,
						WhiteListedValidators: []liquidstake.WhitelistedValidator{},
						CwLockedPoolAddress:   authority.Addr.String(),
						FeeAcountAddress:      authority.Addr.String(),
						WhitelistAdminAddress: authority.Addr.String(),
					},
				}
			},
			200000,
			nil,
			func(data []byte) {
				success, err := s.precompile.Unpack(liquidstake.UpdateParamsMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)
			},
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()

			ctx := s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			authority := s.keyring.GetKey(0)

			var contract *vm.Contract
			contract, ctx = testutil.NewPrecompileContract(s.T(), ctx, authority.Addr, s.precompile, tc.gas)
			if tc.callerAddress != nil {
				contract.CallerAddress = *tc.callerAddress
			}

			bz, err := s.precompile.UpdateParams(ctx, authority.Addr, contract, stDB, &method, tc.malleate(authority))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				tc.postCheck(bz)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestUpdateWhitelistedValidators() {
	var (
		stDB   *statedb.StateDB
		method = s.precompile.Methods[liquidstake.UpdateWhitelistedValidatorsMethod]
	)

	testCases := []struct {
		name          string
		malleate      func(authority testkeyring.Key) []interface{}
		gas           uint64
		callerAddress *common.Address
		postCheck     func(data []byte)
		expError      bool
		errContains   string
	}{
		{
			"fail - empty input args",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
		},
		{
			"fail - invalid authority address",
			func(_ testkeyring.Key) []interface{} {
				return []interface{}{
					"invalid",
					[]liquidstake.WhitelistedValidator{},
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid type",
		},
		{
			"success - update whitelisted validators",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{
					authority.Addr,
					[]liquidstake.WhitelistedValidator{
						{
							ValidatorAddress: "cosmosvaloper1abc123",
							TargetWeight:     big.NewInt(5000),
						},
						{
							ValidatorAddress: "cosmosvaloper1def456",
							TargetWeight:     big.NewInt(5000),
						},
					},
				}
			},
			200000,
			nil,
			func(data []byte) {
				success, err := s.precompile.Unpack(liquidstake.UpdateWhitelistedValidatorsMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)
			},
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()

			ctx := s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			authority := s.keyring.GetKey(0)

			var contract *vm.Contract
			contract, ctx = testutil.NewPrecompileContract(s.T(), ctx, authority.Addr, s.precompile, tc.gas)
			if tc.callerAddress != nil {
				contract.CallerAddress = *tc.callerAddress
			}

			bz, err := s.precompile.UpdateWhitelistedValidators(ctx, authority.Addr, contract, stDB, &method, tc.malleate(authority))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				tc.postCheck(bz)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestSetModulePaused() {
	var (
		stDB   *statedb.StateDB
		method = s.precompile.Methods[liquidstake.SetModulePausedMethod]
	)

	testCases := []struct {
		name          string
		malleate      func(authority testkeyring.Key) []interface{}
		gas           uint64
		callerAddress *common.Address
		postCheck     func(data []byte)
		expError      bool
		errContains   string
	}{
		{
			"fail - empty input args",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
		},
		{
			"fail - invalid authority address",
			func(_ testkeyring.Key) []interface{} {
				return []interface{}{
					"invalid",
					true,
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid type",
		},
		{
			"fail - invalid paused parameter",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{
					authority.Addr,
					"invalid",
				}
			},
			200000,
			nil,
			func([]byte) {},
			true,
			"invalid type",
		},
		{
			"success - pause module",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{
					authority.Addr,
					true,
				}
			},
			200000,
			nil,
			func(data []byte) {
				success, err := s.precompile.Unpack(liquidstake.SetModulePausedMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)
			},
			false,
			"",
		},
		{
			"success - unpause module",
			func(authority testkeyring.Key) []interface{} {
				return []interface{}{
					authority.Addr,
					false,
				}
			},
			200000,
			nil,
			func(data []byte) {
				success, err := s.precompile.Unpack(liquidstake.SetModulePausedMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)
			},
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()

			ctx := s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			authority := s.keyring.GetKey(0)

			var contract *vm.Contract
			contract, ctx = testutil.NewPrecompileContract(s.T(), ctx, authority.Addr, s.precompile, tc.gas)
			if tc.callerAddress != nil {
				contract.CallerAddress = *tc.callerAddress
			}

			bz, err := s.precompile.SetModulePaused(ctx, authority.Addr, contract, stDB, &method, tc.malleate(authority))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				tc.postCheck(bz)
			}
		})
	}
}
