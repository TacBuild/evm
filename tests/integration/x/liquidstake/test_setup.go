package liquidstake

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"time"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/suite"

	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/evm/testutil/integration/evm/factory"
	"github.com/cosmos/evm/testutil/integration/evm/grpc"
	"github.com/cosmos/evm/testutil/integration/evm/network"
	testkeyring "github.com/cosmos/evm/testutil/keyring"
	"github.com/cosmos/evm/x/liquidstake/keeper"
	"github.com/cosmos/evm/x/liquidstake/types"
)

var blockDuration = 6 * time.Second

// KeeperTestSuite is the integration test suite for x/liquidstake keeper.
type KeeperTestSuite struct {
	suite.Suite

	create  network.CreateEvmApp
	options []network.ConfigOption

	nw      *network.UnitTestNetwork
	factory factory.TxFactory
	handler grpc.Handler
	keyring testkeyring.Keyring

	keeper  keeper.Keeper
	querier keeper.Querier

	// pre-funded delegator addresses (10 accounts)
	addrs    []sdk.AccAddress
	delAddrs []sdk.AccAddress
	valAddrs []sdk.ValAddress
}

func NewKeeperTestSuite(create network.CreateEvmApp, options ...network.ConfigOption) *KeeperTestSuite {
	return &KeeperTestSuite{create: create, options: options}
}

func (s *KeeperTestSuite) SetupTest() {
	// 20 accounts: 10 generic + 10 delegator
	keys := testkeyring.New(20)

	opts := []network.ConfigOption{
		network.WithPreFundedAccounts(keys.GetAllAccAddrs()...),
		network.WithAmountOfValidators(5),
	}
	opts = append(opts, s.options...)
	s.nw = network.NewUnitTestNetwork(s.create, opts...)
	s.handler = grpc.NewIntegrationHandler(s.nw)
	s.factory = factory.New(s.nw, s.handler)
	s.keyring = keys

	s.keeper = s.nw.App.GetLiquidStakeKeeper()
	s.querier = keeper.Querier{Keeper: s.keeper}

	allAddrs := keys.GetAllAccAddrs()
	s.addrs = allAddrs[:10]
	s.delAddrs = allAddrs[10:]
	s.valAddrs = convertAddrsToValAddrs(s.delAddrs)

	// configure liquidstake params
	ctx := s.nw.GetContext()
	params := s.keeper.GetParams(ctx)
	params.UnstakeFeeRate = sdkmath.LegacyZeroDec()
	params.AutocompoundFeeRate = types.DefaultAutocompoundFeeRate
	params.ModulePaused = false
	params.LsmDisabled = true
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	// set MaxEntries / MaxValidators
	skParams, err := s.nw.App.GetStakingKeeper().GetParams(ctx)
	s.Require().NoError(err)
	skParams.MaxEntries = 7
	skParams.MaxValidators = 30
	s.Require().NoError(s.nw.App.GetStakingKeeper().SetParams(ctx, skParams))

	// Commit initial params so they survive the first NextBlock.
	s.Require().NoError(s.nw.CommitState())
}

// ctx is a shorthand to avoid verbose s.nw.GetContext() calls in tests.
func (s *KeeperTestSuite) ctx() sdk.Context {
	return s.nw.GetContext()
}

// CreateValidators creates N new validators with the given voting powers.
func (s *KeeperTestSuite) CreateValidators(powers []int64) ([]sdk.AccAddress, []sdk.ValAddress, []cryptotypes.PubKey) {
	s.T().Helper()
	s.Require().NoError(s.nw.NextBlock())

	num := len(powers)
	addrs := createIncrementalAccounts(num)
	valAddrs := convertAddrsToValAddrs(addrs)
	pks := createTestPubKeys(num)

	ctx := s.nw.GetContext()
	sk := s.nw.App.GetStakingKeeper()

	skParams, err := sk.GetParams(ctx)
	s.Require().NoError(err)
	skParams.ValidatorLiquidStakingCap = sdkmath.LegacyOneDec()
	s.Require().NoError(sk.SetParams(ctx, skParams))

	bondDenom := s.nw.GetBaseDenom()

	for i, power := range powers {
		s.Require().NoError(s.nw.FundAccount(addrs[i], sdk.NewCoins(
			sdk.NewCoin(bondDenom, sdkmath.NewInt(power+1_000_000)),
		)))

		val, err := stakingtypes.NewValidator(valAddrs[i].String(), pks[i], stakingtypes.Description{})
		s.Require().NoError(err)

		sk.SetValidator(ctx, val)
		s.Require().NoError(sk.SetValidatorByConsAddr(ctx, val))
		sk.SetNewValidatorByPowerIndex(ctx, val)
		s.Require().NoError(sk.Hooks().AfterValidatorCreated(ctx, valAddrs[i]))

		newShares, err := sk.Delegate(ctx, addrs[i], sdkmath.NewInt(power), stakingtypes.Unbonded, val, true)
		s.Require().NoError(err)
		s.Require().Equal(newShares.TruncateInt(), sdkmath.NewInt(power))

		msgValidatorBond := &stakingtypes.MsgValidatorBond{
			DelegatorAddress: addrs[i].String(),
			ValidatorAddress: val.OperatorAddress,
		}
		_, err = s.nw.App.MsgServiceRouter().Handler(msgValidatorBond)(ctx, msgValidatorBond)
		s.Require().NoError(err)
	}

	// Commit validator writes before advancing the block.
	s.Require().NoError(s.nw.CommitState())
	s.Require().NoError(s.nw.NextBlock())
	return addrs, valAddrs, pks
}

// advanceHeight advances the chain by n blocks.
func (s *KeeperTestSuite) advanceHeight(n int) {
	s.T().Helper()
	for range n {
		s.Require().NoError(s.nw.CommitState())
		s.Require().NoError(s.nw.NextBlockAfter(blockDuration))
	}
}

// liquidStaking calls keeper.LiquidStake and verifies the gTAC balance delta.
func (s *KeeperTestSuite) liquidStaking(liquidStaker sdk.AccAddress, stakingAmt sdkmath.Int) error {
	s.T().Helper()
	ctx := s.nw.GetContext()
	params := s.keeper.GetParams(ctx)
	bk := s.nw.App.GetBankKeeper()
	bondDenom := s.nw.GetBaseDenom()

	before := bk.GetBalance(ctx, liquidStaker, params.LiquidBondDenom).Amount
	mintAmt, err := s.keeper.LiquidStake(
		ctx,
		types.LiquidStakeProxyAcc,
		liquidStaker,
		sdk.NewCoin(bondDenom, stakingAmt),
	)
	if err != nil {
		return err
	}
	after := bk.GetBalance(ctx, liquidStaker, params.LiquidBondDenom).Amount
	s.Require().EqualValues(mintAmt, after.Sub(before))
	// Commit so gTAC supply and bank state survive the next NextBlock.
	return s.nw.CommitState()
}

// setupWhitelistedValidators whitelists the first n existing network validators
// in liquidstake params and calls UpdateLiquidValidatorSet.
// This uses already-bonded validators, which is required for liquid staking to work.
//
// NOTE: Genesis validators are created with DelegatorShares=1.0 and Tokens=1e18,
// giving a sub-unit shares/tokens ratio that breaks LSM TokenizeShares for small amounts.
// This function normalises the ratio to 1:1 (DelegatorShares = Tokens) so that
// LiquidUnstake and StakeToLP work correctly in tests.
func (s *KeeperTestSuite) setupWhitelistedValidators(n int, _ int64) ([]sdk.AccAddress, []sdk.ValAddress) {
	s.T().Helper()
	ctx := s.nw.GetContext()
	sk := s.nw.App.GetStakingKeeper()

	validators, err := sk.GetValidators(ctx, uint32(n))
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(validators), n, "not enough validators in network; increase WithAmountOfValidators")

	skParams, err := sk.GetParams(ctx)
	s.Require().NoError(err)
	skParams.ValidatorLiquidStakingCap = sdkmath.LegacyOneDec()
	s.Require().NoError(sk.SetParams(ctx, skParams))

	params := s.keeper.GetParams(ctx)
	params.WhitelistedValidators = nil
	params.ModulePaused = false

	valWeight := types.TotalValidatorWeight.Quo(sdkmath.NewInt(int64(n)))

	addrs := make([]sdk.AccAddress, n)
	valOpers := make([]sdk.ValAddress, n)
	for i := range n {
		val := validators[i]
		valAddr, err := sdk.ValAddressFromBech32(val.OperatorAddress)
		s.Require().NoError(err)
		valOpers[i] = valAddr
		addrs[i] = sdk.AccAddress(valAddr)

		// Normalise DelegatorShares so that shares/tokens = 1:1.
		// Genesis validators start with DelegatorShares=1.0 and Tokens=1e18, which
		// causes sub-unit shares for small amounts and breaks MsgTokenizeShares.
		if !val.Tokens.IsZero() && !val.DelegatorShares.Equal(sdkmath.LegacyNewDecFromInt(val.Tokens)) {
			val.DelegatorShares = sdkmath.LegacyNewDecFromInt(val.Tokens)
			sk.SetValidator(ctx, val)
			// Also update the genesis delegation to match the new shares.
			del, delErr := sk.GetDelegation(ctx, addrs[i], valAddr)
			if delErr == nil {
				del.Shares = val.DelegatorShares
				s.Require().NoError(sk.SetDelegation(ctx, del))
			}
		}

		s.keeper.SetLiquidValidator(ctx, types.LiquidValidator{
			OperatorAddress: val.OperatorAddress,
		})
		params.WhitelistedValidators = append(params.WhitelistedValidators, types.WhitelistedValidator{
			ValidatorAddress: val.OperatorAddress,
			TargetWeight:     valWeight,
		})
	}
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	// Commit so the whitelist and liquid-validator state survive the next
	// NextBlock's NewContextLegacy replacement (old CacheMultiStore behaviour).
	s.Require().NoError(s.nw.CommitState())

	return addrs, valOpers
}

// (used when we need a pre-existing gTAC balance without going through LiquidStake).
func (s *KeeperTestSuite) fundLiquidBondDenom(addr sdk.AccAddress, amt sdkmath.Int) {
	s.T().Helper()
	ctx := s.nw.GetContext()
	params := s.keeper.GetParams(ctx)
	coins := sdk.NewCoins(sdk.NewCoin(params.LiquidBondDenom, amt))
	s.Require().NoError(s.nw.App.GetBankKeeper().MintCoins(ctx, minttypes.ModuleName, coins))
	s.Require().NoError(s.nw.App.GetBankKeeper().SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, addr, coins))
}

// --- pure helpers ---

func convertAddrsToValAddrs(addrs []sdk.AccAddress) []sdk.ValAddress {
	valAddrs := make([]sdk.ValAddress, len(addrs))
	for i, addr := range addrs {
		valAddrs[i] = sdk.ValAddress(addr)
	}
	return valAddrs
}

func createIncrementalAccounts(n int) []sdk.AccAddress {
	var addrs []sdk.AccAddress
	var buf bytes.Buffer
	for i := 100; i < n+100; i++ {
		buf.WriteString("A58856F0FD53BF058B4909A21AEC019107BA6")
		buf.WriteString(strconv.Itoa(i))
		res, _ := sdk.AccAddressFromHexUnsafe(buf.String())
		addrs = append(addrs, res)
		buf.Reset()
	}
	return addrs
}

func createTestPubKeys(n int) []cryptotypes.PubKey {
	var pks []cryptotypes.PubKey
	var buf bytes.Buffer
	for i := 100; i < n+100; i++ {
		buf.WriteString("0B485CFC0EECC619440448436F8FC9DF40566F2369E72400281454CB552AF")
		buf.WriteString(strconv.Itoa(i))
		pks = append(pks, newPubKeyFromHex(buf.String()))
		buf.Reset()
	}
	return pks
}

func newPubKeyFromHex(pk string) cryptotypes.PubKey {
	pkBytes, err := hex.DecodeString(pk)
	if err != nil {
		panic(err)
	}
	if len(pkBytes) != ed25519.PubKeySize {
		panic(errorsmod.Wrap(sdkerrors.ErrInvalidPubKey, "invalid pubkey size"))
	}
	return &ed25519.PubKey{Key: pkBytes}
}
