package liquidstake_test

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"
	"github.com/cosmos/evm/precompiles/liquidstake"
	"github.com/cosmos/evm/x/vm/statedb"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (s *LiquidStakePrecompileTestSuite) TestApprove() {
	var (
		stDB   *statedb.StateDB
		ctx    sdk.Context
		method = s.precompile.Methods[authorization.ApproveMethod]
	)

	testCases := []struct {
		name        string
		malleate    func(grantee common.Address) []interface{}
		expError    bool
		errContains string
		postCheck   func(granter, grantee common.Address)
	}{
		{
			"fail - empty input args",
			func(grantee common.Address) []interface{} {
				return []interface{}{}
			},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 3, 0),
			func(granter, grantee common.Address) {},
		},
		{
			"fail - invalid grantee address",
			func(_ common.Address) []interface{} {
				return []interface{}{
					"invalid",
					abi.MaxUint256,
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			true,
			"invalid grantee",
			func(granter, grantee common.Address) {},
		},
		{
			"fail - invalid amount",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					"invalid",
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			true,
			"invalid amount",
			func(granter, grantee common.Address) {},
		},
		{
			"fail - invalid method type",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					abi.MaxUint256,
					[]string{"invalid_method"},
				}
			},
			true,
			"invalid msg type",
			func(granter, grantee common.Address) {},
		},
		{
			"success - approve liquid stake with max amount",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					abi.MaxUint256,
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				// Check that authorization was created
				authz, exp, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidStakeMsg)
				s.Require().NoError(err)
				s.Require().NotNil(authz)
				s.Require().NotNil(exp)

				stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
				s.Require().True(ok)
				s.Require().Nil(stakeAuthz.MaxTokens) // MaxUint256 should result in nil (unlimited)
			},
		},
		{
			"success - approve stake to LP with specific amount",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{liquidstake.StakeToLPMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				authz, exp, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.StakeToLPMsg)
				s.Require().NoError(err)
				s.Require().NotNil(authz)
				s.Require().NotNil(exp)

				stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
				s.Require().True(ok)
				s.Require().NotNil(stakeAuthz.MaxTokens)
				s.Require().Equal(big.NewInt(1000000), stakeAuthz.MaxTokens.Amount.BigInt())
			},
		},
		{
			"success - approve liquid unstake with specific amount",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(500000),
					[]string{liquidstake.LiquidUnstakeMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				authz, exp, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidUnstakeMsg)
				s.Require().NoError(err)
				s.Require().NotNil(authz)
				s.Require().NotNil(exp)

				stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
				s.Require().True(ok)
				s.Require().NotNil(stakeAuthz.MaxTokens)
				s.Require().Equal(big.NewInt(500000), stakeAuthz.MaxTokens.Amount.BigInt())
			},
		},
		{
			"success - approve multiple methods at once",
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(2000000),
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
				// Check all three authorizations were created
				for _, msgType := range []string{liquidstake.LiquidStakeMsg, liquidstake.StakeToLPMsg, liquidstake.LiquidUnstakeMsg} {
					authz, exp, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, msgType)
					s.Require().NoError(err, "failed for msgType: %s", msgType)
					s.Require().NotNil(authz)
					s.Require().NotNil(exp)

					stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
					s.Require().True(ok)
					s.Require().NotNil(stakeAuthz.MaxTokens)
					s.Require().Equal(big.NewInt(2000000), stakeAuthz.MaxTokens.Amount.BigInt())
				}
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			bz, err := s.precompile.Approve(ctx, granter.Addr, stDB, &method, tc.malleate(grantee.Addr))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)

				success, err := s.precompile.Unpack(authorization.ApproveMethod, bz)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)

				tc.postCheck(granter.Addr, grantee.Addr)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestRevoke() {
	var (
		stDB   *statedb.StateDB
		ctx    sdk.Context
		method = s.precompile.Methods[authorization.RevokeMethod]
	)

	testCases := []struct {
		name        string
		setup       func(granter, grantee common.Address)
		malleate    func(grantee common.Address) []interface{}
		expError    bool
		errContains string
		postCheck   func(granter, grantee common.Address)
	}{
		{
			"fail - empty input args",
			func(granter, grantee common.Address) {},
			func(grantee common.Address) []interface{} {
				return []interface{}{}
			},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
			func(granter, grantee common.Address) {},
		},
		{
			"fail - invalid grantee address",
			func(granter, grantee common.Address) {},
			func(_ common.Address) []interface{} {
				return []interface{}{
					"invalid",
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			true,
			"invalid grantee",
			func(granter, grantee common.Address) {},
		},
		{
			"fail - invalid method type",
			func(granter, grantee common.Address) {},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					[]string{"invalid_method"},
				}
			},
			true,
			"invalid msg type",
			func(granter, grantee common.Address) {},
		},
		{
			"success - revoke existing authorization",
			func(granter, grantee common.Address) {
				// First create an authorization
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
			},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				// Check that authorization was deleted
				_, _, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidStakeMsg)
				s.Require().Error(err) // Should error because authorization doesn't exist
			},
		},
		{
			"success - revoke multiple authorizations",
			func(granter, grantee common.Address) {
				// Create multiple authorizations
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.StakeToLPMsg,
						liquidstake.LiquidUnstakeMsg,
					},
				})
				s.Require().NoError(err)
			},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.StakeToLPMsg,
					},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				// Check that specified authorizations were deleted
				_, _, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidStakeMsg)
				s.Require().Error(err)
				_, _, err = authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.StakeToLPMsg)
				s.Require().Error(err)

				// Check that unspecified authorization still exists
				_, _, err = authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidUnstakeMsg)
				s.Require().NoError(err)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			tc.setup(granter.Addr, grantee.Addr)

			bz, err := s.precompile.Revoke(ctx, granter.Addr, stDB, &method, tc.malleate(grantee.Addr))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)

				success, err := s.precompile.Unpack(authorization.RevokeMethod, bz)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)

				tc.postCheck(granter.Addr, grantee.Addr)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestIncreaseAllowance() {
	var (
		stDB   *statedb.StateDB
		ctx    sdk.Context
		method = s.precompile.Methods[authorization.IncreaseAllowanceMethod]
	)

	testCases := []struct {
		name        string
		setup       func(granter, grantee common.Address)
		malleate    func(grantee common.Address) []interface{}
		expError    bool
		errContains string
		postCheck   func(granter, grantee common.Address)
	}{
		{
			"fail - empty input args",
			func(granter, grantee common.Address) {},
			func(grantee common.Address) []interface{} {
				return []interface{}{}
			},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 3, 0),
			func(granter, grantee common.Address) {},
		},
		{
			"fail - no existing authorization",
			func(granter, grantee common.Address) {},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(500000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			true,
			"does not exist",
			func(granter, grantee common.Address) {},
		},
		{
			"success - increase allowance for existing authorization",
			func(granter, grantee common.Address) {
				// First create an authorization with 1000000
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
			},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(500000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				// Check that allowance was increased to 1500000
				authz, _, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidStakeMsg)
				s.Require().NoError(err)

				stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
				s.Require().True(ok)
				s.Require().NotNil(stakeAuthz.MaxTokens)
				s.Require().Equal(big.NewInt(1500000), stakeAuthz.MaxTokens.Amount.BigInt())
			},
		},
		{
			"success - increase allowance for unlimited authorization (no-op)",
			func(granter, grantee common.Address) {
				// Create unlimited authorization
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					abi.MaxUint256,
					[]string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
			},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(500000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				// Check that it's still unlimited
				authz, _, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidStakeMsg)
				s.Require().NoError(err)

				stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
				s.Require().True(ok)
				s.Require().Nil(stakeAuthz.MaxTokens) // Should still be unlimited
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			tc.setup(granter.Addr, grantee.Addr)

			bz, err := s.precompile.IncreaseAllowance(ctx, granter.Addr, stDB, &method, tc.malleate(grantee.Addr))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)

				success, err := s.precompile.Unpack(authorization.IncreaseAllowanceMethod, bz)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)

				tc.postCheck(granter.Addr, grantee.Addr)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestDecreaseAllowance() {
	var (
		stDB   *statedb.StateDB
		ctx    sdk.Context
		method = s.precompile.Methods[authorization.DecreaseAllowanceMethod]
	)

	testCases := []struct {
		name        string
		setup       func(granter, grantee common.Address)
		malleate    func(grantee common.Address) []interface{}
		expError    bool
		errContains string
		postCheck   func(granter, grantee common.Address)
	}{
		{
			"fail - empty input args",
			func(granter, grantee common.Address) {},
			func(grantee common.Address) []interface{} {
				return []interface{}{}
			},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 3, 0),
			func(granter, grantee common.Address) {},
		},
		{
			"fail - no existing authorization",
			func(granter, grantee common.Address) {},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(500000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			true,
			"does not exist",
			func(granter, grantee common.Address) {},
		},
		{
			"fail - decrease amount greater than allowance",
			func(granter, grantee common.Address) {
				// Create authorization with 1000000
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
			},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(1500000), // Greater than allowance
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			true,
			"greater than the authorization limit",
			func(granter, grantee common.Address) {},
		},
		{
			"success - decrease allowance for existing authorization",
			func(granter, grantee common.Address) {
				// Create authorization with 1000000
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
			},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(300000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				// Check that allowance was decreased to 700000
				authz, _, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidStakeMsg)
				s.Require().NoError(err)

				stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
				s.Require().True(ok)
				s.Require().NotNil(stakeAuthz.MaxTokens)
				s.Require().Equal(big.NewInt(700000), stakeAuthz.MaxTokens.Amount.BigInt())
			},
		},
		{
			"success - decrease allowance for unlimited authorization (no-op)",
			func(granter, grantee common.Address) {
				// Create unlimited authorization
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					abi.MaxUint256,
					[]string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
			},
			func(grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					big.NewInt(500000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			false,
			"",
			func(granter, grantee common.Address) {
				// Check that it's still unlimited
				authz, _, err := authorization.CheckAuthzExists(ctx, s.nw.App.AuthzKeeper, grantee, granter, liquidstake.LiquidStakeMsg)
				s.Require().NoError(err)

				stakeAuthz, ok := authz.(*stakingtypes.StakeAuthorization)
				s.Require().True(ok)
				s.Require().Nil(stakeAuthz.MaxTokens) // Should still be unlimited
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			tc.setup(granter.Addr, grantee.Addr)

			bz, err := s.precompile.DecreaseAllowance(ctx, granter.Addr, stDB, &method, tc.malleate(grantee.Addr))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)

				success, err := s.precompile.Unpack(authorization.DecreaseAllowanceMethod, bz)
				s.Require().NoError(err)
				s.Require().Equal(success[0], true)

				tc.postCheck(granter.Addr, grantee.Addr)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestAllowance() {
	var (
		stDB   *statedb.StateDB
		ctx    sdk.Context
		method = s.precompile.Methods[authorization.AllowanceMethod]
	)

	testCases := []struct {
		name        string
		setup       func(granter, grantee common.Address)
		malleate    func(granter, grantee common.Address) []interface{}
		expError    bool
		errContains string
		postCheck   func(data []byte)
	}{
		{
			"fail - empty input args",
			func(granter, grantee common.Address) {},
			func(granter, grantee common.Address) []interface{} {
				return []interface{}{}
			},
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 3, 0),
			func(data []byte) {},
		},
		{
			"fail - no existing authorization",
			func(granter, grantee common.Address) {},
			func(granter, grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					granter,
					liquidstake.LiquidStakeMsg,
				}
			},
			true,
			"does not exist",
			func(data []byte) {},
		},
		{
			"success - get allowance for limited authorization",
			func(granter, grantee common.Address) {
				// Create authorization with specific amount
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					big.NewInt(1000000),
					[]string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
			},
			func(granter, grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					granter,
					liquidstake.LiquidStakeMsg,
				}
			},
			false,
			"",
			func(data []byte) {
				var allowanceOutput struct {
					Amount     *big.Int
					Expiration int64
				}
				err := s.precompile.UnpackIntoInterface(&allowanceOutput, authorization.AllowanceMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(big.NewInt(1000000), allowanceOutput.Amount)
				s.Require().Greater(allowanceOutput.Expiration, time.Now().Unix())
			},
		},
		{
			"success - get allowance for unlimited authorization",
			func(granter, grantee common.Address) {
				// Create unlimited authorization
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter, stDB, &approveMethod, []interface{}{
					grantee,
					abi.MaxUint256,
					[]string{liquidstake.StakeToLPMsg},
				})
				s.Require().NoError(err)
			},
			func(granter, grantee common.Address) []interface{} {
				return []interface{}{
					grantee,
					granter,
					liquidstake.StakeToLPMsg,
				}
			},
			false,
			"",
			func(data []byte) {
				var allowanceOutput struct {
					Amount     *big.Int
					Expiration int64
				}
				err := s.precompile.UnpackIntoInterface(&allowanceOutput, authorization.AllowanceMethod, data)
				s.Require().NoError(err)
				s.Require().Equal(abi.MaxUint256, allowanceOutput.Amount)
				s.Require().Greater(allowanceOutput.Expiration, time.Now().Unix())
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			tc.setup(granter.Addr, grantee.Addr)

			contract := vm.NewContract(vm.AccountRef(granter.Addr), s.precompile, common.U2560, 100000)
			bz, err := s.precompile.Allowance(ctx, &method, contract, tc.malleate(granter.Addr, grantee.Addr))

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)
				tc.postCheck(bz)
			}
		})
	}
}
