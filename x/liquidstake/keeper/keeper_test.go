package keeper_test

//import (
//	"fmt"
//	"testing"
//	"time"
//
//	"cosmossdk.io/log"
//	"cosmossdk.io/math"
//	testhelpers "github.com/Asphere-xyz/tacchain/app/helpers"
//	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
//	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
//	sdk "github.com/cosmos/cosmos-sdk/types"
//	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
//	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
//	"github.com/cosmos/cosmos-sdk/x/crisis"
//	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
//	"github.com/cosmos/cosmos-sdk/x/mint"
//	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
//	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
//	"github.com/stretchr/testify/suite"
//
//	chain "github.com/Asphere-xyz/tacchain/app"
//	"github.com/cosmos/evm/x/liquidstake/keeper"
//	"github.com/cosmos/evm/x/liquidstake/types"
//	dbm "github.com/cosmos/cosmos-db"
//)
//
//var BlockTime = 6 * time.Second
//
//type KeeperTestSuite struct {
//	suite.Suite
//
//	app      *chain.TacChainApp
//	ctx      sdk.Context
//	keeper   keeper.Keeper
//	querier  keeper.Querier
//	addrs    []sdk.AccAddress
//	delAddrs []sdk.AccAddress
//	valAddrs []sdk.ValAddress
//}
//
//func TestKeeperTestSuite(t *testing.T) {
//	suite.Run(t, new(KeeperTestSuite))
//}
//
//func (s *KeeperTestSuite) SetupTest() {
//	db := dbm.NewMemDB()
//	logger := log.NewTestLogger(s.T())
//	app := chain.NewTacChainAppWithCustomOptions(s.T(), false, 0, chain.SetupOptions{
//		Logger:  logger.With("instance", "first"),
//		DB:      db,
//		AppOpts: simtestutil.NewAppOptionsWithFlagHome(s.T().TempDir()),
//	})
//	s.app = app
//	s.ctx = s.app.BaseApp.NewContext(false)
//	stakingParams := stakingtypes.DefaultParams()
//	stakingParams.MaxEntries = 7
//	stakingParams.MaxValidators = 30
//	s.Require().NoError(s.app.StakingKeeper.SetParams(s.ctx, stakingParams))
//
//	s.keeper = s.app.LiquidStakeKeeper
//	s.querier = keeper.Querier{Keeper: s.keeper}
//	s.addrs = chain.AddTestAddrs(s.app, s.ctx, 10, math.NewInt(1_000_000_000))
//	s.delAddrs = chain.AddTestAddrs(s.app, s.ctx, 10, math.NewInt(1_000_000_000))
//	s.valAddrs = chain.ConvertAddrsToValAddrs(s.delAddrs)
//
//	s.ctx = s.ctx.WithBlockHeight(100).WithBlockTime(chain.ParseTime("2022-03-01T00:00:00Z"))
//	params := s.keeper.GetParams(s.ctx)
//	params.UnstakeFeeRate = sdk.ZeroDec()
//	params.AutocompoundFeeRate = types.DefaultAutocompoundFeeRate
//	s.Require().NoError(s.keeper.SetParams(s.ctx, params))
//	s.keeper.UpdateLiquidValidatorSet(s.ctx, true)
//	// call mint.BeginBlocker for init k.SetLastBlockTime(ctx, ctx.BlockTime())
//	mint.BeginBlocker(s.ctx, s.app.MintKeeper, minttypes.DefaultInflationCalculationFn)
//}
//
//func (s *KeeperTestSuite) TearDownTest() {
//	// invariant check
//	crisis.EndBlocker(s.ctx, *s.app.CrisisKeeper)
//}
//
//func (s *KeeperTestSuite) CreateValidators(powers []int64) ([]sdk.AccAddress, []sdk.ValAddress, []cryptotypes.PubKey) {
//	s.app.BeginBlocker(s.ctx)
//	num := len(powers)
//	addrs := testhelpers.AddTestAddrsIncremental(s.app, s.ctx, num, math.NewInt(10000000000000))
//	valAddrs := testhelpers.ConvertAddrsToValAddrs(addrs)
//	pks := testhelpers.CreateTestPubKeys(num)
//	skParams, err := s.app.StakingKeeper.GetParams(s.ctx)
//	s.Require().NoError(err)
//	skParams.ValidatorLiquidStakingCap = sdk.OneDec()
//	_ = s.app.StakingKeeper.SetParams(s.ctx, skParams)
//	for i, power := range powers {
//		val, err := stakingtypes.NewValidator(valAddrs[i].String(), pks[i], stakingtypes.Description{})
//		s.Require().NoError(err)
//		s.app.StakingKeeper.SetValidator(s.ctx, val)
//		err = s.app.StakingKeeper.SetValidatorByConsAddr(s.ctx, val)
//		s.Require().NoError(err)
//		s.app.StakingKeeper.SetNewValidatorByPowerIndex(s.ctx, val)
//		_ = s.app.StakingKeeper.Hooks().AfterValidatorCreated(s.ctx, valAddrs[i])
//		newShares, err := s.app.StakingKeeper.Delegate(s.ctx, addrs[i], math.NewInt(power), stakingtypes.Unbonded, val, true)
//		s.Require().NoError(err)
//		s.Require().Equal(newShares.TruncateInt(), math.NewInt(power))
//		msgValidatorBond := &stakingtypes.MsgValidatorBond{
//			DelegatorAddress: addrs[i].String(),
//			ValidatorAddress: val.OperatorAddress,
//		}
//		handler := s.app.MsgServiceRouter().Handler(msgValidatorBond)
//		_, err = handler(s.ctx, msgValidatorBond)
//		s.Require().NoError(err)
//	}
//
//	s.app.EndBlocker(s.ctx)
//	return addrs, valAddrs, pks
//}
//
//func (s *KeeperTestSuite) liquidStaking(liquidStaker sdk.AccAddress, stakingAmt math.Int) error {
//	ctx, writeCache := s.ctx.CacheContext()
//	params := s.keeper.GetParams(ctx)
//
//	gtacBalanceBefore := s.app.BankKeeper.GetBalance(
//		ctx, liquidStaker, params.LiquidBondDenom,
//	).Amount
//
//	gTACMintAmt, err := s.keeper.LiquidStake(
//		ctx,
//		types.LiquidStakeProxyAcc,
//		liquidStaker,
//		sdk.NewCoin(sdk.DefaultBondDenom, stakingAmt),
//	)
//	if err != nil {
//		return err
//	}
//
//	gtacBalanceAfter := s.app.BankKeeper.GetBalance(
//		ctx, liquidStaker, params.LiquidBondDenom,
//	).Amount
//
//	s.Require().NoError(err)
//	s.Require().EqualValues(
//		gTACMintAmt, gtacBalanceAfter.Sub(gtacBalanceBefore),
//	)
//	writeCache()
//
//	return nil
//}
//
//func (s *KeeperTestSuite) liquidUnstaking(
//	liquidStaker sdk.AccAddress,
//	ubdGTACAmt math.Int,
//	ubdComplete bool,
//) error {
//	ctx := s.ctx
//	params := s.keeper.GetParams(ctx)
//
//	balanceBefore := s.app.BankKeeper.GetBalance(
//		ctx,
//		liquidStaker,
//		sdk.DefaultBondDenom,
//	).Amount
//
//	ubdTime, unbondingAmt, _, unbondedAmt, err := s.liquidUnstakingWithResult(
//		liquidStaker,
//		sdk.NewCoin(params.LiquidBondDenom, ubdGTACAmt),
//	)
//	if err != nil {
//		return err
//	}
//
//	if ubdComplete {
//		alv := s.keeper.GetActiveLiquidValidators(ctx, params.WhitelistedValsMap())
//		ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 200).
//			WithBlockTime(ubdTime.Add(1))
//
//		// EndBlock of staking keeper, mature UBD
//		s.app.StakingKeeper.BlockValidatorUpdates(ctx)
//
//		balanceCompleteUBD := s.app.BankKeeper.GetBalance(
//			ctx,
//			liquidStaker,
//			sdk.DefaultBondDenom,
//		)
//		for _, v := range alv {
//			_, err := s.app.StakingKeeper.GetUnbondingDelegation(
//				ctx,
//				liquidStaker,
//				v.GetOperator(),
//			)
//			s.Require().Error(err)
//		}
//
//		s.Require().EqualValues(
//			balanceCompleteUBD.Amount,
//			balanceBefore.Add(unbondingAmt).Add(unbondedAmt),
//		)
//	}
//
//	return nil
//}
//
//func (s *KeeperTestSuite) liquidUnstakingWithResult(
//	liquidStaker sdk.AccAddress, unstakingGTAC sdk.Coin,
//) (time.Time, math.Int, []stakingtypes.UnbondingDelegation, math.Int, error) {
//	ctx, writeCache := s.ctx.CacheContext()
//	params := s.keeper.GetParams(ctx)
//	alv := s.keeper.GetActiveLiquidValidators(ctx, params.WhitelistedValsMap())
//
//	balanceBefore := s.app.BankKeeper.GetBalance(
//		ctx, liquidStaker, sdk.DefaultBondDenom,
//	).Amount
//	gtacBalanceBefore := s.app.BankKeeper.GetBalance(
//		ctx, liquidStaker, params.LiquidBondDenom,
//	).Amount
//
//	ubdTime, unbondingAmt, ubds, unbondedAmt, err := s.keeper.LiquidUnstake(
//		ctx, types.LiquidStakeProxyAcc, liquidStaker, unstakingGTAC,
//	)
//	if err != nil {
//		return ubdTime, unbondingAmt, ubds, unbondedAmt, err
//	}
//
//	balanceAfter := s.app.BankKeeper.GetBalance(
//		ctx, liquidStaker, sdk.DefaultBondDenom,
//	).Amount
//	gtacBalanceAfter := s.app.BankKeeper.GetBalance(
//		ctx, liquidStaker, params.LiquidBondDenom,
//	).Amount
//	s.Require().EqualValues(
//		unstakingGTAC.Amount, gtacBalanceBefore.Sub(gtacBalanceAfter),
//	)
//
//	if unbondedAmt.IsPositive() {
//		s.Require().EqualValues(
//			unbondedAmt, balanceAfter.Sub(balanceBefore),
//		)
//	}
//
//	for _, v := range alv {
//		_, err := s.app.StakingKeeper.GetUnbondingDelegation(
//			ctx, liquidStaker, v.GetOperator(),
//		)
//		s.Require().NoError(err)
//	}
//
//	writeCache()
//	return ubdTime, unbondingAmt, ubds, unbondedAmt, err
//}
//
//func (s *KeeperTestSuite) RequireNetAmountStateZero() {
//	nas, err := s.keeper.GetNetAmountState(s.ctx)
//	s.Require().NoError(err)
//	s.Require().EqualValues(nas.MintRate, sdk.ZeroDec())
//	s.Require().EqualValues(nas.GtacTotalSupply, sdk.ZeroInt())
//	s.Require().EqualValues(nas.NetAmount, sdk.ZeroDec())
//	s.Require().EqualValues(nas.TotalDelShares, sdk.ZeroDec())
//	s.Require().EqualValues(nas.TotalLiquidTokens, sdk.ZeroInt())
//	s.Require().EqualValues(nas.TotalRemainingRewards, sdk.ZeroDec())
//	s.Require().EqualValues(nas.TotalUnbondingBalance, sdk.ZeroDec())
//	s.Require().EqualValues(nas.ProxyAccBalance, sdk.ZeroInt())
//}
//
//// advance block time and height for complete redelegations and unbondings
//func (s *KeeperTestSuite) completeRedelegationUnbonding() {
//	s.ctx = s.ctx.WithBlockHeight(s.ctx.BlockHeight() + 100).
//		WithBlockTime(s.ctx.BlockTime().Add(stakingtypes.DefaultUnbondingTime))
//	s.app.EndBlocker(s.ctx)
//	reds, err := s.app.StakingKeeper.GetRedelegations(s.ctx, types.LiquidStakeProxyAcc, 100)
//	s.Require().NoError(err)
//	s.Require().Len(reds, 0)
//	ubds, err := s.app.StakingKeeper.GetUnbondingDelegations(s.ctx, types.LiquidStakeProxyAcc, 100)
//	s.Require().NoError(err)
//	s.Require().Len(ubds, 0)
//}
//
//func (s *KeeperTestSuite) redelegationsErrorCount(redelegations []types.Redelegation) int {
//	errCnt := 0
//	for _, red := range redelegations {
//		if red.Error != nil {
//			errCnt++
//		}
//	}
//	return errCnt
//}
//
//func (s *KeeperTestSuite) printRedelegationsLiquidTokens() {
//	redsIng, err := s.app.StakingKeeper.GetRedelegations(s.ctx, types.LiquidStakeProxyAcc, 50)
//	s.Require().NoError(err)
//	if len(redsIng) != 0 {
//		fmt.Println("[Redelegations]")
//		for i, red := range redsIng {
//			fmt.Println("\tRedelegation #", i+1)
//			fmt.Println("\t\tDelegatorAddress: ", red.DelegatorAddress)
//			fmt.Println("\t\tValidatorSrcAddress : ", red.ValidatorSrcAddress)
//			fmt.Println("\t\tValidatorDstAddress: ", red.ValidatorDstAddress)
//			fmt.Println("\t\tEntries: ")
//			for _, e := range red.Entries {
//				fmt.Println("\t\t\tCreationHeight: ", e.CreationHeight)
//				fmt.Println("\t\t\tCompletionTime: ", e.CompletionTime)
//				fmt.Println("\t\t\tInitialBalance: ", e.InitialBalance)
//				fmt.Println("\t\t\tSharesDst: ", e.SharesDst)
//			}
//		}
//		fmt.Println("")
//	}
//	liquidVals := s.keeper.GetAllLiquidValidators(s.ctx)
//	if len(liquidVals) != 0 {
//		fmt.Println("[LiquidValidators]")
//		for _, v := range s.keeper.GetAllLiquidValidators(s.ctx) {
//			fmt.Printf("   OperatorAddress %s; LiquidTokens: %s\n",
//				v.OperatorAddress, v.GetLiquidTokens(s.ctx, *s.app.StakingKeeper, false))
//		}
//	}
//}
//
//func (s *KeeperTestSuite) advanceHeight(height int, _ bool) {
//	feeCollector := s.app.AccountKeeper.GetModuleAddress(
//		authtypes.FeeCollectorName,
//	)
//
//	for i := 0; i < height; i++ {
//		s.ctx = s.ctx.WithBlockHeight(s.ctx.BlockHeight() + 1).
//			WithBlockTime(s.ctx.BlockTime().Add(BlockTime))
//
//		mint.BeginBlocker(s.ctx, s.app.MintKeeper, minttypes.DefaultInflationCalculationFn)
//		feeCollectorBalance := s.app.BankKeeper.GetAllBalances(
//			s.ctx, feeCollector,
//		)
//		rewardsToBeDistributed := feeCollectorBalance.AmountOf(
//			sdk.DefaultBondDenom,
//		)
//
//		// mimic distribution.BeginBlock (AllocateTokens, get rewards from
//		// feeCollector, AllocateTokensToValidator, add remaining to feePool)
//		err := s.app.BankKeeper.SendCoinsFromModuleToModule(
//			s.ctx, authtypes.FeeCollectorName, distrtypes.ModuleName,
//			feeCollectorBalance,
//		)
//
//		s.Require().NoError(err)
//		totalRewards := sdk.ZeroDec()
//		totalPower := int64(0)
//		s.app.StakingKeeper.IterateBondedValidatorsByPower(
//			s.ctx,
//			func(index int64, validator stakingtypes.ValidatorI) (stop bool) {
//				consPower := validator.GetConsensusPower(
//					s.app.StakingKeeper.PowerReduction(s.ctx),
//				)
//				totalPower = totalPower + consPower
//				return false
//			},
//		)
//
//		if totalPower != 0 {
//			s.app.StakingKeeper.IterateBondedValidatorsByPower(
//				s.ctx,
//				func(index int64, validator stakingtypes.ValidatorI) (stop bool) {
//					consPower := validator.GetConsensusPower(
//						s.app.StakingKeeper.PowerReduction(s.ctx),
//					)
//					powerFraction := math.LegacyNewDec(consPower).QuoTruncate(
//						math.LegacyNewDec(totalPower),
//					)
//					reward := rewardsToBeDistributed.ToLegacyDec().MulTruncate(
//						powerFraction,
//					)
//
//					s.app.DistrKeeper.AllocateTokensToValidator(
//						s.ctx, validator,
//						sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: reward}},
//					)
//
//					totalRewards = totalRewards.Add(reward)
//					return false
//				},
//			)
//		}
//
//		remaining := rewardsToBeDistributed.ToLegacyDec().Sub(totalRewards)
//		s.Require().False(remaining.GT(math.LegacyNewDec(1)))
//		feePool, err := s.app.DistrKeeper.FeePool.Get(s.ctx)
//		if err != nil {
//			s.Require().NoError(err)
//		}
//		feePool.CommunityPool = feePool.CommunityPool.Add(
//			sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: remaining}}...,
//		)
//
//		s.app.DistrKeeper.FeePool.Set(s.ctx, feePool)
//
//		_, err = s.app.StakingKeeper.EndBlocker(s.ctx)
//		s.Require().NoError(err)
//	}
//}
//
//// doubleSign, tombstone, slash, jail
//func (s *KeeperTestSuite) doubleSign(valOper sdk.ValAddress, consAddr sdk.ConsAddress) {
//	liquidValidator, found := s.keeper.GetLiquidValidator(s.ctx, valOper)
//	s.Require().True(found)
//	val, err := s.app.StakingKeeper.GetValidator(s.ctx, valOper)
//	s.Require().NoError(err)
//	tokens := val.Tokens
//	liquidTokens := liquidValidator.GetLiquidTokens(s.ctx, *s.app.StakingKeeper, false)
//
//	// check sign info
//	info, err := s.app.SlashingKeeper.GetValidatorSigningInfo(s.ctx, consAddr)
//	s.Require().NoError(err)
//	s.Require().Equal(info.Address, consAddr.String())
//
//	// make evidence
//	// evidence := &evidencetypes.Equivocation{
//	// 	// Height: 0,
//	// 	// Time:   time.Unix(0, 0),
//	// 	Height:           s.ctx.BlockHeight(),
//	// 	Time:             s.ctx.BlockTime(),
//	// 	Power:            s.app.StakingKeeper.TokensToConsensusPower(s.ctx, tokens),
//	// 	ConsensusAddress: consAddr.String(),
//	// }
//
//	// Double sign
//	// TODO: Upgrade Cosmos SDK
//	// s.app.EvidenceKeeper.HandleEquivocationEvidence(s.ctx, evidence)
//	// HandleEquivocationEvidence call below functions
//	// s.app.SlashingKeeper.Slash()
//	// s.app.SlashingKeeper.Jail(s.ctx, consAddr)
//	// s.app.SlashingKeeper.JailUntil(s.ctx, consAddr, evidencetypes.DoubleSignJailEndTime)
//	// s.app.SlashingKeeper.Tombstone(s.ctx, consAddr)
//
//	// should be jailed and tombstoned
//	_, err = s.app.StakingKeeper.Validator(s.ctx, liquidValidator.GetOperator())
//	s.Require().Error(err)
//	s.Require().True(s.app.SlashingKeeper.IsTombstoned(s.ctx, consAddr))
//
//	// check tombstoned on sign info
//	info, err = s.app.SlashingKeeper.GetValidatorSigningInfo(s.ctx, consAddr)
//	s.Require().NoError(err)
//	s.Require().True(info.Tombstoned)
//	val, _ = s.app.StakingKeeper.GetValidator(s.ctx, valOper)
//	s.Require().True(s.keeper.IsTombstoned(s.ctx, val))
//	liquidTokensSlashed := liquidValidator.GetLiquidTokens(s.ctx, *s.app.StakingKeeper, false)
//	tokensSlashed := val.Tokens
//	s.Require().True(tokensSlashed.LT(tokens))
//	s.Require().True(liquidTokensSlashed.LT(liquidTokens))
//
//	s.app.StakingKeeper.BlockValidatorUpdates(s.ctx)
//	val, _ = s.app.StakingKeeper.GetValidator(s.ctx, valOper)
//
//	// set unbonding status, no more rewards before return Bonded
//	s.Require().Equal(val.Status, stakingtypes.Unbonding)
//}
//
//func (s *KeeperTestSuite) createContinuousVestingAccount(
//	from, to sdk.AccAddress, amt sdk.Coins,
//	startTime, endTime time.Time,
//) vestingtypes.ContinuousVestingAccount {
//	baseAccount := s.app.AccountKeeper.NewAccountWithAddress(s.ctx, to)
//	_, ok := baseAccount.(*authtypes.BaseAccount)
//	s.Require().True(ok)
//	baseVestingAccount, err := vestingtypes.NewBaseVestingAccount(
//		baseAccount.(*authtypes.BaseAccount), amt, endTime.Unix(),
//	)
//	s.Require().NoError(err)
//
//	cVestingAcc := vestingtypes.NewContinuousVestingAccountRaw(
//		baseVestingAccount, startTime.Unix(),
//	)
//
//	s.app.AccountKeeper.SetAccount(s.ctx, cVestingAcc)
//	err = s.app.BankKeeper.SendCoins(s.ctx, from, to, amt)
//	s.Require().NoError(err)
//
//	return *cVestingAcc
//}
//
//func (s *KeeperTestSuite) fundAddr(addr sdk.AccAddress, amt sdk.Coins) {
//	err := s.app.BankKeeper.MintCoins(s.ctx, "mint", amt)
//	s.Require().NoError(err)
//	err = s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", addr, amt)
//	s.Require().NoError(err)
//}
