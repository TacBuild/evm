package liquidstake_test

import (
	"fmt"
	"math/big"

	"cosmossdk.io/math"

	"github.com/cosmos/evm/precompiles/authorization"
	cmn "github.com/cosmos/evm/precompiles/common"
	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	testutil "github.com/cosmos/evm/precompiles/testutil"

	sdk "github.com/cosmos/cosmos-sdk/types"

	testkeyring "github.com/cosmos/evm/testutil/integration/os/keyring"

	"github.com/cosmos/evm/x/vm/statedb"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/vm"
)

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


