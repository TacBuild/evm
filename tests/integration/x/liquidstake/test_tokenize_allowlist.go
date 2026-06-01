package liquidstake

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/evm/x/liquidstake/types"
)

func (s *KeeperTestSuite) TestTokenizeSharesAllowlistOnlyAllowsLiquidStakeProxyAcc() {
	sk := s.nw.App.GetStakingKeeper()

	s.Require().True(sk.CanTokenizeShares(types.LiquidStakeProxyAcc))
	s.Require().False(sk.CanTokenizeShares(s.addrs[0]))
	s.Require().False(sk.CanTokenizeShares(s.delAddrs[0]))
}

func (s *KeeperTestSuite) TestTokenizeSharesAllowlistRejectsRegularDelegatorButAllowsProxy() {
	_, valAddrs := s.setupWhitelistedValidators(1, 1_000_000)

	ctx := s.ctx()
	sk := s.nw.App.GetStakingKeeper()
	msgServer := stakingkeeper.NewMsgServerImpl(sk)

	skParams, err := sk.GetParams(ctx)
	s.Require().NoError(err)

	regularDelegator := s.delAddrs[0]
	_, err = msgServer.TokenizeShares(ctx, &stakingtypes.MsgTokenizeShares{
		DelegatorAddress:    regularDelegator.String(),
		ValidatorAddress:    valAddrs[0].String(),
		Amount:              sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(1)),
		TokenizedShareOwner: regularDelegator.String(),
	})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)

	liquidStaker := s.delAddrs[1]
	_, err = s.keeper.LiquidStake(ctx, types.LiquidStakeProxyAcc, liquidStaker, sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(1_000_000)))
	s.Require().NoError(err)

	res, err := msgServer.TokenizeShares(ctx, &stakingtypes.MsgTokenizeShares{
		DelegatorAddress:    types.LiquidStakeProxyAcc.String(),
		ValidatorAddress:    valAddrs[0].String(),
		Amount:              sdk.NewCoin(skParams.BondDenom, sdkmath.NewInt(1)),
		TokenizedShareOwner: liquidStaker.String(),
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Require().True(res.Amount.Amount.IsPositive())
}
