package liquidstake

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"

	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	"github.com/cosmos/evm/testutil/integration/evm/factory"
	"github.com/cosmos/evm/testutil/integration/evm/grpc"
	"github.com/cosmos/evm/testutil/integration/evm/network"
	testkeyring "github.com/cosmos/evm/testutil/keyring"
	erc20types "github.com/cosmos/evm/x/erc20/types"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
)

type PrecompileTestSuite struct {
	suite.Suite

	create      network.CreateEvmApp
	options     []network.ConfigOption
	nw          *network.UnitTestNetwork
	factory     factory.TxFactory
	grpcHandler grpc.Handler
	keyring     testkeyring.Keyring

	bondDenom  string
	precompile *liquidstake.Precompile

	liquidValidatorAddr common.Address
	liquidValidator     stakingtypes.Validator
	liquidBondERC20Addr common.Address

	ValidatorAddr common.Address
	Validator     stakingtypes.Validator

	admin testkeyring.Key
}

func NewPrecompileTestSuite(create network.CreateEvmApp, options ...network.ConfigOption) *PrecompileTestSuite {
	return &PrecompileTestSuite{
		create:  create,
		options: options,
	}
}

func (s *PrecompileTestSuite) SetupTest() {
	keyring := testkeyring.New(10)
	opts := []network.ConfigOption{
		network.WithPreFundedAccounts(keyring.GetAllAccAddrs()...),
	}
	opts = append(opts, s.options...)
	nw := network.NewUnitTestNetwork(s.create, opts...)
	grpcHandler := grpc.NewIntegrationHandler(nw)
	txFactory := factory.New(nw, grpcHandler)

	ctx := nw.GetContext()
	sk := nw.App.GetStakingKeeper()
	bondDenom, err := sk.BondDenom(ctx)
	if err != nil {
		panic(err)
	}

	s.bondDenom = bondDenom
	s.factory = txFactory
	s.grpcHandler = grpcHandler
	s.keyring = keyring
	s.nw = nw

	lsKeeper := s.nw.App.GetLiquidStakeKeeper()

	validators, err := s.nw.App.GetStakingKeeper().GetValidators(ctx, 2)
	if err != nil {
		panic(err)
	}
	s.liquidValidator = validators[0]

	params := lsKeeper.GetParams(ctx)
	params.ModulePaused = false
	params.LiquidBondDenom = "agatom"

	valAddr, err := sdk.ValAddressFromBech32(validators[0].OperatorAddress)
	if err != nil {
		panic(err)
	}
	s.liquidValidatorAddr = common.BytesToAddress(valAddr.Bytes())

	lsKeeper.SetLiquidValidator(ctx, liquidstaketypes.LiquidValidator{
		OperatorAddress: validators[0].OperatorAddress,
	})

	params.WhitelistedValidators = append(params.WhitelistedValidators, liquidstaketypes.WhitelistedValidator{
		ValidatorAddress: validators[0].OperatorAddress,
		TargetWeight:     sdkmath.NewInt(10000),
	})

	params.WhitelistAdminAddress = validators[0].OperatorAddress

	// extract validator that wouldn't be liquidValidator
	s.Validator = validators[1]
	valAddr, err = sdk.ValAddressFromBech32(s.Validator.OperatorAddress)
	if err != nil {
		panic(err)
	}
	s.ValidatorAddr = common.BytesToAddress(valAddr.Bytes())

	s.admin = keyring.GetKey(9)
	params.WhitelistAdminAddress = s.admin.AccAddr.String()

	err = lsKeeper.SetParams(ctx, params)
	if err != nil {
		panic(err)
	}

	// Register liquidBondDenom as ERC-20 token pair (needed for Transfer event emission).
	// We do NOT enable it as a native precompile here because that would register a
	// codehash and add it to the precompile set, increasing gas overhead in unrelated
	// tests. The token pair lookup is sufficient for emitting Transfer events.
	liquidBondDenom := params.LiquidBondDenom
	liquidBondERC20Addr := denomToERC20Addr(liquidBondDenom)
	erc20Keeper := nw.App.GetErc20Keeper()
	lsmTokenPair := erc20types.NewTokenPair(liquidBondERC20Addr, liquidBondDenom, erc20types.OWNER_MODULE)
	if err := erc20Keeper.SetToken(ctx, lsmTokenPair); err != nil {
		panic(err)
	}
	s.liquidBondERC20Addr = liquidBondERC20Addr

	s.precompile = liquidstake.NewPrecompile(lsKeeper, nw.App.GetBankKeeper(), erc20Keeper)
}

// denomToERC20Addr replicates the keccak256-based address derivation used in tacchain upgrades.
func denomToERC20Addr(denom string) common.Address {
	hash := crypto.Keccak256([]byte(denom))
	return common.BytesToAddress(hash)
}
