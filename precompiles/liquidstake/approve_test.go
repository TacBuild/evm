package liquidstake_test

import (
	"fmt"
	"math/big"
	"time"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"

	"cosmossdk.io/math"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"
	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	testutil "github.com/cosmos/evm/precompiles/testutil"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkauthz "github.com/cosmos/cosmos-sdk/x/authz"

	testkeyring "github.com/cosmos/evm/testutil/integration/os/keyring"

	"github.com/cosmos/evm/x/vm/statedb"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/vm"
)

func (s *LiquidStakePrecompileTestSuite) TestDecreaseAllowance() {
	var (
		ctx  sdk.Context
		stDB *statedb.StateDB
	)
	method := s.precompile.Methods[authorization.DecreaseAllowanceMethod]

	testCases := []struct {
		name        string
		malleate    func(granter, grantee testkeyring.Key) []interface{}
		postCheck   func(granter, grantee testkeyring.Key, data []byte, inputArgs []interface{})
		gas         uint64
		expError    bool
		errContains string
	}{
		{
			"fail - empty input args",
			func(_, _ testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 3, 0),
		},
		{
			"fail - authorization does not exist",
			func(_, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					big.NewInt(15000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			"authorization to /tac.liquidstake.v1beta1.MsgLiquidStake for address",
		},
		{
			"fail - liquidstake authorization is a generic Authorization",
			func(granter, grantee testkeyring.Key) []interface{} {
				authz := sdkauthz.NewGenericAuthorization(liquidstake.LiquidStakeMsg)
				exp := time.Now().Add(time.Hour)
				err := s.nw.App.AuthzKeeper.SaveGrant(ctx, grantee.AccAddr, granter.AccAddr, authz, &exp)
				s.Require().NoError(err)
				return []interface{}{
					grantee.Addr,
					big.NewInt(15000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			sdkauthz.ErrUnknownAuthorizationType.Error(),
		},
		{
			"fail - decrease amount greater than allowance",
			func(granter, grantee testkeyring.Key) []interface{} {
				s.ApproveAndCheckAuthz(method, granter, grantee, liquidstake.LiquidStakeMsg, big.NewInt(1e18))
				return []interface{}{
					grantee.Addr,
					big.NewInt(2e18),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			"amount by which the allowance should be decreased is greater than the authorization limit",
		},
		{
			"success - decrease allowance",
			func(granter, grantee testkeyring.Key) []interface{} {
				s.ApproveAndCheckAuthz(method, granter, grantee, liquidstake.LiquidStakeMsg, big.NewInt(2e18))
				return []interface{}{
					grantee.Addr,
					big.NewInt(1e18),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, _ []byte, _ []interface{}) {
				authz, _ := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidStakeAuthz)
				s.Require().Equal(authz.MaxTokens, &sdk.Coin{Denom: s.bondDenom, Amount: math.NewInt(1e18)})
			},
			200000,
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			args := tc.malleate(granter, grantee)
			bz, err := s.precompile.DecreaseAllowance(ctx, granter.Addr, stDB, &method, args)

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)
				tc.postCheck(granter, grantee, bz, args)
			}
		})
	}
}



// ApproveAndCheckAuthz is a helper function to approve a given authorization method and check if the authorization was created.
func (s *LiquidStakePrecompileTestSuite) ApproveAndCheckAuthz(method abi.Method, granter, grantee testkeyring.Key, msgType string, amount *big.Int) {
	approveMethod := s.precompile.Methods[authorization.ApproveMethod]
	approveArgs := []interface{}{
		grantee.Addr,
		amount,
		[]string{msgType},
	}
	resp, err := s.precompile.Approve(s.nw.GetContext(), granter.Addr, s.nw.GetStateDB(), &approveMethod, approveArgs)
	s.Require().NoError(err)
	s.Require().Equal(resp, cmn.TrueValue)

	var authorizationType liquidstaketypes.AuthorizationType
	switch msgType {
	case liquidstake.LiquidStakeMsg:
		authorizationType = liquidstake.LiquidStakeAuthz
	case liquidstake.LiquidUnstakeMsg:
		authorizationType = liquidstake.LiquidUnstakeAuthz
	}

	auth, _ := CheckAuthorizationWithContext(s.nw.GetContext(), s.nw.App.AuthzKeeper, authorizationType, grantee.Addr, granter.Addr)
	s.Require().NotNil(auth)
	s.Require().Equal(auth.AuthorizationType, authorizationType)
	s.Require().Equal(auth.MaxTokens, &sdk.Coin{Denom: s.bondDenom, Amount: math.NewIntFromBigInt(amount)})
}

func (s *LiquidStakePrecompileTestSuite) TestRevoke() {
	var (
		ctx  sdk.Context
		stDB *statedb.StateDB
	)
	method := s.precompile.Methods[authorization.RevokeMethod]

	testCases := []struct {
		name        string
		malleate    func(granter, grantee testkeyring.Key) []interface{}
		postCheck   func(granter, grantee testkeyring.Key, data []byte)
		gas         uint64
		expError    bool
		errContains string
	}{
		{
			"fail - empty input args",
			func(_, _ testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			func(_, _ testkeyring.Key, _ []byte) {},
			200000,
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
		},
		{
			"fail - invalid message type",
			func(_, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					[]string{"invalid"},
				}
			},
			func(_, _ testkeyring.Key, _ []byte) {},
			200000,
			true,
			fmt.Sprintf(cmn.ErrInvalidMsgType, "liquidstake", "invalid"),
		},
		{
			"success - revoke liquid stake authorization",
			func(granter, grantee testkeyring.Key) []interface{} {
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter.Addr, stDB, &approveMethod, []interface{}{
					grantee.Addr, big.NewInt(1), []string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)

				// Check that the authorization exists
				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstaketypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)

				return []interface{}{
					grantee.Addr,
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte) {
				s.Require().Equal(data, cmn.TrueValue)

				// Check that the authorization is revoked
				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstaketypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE, grantee.Addr, granter.Addr)
				s.Require().Nil(authz)
				s.Require().Nil(expirationTime)
			},
			200000,
			false,
			"",
		},
		{
			"fail - should not revoke the approval when trying to revoke for a different message type",
			func(granter, grantee testkeyring.Key) []interface{} {
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter.Addr, stDB, &approveMethod, []interface{}{
					grantee.Addr, big.NewInt(1), []string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)

				return []interface{}{
					grantee.Addr,
					[]string{liquidstake.LiquidUnstakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte) {
				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstaketypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)
			},
			200000,
			true,
			"authorization not found",
		},
		{
			"fail - should return error if the approval does not exist",
			func(granter, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte) {},
			200000,
			true,
			"authorization not found",
		},
		{
			"fail - should not revoke the approval if sent by someone else than the granter",
			func(granter, grantee testkeyring.Key) []interface{} {
				approveMethod := s.precompile.Methods[authorization.ApproveMethod]
				_, err := s.precompile.Approve(ctx, granter.Addr, stDB, &approveMethod, []interface{}{
					grantee.Addr, big.NewInt(1), []string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)

				differentSender := s.keyring.GetKey(2)

				return []interface{}{
					differentSender.Addr,
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte) {
				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstaketypes.AuthorizationType_AUTHORIZATION_TYPE_STAKE, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)
			},
			200000,
			true,
			"authorization not found",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			args := tc.malleate(granter, grantee)
			bz, err := s.precompile.Revoke(ctx, granter.Addr, stDB, &method, args)

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)
				tc.postCheck(granter, grantee, bz)
			}
		})
	}
}

func (s *LiquidStakePrecompileTestSuite) TestIncreaseAllowance() {
	var (
		ctx  sdk.Context
		stDB *statedb.StateDB
	)
	method := s.precompile.Methods[authorization.IncreaseAllowanceMethod]

	testCases := []struct {
		name        string
		malleate    func(granter, grantee testkeyring.Key) []interface{}
		postCheck   func(granter, grantee testkeyring.Key, data []byte, inputArgs []interface{})
		gas         uint64
		expError    bool
		errContains string
	}{
		{
			"fail - empty input args",
			func(_, _ testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 3, 0),
		},
		{
			"fail - authorization does not exist",
			func(_, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					big.NewInt(15000),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			"does not exist or is expired",
		},
		{
			"success - no-op, allowance amount is already set to the maximum value",
			func(granter, grantee testkeyring.Key) []interface{} {
				approveArgs := []interface{}{
					grantee.Addr,
					abi.MaxUint256,
					[]string{liquidstake.LiquidStakeMsg},
				}
				resp, err := s.precompile.Approve(ctx, granter.Addr, stDB, &method, approveArgs)
				s.Require().NoError(err)
				s.Require().Equal(resp, cmn.TrueValue)

				authz, _ := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidStakeAuthz)
				var coin *sdk.Coin
				s.Require().Equal(authz.MaxTokens, coin)

				return []interface{}{
					grantee.Addr,
					big.NewInt(2e18),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			false,
			"",
		},
		{
			"success - increase allowance by specific amount",
			func(granter, grantee testkeyring.Key) []interface{} {
				s.ApproveAndCheckAuthz(method, granter, grantee, liquidstake.LiquidStakeMsg, big.NewInt(1e18))
				return []interface{}{
					grantee.Addr,
					big.NewInt(1e18),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, _ []byte, _ []interface{}) {
				authz, _ := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidStakeAuthz)
				s.Require().Equal(authz.MaxTokens, &sdk.Coin{Denom: s.bondDenom, Amount: math.NewInt(2e18)})
			},
			200000,
			false,
			"",
		},
		{
			"success - increase allowance",
			func(granter, grantee testkeyring.Key) []interface{} {
				s.ApproveAndCheckAuthz(method, granter, grantee, liquidstake.LiquidStakeMsg, big.NewInt(1e18))
				return []interface{}{
					grantee.Addr,
					big.NewInt(1e18),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, _ []byte, _ []interface{}) {
				authz, _ := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidStakeAuthz)
				s.Require().Equal(authz.MaxTokens, &sdk.Coin{Denom: s.bondDenom, Amount: math.NewInt(2e18)})
			},
			200000,
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			args := tc.malleate(granter, grantee)
			bz, err := s.precompile.IncreaseAllowance(ctx, granter.Addr, stDB, &method, args)

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)
				tc.postCheck(granter, grantee, bz, args)
			}
		})
	}
}


func (s *LiquidStakePrecompileTestSuite) TestApprove() {
	var (
		ctx  sdk.Context
		stDB *statedb.StateDB
	)
	method := s.precompile.Methods[authorization.ApproveMethod]

	testCases := []struct {
		name        string
		malleate    func(contract *vm.Contract, granter, grantee testkeyring.Key) []interface{}
		postCheck   func(granter, grantee testkeyring.Key, data []byte, inputArgs []interface{})
		gas         uint64
		expError    bool
		errContains string
	}{
		{
			"fail - empty input args",
			func(_ *vm.Contract, _, _ testkeyring.Key) []interface{} {
				return []interface{}{}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 3, 0),
		},
		{
			"fail - invalid message type",
			func(_ *vm.Contract, _, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					abi.MaxUint256,
					[]string{"invalid"},
				}
			},
			func(_, _ testkeyring.Key, _ []byte, _ []interface{}) {},
			200000,
			true,
			fmt.Sprintf(cmn.ErrInvalidMsgType, "liquidstake", "invalid"),
		},
		{
			"success - MsgDelegate with unlimited coins",
			func(_ *vm.Contract, _, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					abi.MaxUint256,
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte, _ []interface{}) {
				s.Require().Equal(data, cmn.TrueValue)
				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)

				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidStakeAuthz)
				var coin *sdk.Coin
				s.Require().Equal(authz.MaxTokens, coin)
			},
			20000,
			false,
			"",
		},
		{
			"success - MsgUndelegate with unlimited coins",
			func(_ *vm.Contract, _, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					abi.MaxUint256,
					[]string{liquidstake.LiquidUnstakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte, _ []interface{}) {
				s.Require().Equal(data, cmn.TrueValue)

				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidUnstakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidUnstakeAuthz)
				var coin *sdk.Coin
				s.Require().Equal(authz.MaxTokens, coin)
			},
			20000,
			false,
			"",
		},
		{
			"success - All liquidstake methods with certain amount of coins",
			func(_ *vm.Contract, _, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					big.NewInt(1e18),
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.LiquidUnstakeMsg,
					},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte, _ []interface{}) {
				s.Require().Equal(data, cmn.TrueValue)

				allAuthz, err := s.nw.App.AuthzKeeper.GetAuthorizations(ctx, grantee.AccAddr, granter.AccAddr)
				s.Require().NoError(err)
				s.Require().Len(allAuthz, 2)
			},
			20000,
			false,
			"",
		},
		{
			"success - remove MsgLiquidStake authorization",
			func(_ *vm.Contract, granter, grantee testkeyring.Key) []interface{} {
				res, err := s.precompile.Approve(ctx, granter.Addr, stDB, &method, []interface{}{
					grantee.Addr, big.NewInt(1), []string{liquidstake.LiquidStakeMsg},
				})
				s.Require().NoError(err)
				s.Require().Equal(res, cmn.TrueValue)

				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)

				return []interface{}{
					grantee.Addr,
					big.NewInt(0),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte, _ []interface{}) {
				s.Require().Equal(data, cmn.TrueValue)

				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().Nil(authz)
				s.Require().Nil(expirationTime)
			},
			200000,
			false,
			"",
		},
		{ //nolint:dupl
			"success - MsgLiquidStake with 1 ATOM as limit amount",
			func(_ *vm.Contract, _, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					big.NewInt(1e18),
					[]string{liquidstake.LiquidStakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte, _ []interface{}) {
				s.Require().Equal(data, cmn.TrueValue)

				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidStakeAuthz)
				s.Require().Equal(authz.MaxTokens, &sdk.Coin{Denom: s.bondDenom, Amount: math.NewInt(1e18)})
			},
			20000,
			false,
			"",
		},
		{ //nolint:dupl
			"success - MsgUnstake with 1 ATOM as limit amount",
			func(_ *vm.Contract, _, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					big.NewInt(1e18),
					[]string{liquidstake.LiquidUnstakeMsg},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte, _ []interface{}) {
				s.Require().Equal(data, cmn.TrueValue)

				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidUnstakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)

				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidUnstakeAuthz)
				s.Require().Equal(authz.MaxTokens, &sdk.Coin{Denom: s.nw.App.LiquidStakeKeeper.LiquidBondDenom(ctx), Amount: math.NewInt(1e18)})
			},
			20000,
			false,
			"",
		},
		{
			"success - MsgLiquidStake, and MsgLiquidUnstake with 1 ATOM as limit amount",
			func(_ *vm.Contract, _, grantee testkeyring.Key) []interface{} {
				return []interface{}{
					grantee.Addr,
					big.NewInt(1e18),
					[]string{
						liquidstake.LiquidStakeMsg,
						liquidstake.LiquidUnstakeMsg,
					},
				}
			},
			func(granter, grantee testkeyring.Key, data []byte, _ []interface{}) {
				s.Require().Equal(data, cmn.TrueValue)

				authz, expirationTime := CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidStakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)
				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidStakeAuthz)
				s.Require().Equal(authz.MaxTokens, &sdk.Coin{Denom: s.bondDenom, Amount: math.NewInt(1e18)})

				authz, expirationTime = CheckAuthorizationWithContext(ctx, s.nw.App.AuthzKeeper, liquidstake.LiquidUnstakeAuthz, grantee.Addr, granter.Addr)
				s.Require().NotNil(authz)
				s.Require().NotNil(expirationTime)

				s.Require().Equal(authz.AuthorizationType, liquidstake.LiquidUnstakeAuthz)
				s.Require().Equal(authz.MaxTokens, &sdk.Coin{Denom: s.nw.App.LiquidStakeKeeper.LiquidBondDenom(ctx), Amount: math.NewInt(1e18)})

				allAuthz, err := s.nw.App.AuthzKeeper.GetAuthorizations(s.nw.GetContext(), grantee.AccAddr, granter.AccAddr)
				s.Require().NoError(err)
				s.Require().Len(allAuthz, 2)
			},
			20000,
			false,
			"",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			ctx = s.nw.GetContext()
			stDB = s.nw.GetStateDB()

			granter := s.keyring.GetKey(0)
			grantee := s.keyring.GetKey(1)

			var contract *vm.Contract
			contract, ctx = testutil.NewPrecompileContract(s.T(), ctx, granter.Addr, s.precompile, tc.gas)

			args := tc.malleate(contract, granter, grantee)
			bz, err := s.precompile.Approve(ctx, granter.Addr, stDB, &method, args)

			if tc.expError {
				s.Require().ErrorContains(err, tc.errContains)
				s.Require().Empty(bz)
			} else {
				s.Require().NoError(err)
				s.Require().NotEmpty(bz)
				tc.postCheck(granter, grantee, bz, args)
			}
		})
	}
}


