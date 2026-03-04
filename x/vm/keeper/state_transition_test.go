package keeper_test

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"

	"github.com/cometbft/cometbft/crypto/tmhash"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmttypes "github.com/cometbft/cometbft/types"

	exampleapp "github.com/cosmos/evm/evmd"
	"github.com/cosmos/evm/testutil/integration/os/factory"
	"github.com/cosmos/evm/testutil/integration/os/grpc"
	testkeyring "github.com/cosmos/evm/testutil/integration/os/keyring"
	"github.com/cosmos/evm/testutil/integration/os/network"
	"github.com/cosmos/evm/testutil/integration/os/utils"
	utiltx "github.com/cosmos/evm/testutil/tx"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	"github.com/cosmos/evm/x/vm/keeper"
	"github.com/cosmos/evm/x/vm/overrides"
	"github.com/cosmos/evm/x/vm/types"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (suite *KeeperTestSuite) TestGetHashFn() {
	suite.SetupTest()
	header := suite.network.GetContext().BlockHeader()
	h, _ := cmttypes.HeaderFromProto(&header)
	hash := h.Hash()

	testCases := []struct {
		msg      string
		height   uint64
		malleate func() sdk.Context
		expHash  common.Hash
	}{
		{
			"case 1.1: context hash cached",
			uint64(suite.network.GetContext().BlockHeight()), //nolint:gosec // G115
			func() sdk.Context {
				return suite.network.GetContext().WithHeaderHash(
					tmhash.Sum([]byte("header")),
				)
			},
			common.BytesToHash(tmhash.Sum([]byte("header"))),
		},
		{
			"case 1.2: failed to cast Tendermint header",
			uint64(suite.network.GetContext().BlockHeight()), //nolint:gosec // G115
			func() sdk.Context {
				header := tmproto.Header{}
				header.Height = suite.network.GetContext().BlockHeight()
				return suite.network.GetContext().WithBlockHeader(header)
			},
			common.Hash{},
		},
		{
			"case 1.3: hash calculated from Tendermint header",
			uint64(suite.network.GetContext().BlockHeight()), //nolint:gosec // G115
			func() sdk.Context {
				return suite.network.GetContext().WithBlockHeader(header)
			},
			common.BytesToHash(hash),
		},
		{
			"case 2.1: height lower than current one, hist info not found",
			1,
			func() sdk.Context {
				return suite.network.GetContext().WithBlockHeight(10)
			},
			common.Hash{},
		},
		{
			"case 2.2: height lower than current one, invalid hist info header",
			1,
			func() sdk.Context {
				suite.Require().NoError(suite.network.App.StakingKeeper.SetHistoricalInfo(suite.network.GetContext(), 1, &stakingtypes.HistoricalInfo{}))
				return suite.network.GetContext().WithBlockHeight(10)
			},
			common.Hash{},
		},
		{
			"case 2.3: height lower than current one, calculated from hist info header",
			1,
			func() sdk.Context {
				histInfo := &stakingtypes.HistoricalInfo{
					Header: header,
				}
				suite.Require().NoError(suite.network.App.StakingKeeper.SetHistoricalInfo(suite.network.GetContext(), 1, histInfo))
				return suite.network.GetContext().WithBlockHeight(10)
			},
			common.BytesToHash(hash),
		},
		{
			"case 3: height greater than current one",
			200,
			func() sdk.Context { return suite.network.GetContext() },
			common.Hash{},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			ctx := tc.malleate()

			// Function being tested
			hash := suite.network.App.EVMKeeper.GetHashFn(ctx)(tc.height)
			suite.Require().Equal(tc.expHash, hash)

			err := suite.network.NextBlock()
			suite.Require().NoError(err)
		})
	}
}

func (suite *KeeperTestSuite) TestGetCoinbaseAddress() {
	suite.SetupTest()
	validators := suite.network.GetValidators()
	proposerAddressHex := utils.ValidatorConsAddressToHex(
		validators[0].OperatorAddress,
	)

	testCases := []struct {
		msg      string
		malleate func() sdk.Context
		expPass  bool
	}{
		{
			"validator not found",
			func() sdk.Context {
				header := suite.network.GetContext().BlockHeader()
				header.ProposerAddress = []byte{}
				return suite.network.GetContext().WithBlockHeader(header)
			},
			false,
		},
		{
			"success",
			func() sdk.Context {
				return suite.network.GetContext()
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			ctx := tc.malleate()
			proposerAddress := ctx.BlockHeader().ProposerAddress

			// Function being tested
			coinbase, err := suite.network.App.EVMKeeper.GetCoinbaseAddress(
				ctx,
				sdk.ConsAddress(proposerAddress),
			)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(proposerAddressHex, coinbase)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestGetEthIntrinsicGas() {
	suite.SetupTest()
	testCases := []struct {
		name               string
		data               []byte
		accessList         gethtypes.AccessList
		height             int64
		isContractCreation bool
		noError            bool
		expGas             uint64
	}{
		{
			"no data, no accesslist, not contract creation, not homestead, not istanbul, not shanghai",
			nil,
			nil,
			1,
			false,
			true,
			params.TxGas,
		},
		{
			"with one zero data, no accesslist, not contract creation, not homestead, not istanbul, not shanghai",
			[]byte{0},
			nil,
			1,
			false,
			true,
			params.TxGas + params.TxDataZeroGas*1,
		},
		{
			"with one non zero data, no accesslist, not contract creation, not homestead, not istanbul, not shanghai",
			[]byte{1},
			nil,
			1,
			true,
			true,
			params.TxGas + params.TxDataNonZeroGasFrontier*1,
		},
		{
			"no data, one accesslist, not contract creation, not homestead, not istanbul, not shanghai",
			nil,
			[]gethtypes.AccessTuple{
				{},
			},
			1,
			false,
			true,
			params.TxGas + params.TxAccessListAddressGas,
		},
		{
			"no data, one accesslist with one storageKey, not contract creation, not homestead, not istanbul, not shanghai",
			nil,
			[]gethtypes.AccessTuple{
				{StorageKeys: make([]common.Hash, 1)},
			},
			1,
			false,
			true,
			params.TxGas + params.TxAccessListAddressGas + params.TxAccessListStorageKeyGas*1,
		},
		{
			"no data, no accesslist, is contract creation, is homestead, not istanbul, not shanghai",
			nil,
			nil,
			2,
			true,
			true,
			params.TxGasContractCreation,
		},
		{
			"with one zero data, no accesslist, not contract creation, is homestead, is istanbul, not shanghai",
			[]byte{1},
			nil,
			3,
			false,
			true,
			params.TxGas + params.TxDataNonZeroGasEIP2028*1,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			ethCfg := types.GetEthChainConfig()
			ethCfg.HomesteadBlock = big.NewInt(2)
			ethCfg.IstanbulBlock = big.NewInt(3)
			signer := gethtypes.LatestSignerForChainID(ethCfg.ChainID)

			shanghaiTime := uint64(suite.network.GetContext().BlockTime().Unix()) + 10000 // in the future, fork not enabled
			ethCfg.ShanghaiTime = &shanghaiTime

			ctx := suite.network.GetContext().WithBlockHeight(tc.height)

			addr := suite.keyring.GetAddr(0)
			krSigner := utiltx.NewSigner(suite.keyring.GetPrivKey(0))
			nonce := suite.network.App.EVMKeeper.GetNonce(ctx, addr)
			m, err := newNativeMessage(
				nonce,
				ctx.BlockHeight(),
				addr,
				ethCfg,
				krSigner,
				signer,
				gethtypes.AccessListTxType,
				tc.data,
				tc.accessList,
			)
			suite.Require().NoError(err)

			// Function being tested
			gas, err := suite.network.App.EVMKeeper.GetEthIntrinsicGas(
				ctx,
				*m,
				ethCfg,
				tc.isContractCreation,
			)

			if tc.noError {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}

			suite.Require().Equal(tc.expGas, gas)
		})
	}
}

func (suite *KeeperTestSuite) TestGasToRefund() {
	suite.SetupTest()
	testCases := []struct {
		name           string
		gasconsumed    uint64
		refundQuotient uint64
		expGasRefund   uint64
		expPanic       bool
	}{
		{
			"gas refund 5",
			5,
			1,
			5,
			false,
		},
		{
			"gas refund 10",
			10,
			1,
			10,
			false,
		},
		{
			"gas refund availableRefund",
			11,
			1,
			10,
			false,
		},
		{
			"gas refund quotient 0",
			11,
			0,
			0,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			vmdb := suite.network.GetStateDB()
			vmdb.AddRefund(10)

			if tc.expPanic {
				panicF := func() {
					//nolint:staticcheck
					keeper.GasToRefund(vmdb.GetRefund(), tc.gasconsumed, tc.refundQuotient)
				}
				suite.Require().Panics(panicF)
			} else {
				gr := keeper.GasToRefund(vmdb.GetRefund(), tc.gasconsumed, tc.refundQuotient)
				suite.Require().Equal(tc.expGasRefund, gr)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestRefundGas() {
	// FeeCollector account is pre-funded with enough tokens
	// for refund to work
	// NOTE: everything should happen within the same block for
	// feecollector account to remain funded
	baseDenom := types.GetEVMCoinDenom()

	coins := sdk.NewCoins(sdk.NewCoin(
		baseDenom,
		sdkmath.NewInt(6e18),
	))
	balances := []banktypes.Balance{
		{
			Address: authtypes.NewModuleAddress(authtypes.FeeCollectorName).String(),
			Coins:   coins,
		},
	}
	bankGenesis := banktypes.DefaultGenesisState()
	bankGenesis.Balances = balances
	customGenesis := network.CustomGenesisState{}
	customGenesis[banktypes.ModuleName] = bankGenesis

	keyring := testkeyring.New(2)
	unitNetwork := network.NewUnitTestNetwork(
		network.WithPreFundedAccounts(keyring.GetAllAccAddrs()...),
		network.WithCustomGenesis(customGenesis),
	)
	grpcHandler := grpc.NewIntegrationHandler(unitNetwork)
	txFactory := factory.New(unitNetwork, grpcHandler)

	sender := keyring.GetKey(0)
	recipient := keyring.GetAddr(1)

	testCases := []struct {
		name           string
		leftoverGas    uint64
		refundQuotient uint64
		noError        bool
		expGasRefund   uint64
		gasPrice       *big.Int
	}{
		{
			name:           "leftoverGas more than tx gas limit",
			leftoverGas:    params.TxGas + 1,
			refundQuotient: params.RefundQuotient,
			noError:        false,
			expGasRefund:   params.TxGas + 1,
		},
		{
			name:           "leftoverGas equal to tx gas limit, insufficient fee collector account",
			leftoverGas:    params.TxGas,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   0,
		},
		{
			name:           "leftoverGas less than to tx gas limit",
			leftoverGas:    params.TxGas - 1,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   0,
		},
		{
			name:           "no leftoverGas, refund half used gas ",
			leftoverGas:    0,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   params.TxGas / params.RefundQuotient,
		},
		{
			name:           "invalid GasPrice in message",
			leftoverGas:    0,
			refundQuotient: params.RefundQuotient,
			noError:        false,
			expGasRefund:   params.TxGas / params.RefundQuotient,
			gasPrice:       big.NewInt(-100),
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			coreMsg, err := txFactory.GenerateGethCoreMsg(
				sender.Priv,
				types.EvmTxArgs{
					To:       &recipient,
					Amount:   big.NewInt(100),
					GasPrice: tc.gasPrice,
				},
			)
			suite.Require().NoError(err)
			transactionGas := coreMsg.GasLimit

			vmdb := unitNetwork.GetStateDB()
			vmdb.AddRefund(params.TxGas)

			if tc.leftoverGas > transactionGas {
				return
			}

			gasUsed := transactionGas - tc.leftoverGas
			refund := keeper.GasToRefund(vmdb.GetRefund(), gasUsed, tc.refundQuotient)
			suite.Require().Equal(tc.expGasRefund, refund)

			err = unitNetwork.App.EVMKeeper.RefundGas(
				unitNetwork.GetContext(),
				*coreMsg,
				refund,
				unitNetwork.GetBaseDenom(),
			)

			if tc.noError {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestResetGasMeterAndConsumeGas() {
	suite.SetupTest()
	testCases := []struct {
		name        string
		gasConsumed uint64
		gasUsed     uint64
		expPanic    bool
	}{
		{
			"gas consumed 5, used 5",
			5,
			5,
			false,
		},
		{
			"gas consumed 5, used 10",
			5,
			10,
			false,
		},
		{
			"gas consumed 10, used 10",
			10,
			10,
			false,
		},
		{
			"gas consumed 11, used 10, NegativeGasConsumed panic",
			11,
			10,
			true,
		},
		{
			"gas consumed 1, used 10, overflow panic",
			1,
			math.MaxUint64,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			panicF := func() {
				gm := storetypes.NewGasMeter(10)
				gm.ConsumeGas(tc.gasConsumed, "")
				ctx := suite.network.GetContext().WithGasMeter(gm)
				suite.network.App.EVMKeeper.ResetGasMeterAndConsumeGas(ctx, tc.gasUsed)
			}

			if tc.expPanic {
				suite.Require().Panics(panicF)
			} else {
				suite.Require().NotPanics(panicF)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestEVMConfig() {
	suite.SetupTest()

	defaultChainEVMParams := exampleapp.NewEVMGenesisState().Params

	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	cfg, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)
	suite.Require().Equal(defaultChainEVMParams, cfg.Params)
	// london hardfork is enabled by default
	suite.Require().Equal(big.NewInt(0), cfg.BaseFee)
	suite.Require().Equal(types.GetEthChainConfig(), cfg.ChainConfig)

	validators := suite.network.GetValidators()
	proposerHextAddress := utils.ValidatorConsAddressToHex(validators[0].OperatorAddress)
	suite.Require().Equal(proposerHextAddress, cfg.CoinBase)
}

func (suite *KeeperTestSuite) TestApplyMessage() {
	suite.enableFeemarket = true
	defer func() { suite.enableFeemarket = false }()
	suite.SetupTest()

	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	// Generate a transfer tx message
	sender := suite.keyring.GetKey(0)
	recipient := suite.keyring.GetAddr(1)
	transferArgs := types.EvmTxArgs{
		To:     &recipient,
		Amount: big.NewInt(100),
	}
	coreMsg, err := suite.factory.GenerateGethCoreMsg(
		sender.Priv,
		transferArgs,
	)
	suite.Require().NoError(err)

	tracer := suite.network.App.EVMKeeper.Tracer(
		suite.network.GetContext(),
		*coreMsg,
		config.ChainConfig,
	)
	res, err := suite.network.App.EVMKeeper.ApplyMessage(
		suite.network.GetContext(),
		*coreMsg,
		tracer,
		true,
	)
	suite.Require().NoError(err)
	suite.Require().False(res.Failed())

	// Compare gas to a transfer tx gas
	expectedGasUsed := params.TxGas
	suite.Require().Equal(expectedGasUsed, res.GasUsed)
}

func (suite *KeeperTestSuite) TestApplyMessageWithConfig() {
	suite.enableFeemarket = true
	defer func() { suite.enableFeemarket = false }()
	suite.SetupTest()
	testCases := []struct {
		name               string
		getMessage         func() core.Message
		getEVMParams       func() types.Params
		getFeeMarketParams func() feemarkettypes.Params
		expErr             bool
		expVMErr           bool
		expectedGasUsed    uint64
	}{
		{
			"success - messsage applied ok with default params",
			func() core.Message {
				sender := suite.keyring.GetKey(0)
				recipient := suite.keyring.GetAddr(1)
				msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
					To:     &recipient,
					Amount: big.NewInt(100),
				})
				suite.Require().NoError(err)
				return *msg
			},
			types.DefaultParams,
			feemarkettypes.DefaultParams,
			false,
			false,
			params.TxGas,
		},
		{
			"call contract tx with config param EnableCall = false",
			func() core.Message {
				sender := suite.keyring.GetKey(0)
				recipient := suite.keyring.GetAddr(1)
				msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
					To:     &recipient,
					Amount: big.NewInt(100),
					Input:  []byte("contract_data"),
				})
				suite.Require().NoError(err)
				return *msg
			},
			func() types.Params {
				defaultParams := types.DefaultParams()
				defaultParams.AccessControl = types.AccessControl{
					Call: types.AccessControlType{
						AccessType: types.AccessTypeRestricted,
					},
				}
				return defaultParams
			},
			feemarkettypes.DefaultParams,
			false,
			true,
			0,
		},
		{
			"create contract tx with config param EnableCreate = false",
			func() core.Message {
				sender := suite.keyring.GetKey(0)
				msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
					Amount: big.NewInt(100),
					Input:  []byte("contract_data"),
				})
				suite.Require().NoError(err)
				return *msg
			},
			func() types.Params {
				defaultParams := types.DefaultParams()
				defaultParams.AccessControl = types.AccessControl{
					Create: types.AccessControlType{
						AccessType: types.AccessTypeRestricted,
					},
				}
				return defaultParams
			},
			feemarkettypes.DefaultParams,
			false,
			true,
			0,
		},
		{
			"fail - fix panic when minimumGasUsed is not uint64",
			func() core.Message {
				sender := suite.keyring.GetKey(0)
				recipient := suite.keyring.GetAddr(1)
				msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
					To:     &recipient,
					Amount: big.NewInt(100),
				})
				suite.Require().NoError(err)
				return *msg
			},
			types.DefaultParams,
			func() feemarkettypes.Params {
				paramsRes, err := suite.handler.GetFeeMarketParams()
				suite.Require().NoError(err)
				params := paramsRes.GetParams()
				params.MinGasMultiplier = sdkmath.LegacyNewDec(math.MaxInt64).MulInt64(100)
				return params
			},
			true,
			false,
			0,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			msg := tc.getMessage()
			evmParams := tc.getEVMParams()
			err := suite.network.App.EVMKeeper.SetParams(
				suite.network.GetContext(),
				evmParams,
			)
			suite.Require().NoError(err)
			feeMarketparams := tc.getFeeMarketParams()
			err = suite.network.App.FeeMarketKeeper.SetParams(
				suite.network.GetContext(),
				feeMarketparams,
			)
			suite.Require().NoError(err)

			txConfig := suite.network.App.EVMKeeper.TxConfig(
				suite.network.GetContext(),
				common.Hash{},
			)
			proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
			config, err := suite.network.App.EVMKeeper.EVMConfig(
				suite.network.GetContext(),
				proposerAddress,
			)
			suite.Require().NoError(err)

			// Function being tested
			res, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
				suite.network.GetContext(),
				msg,
				nil,
				true,
				config,
				txConfig,
				nil,
				nil,
			)

			if tc.expErr {
				suite.Require().Error(err)
			} else if !tc.expVMErr {
				suite.Require().NoError(err)
				suite.Require().False(res.Failed())
				suite.Require().Equal(tc.expectedGasUsed, res.GasUsed)
			}

			err = suite.network.NextBlock()
			if tc.expVMErr {
				suite.Require().NotEmpty(res.VmError)
				return
			}

			suite.Require().NoError(err)
		})
	}
}

func (suite *KeeperTestSuite) TestApplyMessageWithStateOverride() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)
	recipient := suite.keyring.GetAddr(1)

	// Get proposer address and EVM config
	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	testCases := []struct {
		name          string
		stateOverride overrides.StateOverride
		commit        bool
		expErr        bool
		expErrMsg     string
	}{
		{
			name:          "success - nil state override",
			stateOverride: nil,
			commit:        false,
			expErr:        false,
		},
		{
			name: "success - override balance",
			stateOverride: overrides.StateOverride{
				recipient: overrides.OverrideAccount{
					Balance: (*hexutil.Big)(big.NewInt(1e18)),
				},
			},
			commit: false,
			expErr: false,
		},
		{
			name: "success - override nonce",
			stateOverride: overrides.StateOverride{
				recipient: overrides.OverrideAccount{
					Nonce: func() *hexutil.Uint64 { n := hexutil.Uint64(100); return &n }(),
				},
			},
			commit: false,
			expErr: false,
		},
		{
			name: "success - override code (empty code does not affect simple transfer)",
			stateOverride: overrides.StateOverride{
				recipient: overrides.OverrideAccount{
					// Setting empty code on recipient - simple transfer should still work
					Code: func() *hexutil.Bytes { b := hexutil.Bytes([]byte{}); return &b }(),
				},
			},
			commit: false,
			expErr: false,
		},
		{
			name: "success - override state diff",
			stateOverride: overrides.StateOverride{
				recipient: overrides.OverrideAccount{
					StateDiff: map[common.Hash]common.Hash{
						common.HexToHash("0x01"): common.HexToHash("0x02"),
					},
				},
			},
			commit: false,
			expErr: false,
		},
		{
			name: "fail - state override with commit=true",
			stateOverride: overrides.StateOverride{
				recipient: overrides.OverrideAccount{
					Balance: (*hexutil.Big)(big.NewInt(1e18)),
				},
			},
			commit:    true,
			expErr:    true,
			expErrMsg: "state override is not nil",
		},
		{
			name: "fail - both state and stateDiff provided",
			stateOverride: overrides.StateOverride{
				recipient: overrides.OverrideAccount{
					State: map[common.Hash]common.Hash{
						common.HexToHash("0x01"): common.HexToHash("0x02"),
					},
					StateDiff: map[common.Hash]common.Hash{
						common.HexToHash("0x03"): common.HexToHash("0x04"),
					},
				},
			},
			commit:    false,
			expErr:    true,
			expErrMsg: "has both 'state' and 'stateDiff'",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			// Generate a transfer message
			msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
				To:     &recipient,
				Amount: big.NewInt(100),
			})
			suite.Require().NoError(err)

			txConfig := suite.network.App.EVMKeeper.TxConfig(
				suite.network.GetContext(),
				common.Hash{},
			)

			// Function being tested
			res, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
				suite.network.GetContext(),
				*msg,
				nil,
				tc.commit,
				config,
				txConfig,
				tc.stateOverride,
				nil,
			)

			if tc.expErr {
				suite.Require().Error(err)
				suite.Require().Contains(err.Error(), tc.expErrMsg)
			} else {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)
				suite.Require().False(res.Failed())
			}
		})
	}
}

// TestStateOverrideBalanceCheck tests that StateOverride actually changes the balance
// by calling a contract that checks the balance of an address
func (suite *KeeperTestSuite) TestStateOverrideBalanceCheck() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)
	senderAddr := suite.keyring.GetAddr(0)

	// Address to check balance
	targetAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// Contract bytecode that returns balance of msg.sender:
	// SELFBALANCE opcode returns the balance of the current contract
	// We'll use BALANCE opcode to get balance of a specific address
	//
	// Bytecode explanation:
	// PUSH20 <address> - push target address
	// BALANCE - get balance of that address
	// PUSH1 0x00 - push memory offset
	// MSTORE - store balance in memory
	// PUSH1 0x20 - push return size (32 bytes)
	// PUSH1 0x00 - push return offset
	// RETURN - return the balance
	//
	// 73 <20 bytes address> 31 60 00 52 60 20 60 00 f3
	balanceCheckerCode := append(
		[]byte{0x73}, // PUSH20
		append(
			targetAddr.Bytes(),
			[]byte{
				0x31,       // BALANCE
				0x60, 0x00, // PUSH1 0x00
				0x52,       // MSTORE
				0x60, 0x20, // PUSH1 0x20
				0x60, 0x00, // PUSH1 0x00
				0xf3, // RETURN
			}...,
		)...,
	)

	// Deploy a simple contract that will check balance
	contractAddr := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	// Get proposer address and EVM config
	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	// Test 1: Without state override - balance should be 0
	txConfig := suite.network.App.EVMKeeper.TxConfig(
		suite.network.GetContext(),
		common.Hash{},
	)

	msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
		To:       &contractAddr,
		Input:    []byte{}, // just call the contract
		GasLimit: 100000,   // enough gas for the call
	})
	suite.Require().NoError(err)

	// State override: set code for contractAddr and balance for targetAddr
	overriddenBalance := big.NewInt(123456789)
	stateOverride := overrides.StateOverride{
		contractAddr: overrides.OverrideAccount{
			Code: func() *hexutil.Bytes { b := hexutil.Bytes(balanceCheckerCode); return &b }(),
		},
		targetAddr: overrides.OverrideAccount{
			Balance: (*hexutil.Big)(overriddenBalance),
		},
	}

	res, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
		suite.network.GetContext(),
		*msg,
		nil,
		false, // don't commit
		config,
		txConfig,
		stateOverride,
		nil,
	)
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
	suite.Require().False(res.Failed(), "VM error: %s", res.VmError)

	// The result should contain the overridden balance
	returnedBalance := new(big.Int).SetBytes(res.Ret)
	suite.Require().Equal(overriddenBalance.String(), returnedBalance.String(),
		"StateOverride should have changed the balance visible to the contract")

	// Test 2: Verify original state is unchanged (targetAddr should have 0 balance)
	originalBalance := suite.network.App.EVMKeeper.GetBalance(suite.network.GetContext(), targetAddr)
	suite.Require().Equal(big.NewInt(0).String(), originalBalance.String(),
		"Original state should not be modified")

	_ = senderAddr // suppress unused variable warning
}

// TestStateOverrideStateMapResetsNonOverriddenKeys verifies that when State map (full state replacement)
// is used in state override, storage keys NOT present in the override return zero values,
// even if they exist in the actual on-chain storage. This mimics geth behavior.
// In contrast, StateDiff (partial override) should preserve non-overridden keys.
func (suite *KeeperTestSuite) TestStateOverrideStateMapResetsNonOverriddenKeys() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)

	// Contract address where we'll set real storage and override it
	contractAddr := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	// Storage slots and values
	slot0 := common.HexToHash("0x00")
	slot1 := common.HexToHash("0x01")
	realValue0 := common.HexToHash("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	realValue1 := common.HexToHash("0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	overrideValue0 := common.HexToHash("0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC")

	// Write real storage values into the keeper (on-chain state)
	suite.network.App.EVMKeeper.SetState(suite.network.GetContext(), contractAddr, slot0, realValue0.Bytes())
	suite.network.App.EVMKeeper.SetState(suite.network.GetContext(), contractAddr, slot1, realValue1.Bytes())

	// Verify real storage was written correctly
	readValue0 := suite.network.App.EVMKeeper.GetState(suite.network.GetContext(), contractAddr, slot0)
	readValue1 := suite.network.App.EVMKeeper.GetState(suite.network.GetContext(), contractAddr, slot1)
	suite.Require().Equal(realValue0, readValue0, "slot0 should be written to keeper")
	suite.Require().Equal(realValue1, readValue1, "slot1 should be written to keeper")

	// Bytecode: reads slot 0 and slot 1, returns both as 64 bytes
	//
	// PUSH1 0x00  SLOAD  PUSH1 0x00  MSTORE   -> memory[0..31] = storage[0]
	// PUSH1 0x01  SLOAD  PUSH1 0x20  MSTORE   -> memory[32..63] = storage[1]
	// PUSH1 0x40  PUSH1 0x00  RETURN           -> return 64 bytes from memory
	storageReaderCode := []byte{
		0x60, 0x00, // PUSH1 0x00
		0x54,       // SLOAD
		0x60, 0x00, // PUSH1 0x00
		0x52,       // MSTORE
		0x60, 0x01, // PUSH1 0x01
		0x54,       // SLOAD
		0x60, 0x20, // PUSH1 0x20
		0x52,       // MSTORE
		0x60, 0x40, // PUSH1 0x40
		0x60, 0x00, // PUSH1 0x00
		0xf3, // RETURN
	}

	// Get proposer address and EVM config
	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	txConfig := suite.network.App.EVMKeeper.TxConfig(
		suite.network.GetContext(),
		common.Hash{},
	)

	msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
		To:       &contractAddr,
		Input:    []byte{},
		GasLimit: 100000,
	})
	suite.Require().NoError(err)

	// ---------------------------------------------------------------
	// Test 1: State override with State map (full replacement)
	// Override only slot0 -> non-overridden slot1 MUST return zero
	// ---------------------------------------------------------------
	stateOverrideFullReplace := overrides.StateOverride{
		contractAddr: overrides.OverrideAccount{
			Code: func() *hexutil.Bytes { b := hexutil.Bytes(storageReaderCode); return &b }(),
			State: map[common.Hash]common.Hash{
				slot0: overrideValue0,
				// slot1 is intentionally NOT in the State map
			},
		},
	}

	res, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
		suite.network.GetContext(),
		*msg,
		nil,
		false,
		config,
		txConfig,
		stateOverrideFullReplace,
		nil,
	)
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
	suite.Require().False(res.Failed(), "VM error: %s", res.VmError)
	suite.Require().Len(res.Ret, 64, "expected 64 bytes return (two 32-byte slots)")

	returnedSlot0 := common.BytesToHash(res.Ret[:32])
	returnedSlot1 := common.BytesToHash(res.Ret[32:64])

	suite.Require().Equal(overrideValue0, returnedSlot0,
		"State override: slot0 should return the overridden value")
	suite.Require().Equal(common.Hash{}, returnedSlot1,
		"State override: slot1 (not in override State map) should return zero, not the real on-chain value")

	// ---------------------------------------------------------------
	// Test 2: State override with StateDiff (partial override)
	// Override only slot0 -> non-overridden slot1 MUST return real value
	// ---------------------------------------------------------------
	msg2, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
		To:       &contractAddr,
		Input:    []byte{},
		GasLimit: 100000,
	})
	suite.Require().NoError(err)

	stateOverridePartial := overrides.StateOverride{
		contractAddr: overrides.OverrideAccount{
			Code: func() *hexutil.Bytes { b := hexutil.Bytes(storageReaderCode); return &b }(),
			StateDiff: map[common.Hash]common.Hash{
				slot0: overrideValue0,
				// slot1 is intentionally NOT in the StateDiff map
			},
		},
	}

	txConfig2 := suite.network.App.EVMKeeper.TxConfig(
		suite.network.GetContext(),
		common.Hash{},
	)

	res2, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
		suite.network.GetContext(),
		*msg2,
		nil,
		false,
		config,
		txConfig2,
		stateOverridePartial,
		nil,
	)
	suite.Require().NoError(err)
	suite.Require().NotNil(res2)
	suite.Require().False(res2.Failed(), "VM error: %s", res2.VmError)
	suite.Require().Len(res2.Ret, 64, "expected 64 bytes return (two 32-byte slots)")

	returnedSlot0Diff := common.BytesToHash(res2.Ret[:32])
	returnedSlot1Diff := common.BytesToHash(res2.Ret[32:64])

	suite.Require().Equal(overrideValue0, returnedSlot0Diff,
		"StateDiff override: slot0 should return the overridden value")
	suite.Require().Equal(realValue1, returnedSlot1Diff,
		"StateDiff override: slot1 (not in override) should return the real on-chain value")

	// ---------------------------------------------------------------
	// Test 3: Verify original on-chain state is unchanged after overrides
	// ---------------------------------------------------------------
	finalValue0 := suite.network.App.EVMKeeper.GetState(suite.network.GetContext(), contractAddr, slot0)
	finalValue1 := suite.network.App.EVMKeeper.GetState(suite.network.GetContext(), contractAddr, slot1)
	suite.Require().Equal(realValue0, finalValue0, "Original slot0 should not be modified")
	suite.Require().Equal(realValue1, finalValue1, "Original slot1 should not be modified")
}

// TestStateOverrideCannotOverridePrecompile verifies that state override
// returns an error when trying to override a precompile contract address.
func (suite *KeeperTestSuite) TestStateOverrideCannotOverridePrecompile() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)

	// Use standard EVM precompile addresses (ecrecover=0x01, sha256=0x02, etc.)
	// These are always present in ActivePrecompiles regardless of the chain configuration.
	ecrecoverAddr := common.HexToAddress("0x0000000000000000000000000000000000000001")

	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	txConfig := suite.network.App.EVMKeeper.TxConfig(
		suite.network.GetContext(),
		common.Hash{},
	)

	recipient := suite.keyring.GetAddr(1)
	msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
		To:     &recipient,
		Amount: big.NewInt(100),
	})
	suite.Require().NoError(err)

	testCases := []struct {
		name          string
		stateOverride overrides.StateOverride
	}{
		{
			name: "fail - override precompile balance",
			stateOverride: overrides.StateOverride{
				ecrecoverAddr: overrides.OverrideAccount{
					Balance: (*hexutil.Big)(big.NewInt(1e18)),
				},
			},
		},
		{
			name: "fail - override precompile code",
			stateOverride: overrides.StateOverride{
				ecrecoverAddr: overrides.OverrideAccount{
					Code: func() *hexutil.Bytes { b := hexutil.Bytes([]byte{0x60, 0x00}); return &b }(),
				},
			},
		},
		{
			name: "fail - override precompile nonce",
			stateOverride: overrides.StateOverride{
				ecrecoverAddr: overrides.OverrideAccount{
					Nonce: func() *hexutil.Uint64 { n := hexutil.Uint64(1); return &n }(),
				},
			},
		},
		{
			name: "fail - override precompile state",
			stateOverride: overrides.StateOverride{
				ecrecoverAddr: overrides.OverrideAccount{
					State: map[common.Hash]common.Hash{
						common.HexToHash("0x01"): common.HexToHash("0x02"),
					},
				},
			},
		},
		{
			name: "fail - override precompile stateDiff",
			stateOverride: overrides.StateOverride{
				ecrecoverAddr: overrides.OverrideAccount{
					StateDiff: map[common.Hash]common.Hash{
						common.HexToHash("0x01"): common.HexToHash("0x02"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			_, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
				suite.network.GetContext(),
				*msg,
				nil,
				false,
				config,
				txConfig,
				tc.stateOverride,
				nil,
			)
			suite.Require().Error(err)
			suite.Require().Contains(err.Error(), "is a precompile, state override is not allowed")
		})
	}
}

// TestApplyMessageWithBlockOverrides verifies that BlockOverrides correctly override
// the EVM block context fields (NUMBER, TIMESTAMP, COINBASE, DIFFICULTY, GASLIMIT, BASEFEE)
// when calling ApplyMessageWithConfig.
func (suite *KeeperTestSuite) TestApplyMessageWithBlockOverrides() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)

	// Contract address where we deploy code via state override
	contractAddr := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	// Get proposer address and EVM config
	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	// -------------------------------------------------------------------
	// Bytecode that returns NUMBER (block number):
	// NUMBER PUSH1 0x00 MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	// 43 60 00 52 60 20 60 00 f3
	numberCode := []byte{0x43, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3}

	// Bytecode that returns TIMESTAMP:
	// TIMESTAMP PUSH1 0x00 MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	// 42 60 00 52 60 20 60 00 f3
	timestampCode := []byte{0x42, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3}

	// Bytecode that returns COINBASE:
	// COINBASE PUSH1 0x00 MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	// 41 60 00 52 60 20 60 00 f3
	coinbaseCode := []byte{0x41, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3}

	// Bytecode that returns DIFFICULTY (prevrandao after merge):
	// DIFFICULTY PUSH1 0x00 MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	// 44 60 00 52 60 20 60 00 f3
	difficultyCode := []byte{0x44, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3}

	// Bytecode that returns GASLIMIT:
	// GASLIMIT PUSH1 0x00 MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	// 45 60 00 52 60 20 60 00 f3
	gasLimitCode := []byte{0x45, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3}

	// Bytecode that returns BASEFEE:
	// BASEFEE PUSH1 0x00 MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	// 48 60 00 52 60 20 60 00 f3
	baseFeeCode := []byte{0x48, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3}

	overriddenNumber := big.NewInt(999999)
	overriddenTime := hexutil.Uint64(1234567890)
	overriddenCoinbase := common.HexToAddress("0xCAFECAFECAFECAFECAFECAFECAFECAFECAFECAFE")
	overriddenDifficulty := big.NewInt(42)
	overriddenGasLimit := hexutil.Uint64(30000000)
	overriddenBaseFee := big.NewInt(5000000000)

	testCases := []struct {
		name           string
		code           []byte
		blockOverrides *overrides.BlockOverrides
		expValue       *big.Int
		expErr         bool
	}{
		{
			name: "success - override block number",
			code: numberCode,
			blockOverrides: &overrides.BlockOverrides{
				Number: (*hexutil.Big)(overriddenNumber),
			},
			expValue: overriddenNumber,
		},
		{
			name: "success - override timestamp",
			code: timestampCode,
			blockOverrides: &overrides.BlockOverrides{
				Time: &overriddenTime,
			},
			expValue: big.NewInt(int64(overriddenTime)),
		},
		{
			name: "success - override coinbase (feeRecipient)",
			code: coinbaseCode,
			blockOverrides: &overrides.BlockOverrides{
				FeeRecipient: &overriddenCoinbase,
			},
			expValue: overriddenCoinbase.Big(),
		},
		{
			name: "success - override difficulty via prevRandao (post-merge DIFFICULTY returns Random)",
			code: difficultyCode,
			blockOverrides: &overrides.BlockOverrides{
				PrevRandao: func() *common.Hash {
					h := common.BigToHash(overriddenDifficulty)
					return &h
				}(),
			},
			expValue: overriddenDifficulty,
		},
		{
			name: "success - override gas limit",
			code: gasLimitCode,
			blockOverrides: &overrides.BlockOverrides{
				GasLimit: &overriddenGasLimit,
			},
			expValue: big.NewInt(int64(overriddenGasLimit)),
		},
		{
			name: "success - override baseFeePerGas",
			code: baseFeeCode,
			blockOverrides: &overrides.BlockOverrides{
				BaseFeePerGas: (*hexutil.Big)(overriddenBaseFee),
			},
			expValue: overriddenBaseFee,
		},
		{
			name:           "success - nil block overrides (use defaults)",
			code:           numberCode,
			blockOverrides: nil,
			expValue:       big.NewInt(suite.network.GetContext().BlockHeight()),
		},
		{
			name: "success - override multiple fields at once",
			code: numberCode,
			blockOverrides: &overrides.BlockOverrides{
				Number:        (*hexutil.Big)(overriddenNumber),
				Time:          &overriddenTime,
				FeeRecipient:  &overriddenCoinbase,
				BaseFeePerGas: (*hexutil.Big)(overriddenBaseFee),
			},
			expValue: overriddenNumber, // we check block number from the contract
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
				To:       &contractAddr,
				Input:    []byte{},
				GasLimit: 100000,
			})
			suite.Require().NoError(err)

			txConfig := suite.network.App.EVMKeeper.TxConfig(
				suite.network.GetContext(),
				common.Hash{},
			)

			// Use state override to deploy contract code at contractAddr
			stateOverride := overrides.StateOverride{
				contractAddr: overrides.OverrideAccount{
					Code: func() *hexutil.Bytes { b := hexutil.Bytes(tc.code); return &b }(),
				},
			}

			res, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
				suite.network.GetContext(),
				*msg,
				nil,
				false, // don't commit (required for state override)
				config,
				txConfig,
				stateOverride,
				tc.blockOverrides,
			)

			if tc.expErr {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)
				suite.Require().False(res.Failed(), "VM error: %s", res.VmError)

				returnedValue := new(big.Int).SetBytes(res.Ret)
				suite.Require().Equal(tc.expValue.String(), returnedValue.String(),
					"block override value mismatch")
			}
		})
	}
}

// TestBlockOverridesWithCommitTrue verifies that block overrides are rejected
// when commit=true, similar to state overrides.
func (suite *KeeperTestSuite) TestBlockOverridesWithCommitTrue() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)
	recipient := suite.keyring.GetAddr(1)

	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	overriddenNumber := big.NewInt(777777)

	msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
		To:     &recipient,
		Amount: big.NewInt(100),
	})
	suite.Require().NoError(err)

	txConfig := suite.network.App.EVMKeeper.TxConfig(
		suite.network.GetContext(),
		common.Hash{},
	)

	// Block overrides should NOT be allowed with commit=true
	_, err = suite.network.App.EVMKeeper.ApplyMessageWithConfig(
		suite.network.GetContext(),
		*msg,
		nil,
		true,
		config,
		txConfig,
		nil,
		&overrides.BlockOverrides{
			Number: (*hexutil.Big)(overriddenNumber),
		},
	)
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "block overrides are not nil")
}

// TestBlockOverridesPrevRandao verifies that the PREVRANDAO (DIFFICULTY post-merge)
// opcode returns the overridden value from BlockOverrides.
func (suite *KeeperTestSuite) TestBlockOverridesPrevRandao() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)
	contractAddr := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	// DIFFICULTY opcode (0x44) returns prevRandao after the merge
	// DIFFICULTY PUSH1 0x00 MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	prevRandaoCode := []byte{0x44, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3}

	overriddenRandao := common.HexToHash("0xDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF")

	msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
		To:       &contractAddr,
		Input:    []byte{},
		GasLimit: 100000,
	})
	suite.Require().NoError(err)

	txConfig := suite.network.App.EVMKeeper.TxConfig(
		suite.network.GetContext(),
		common.Hash{},
	)

	stateOverride := overrides.StateOverride{
		contractAddr: overrides.OverrideAccount{
			Code: func() *hexutil.Bytes { b := hexutil.Bytes(prevRandaoCode); return &b }(),
		},
	}

	res, err := suite.network.App.EVMKeeper.ApplyMessageWithConfig(
		suite.network.GetContext(),
		*msg,
		nil,
		false,
		config,
		txConfig,
		stateOverride,
		&overrides.BlockOverrides{
			PrevRandao: &overriddenRandao,
		},
	)
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
	suite.Require().False(res.Failed(), "VM error: %s", res.VmError)

	// The DIFFICULTY opcode post-merge returns the random value which is
	// the prevRandao field. It's returned as a uint256.
	returnedHash := common.BytesToHash(res.Ret)
	suite.Require().Equal(overriddenRandao, returnedHash,
		"PrevRandao override should be reflected via DIFFICULTY opcode")
}

// TestBlockOverridesDoNotAffectContext verifies that block overrides
// do not persist into the SDK context after execution.
func (suite *KeeperTestSuite) TestBlockOverridesDoNotAffectContext() {
	suite.SetupTest()

	sender := suite.keyring.GetKey(0)
	recipient := suite.keyring.GetAddr(1)

	proposerAddress := suite.network.GetContext().BlockHeader().ProposerAddress
	config, err := suite.network.App.EVMKeeper.EVMConfig(
		suite.network.GetContext(),
		proposerAddress,
	)
	suite.Require().NoError(err)

	originalHeight := suite.network.GetContext().BlockHeight()
	overriddenNumber := big.NewInt(999999)

	msg, err := suite.factory.GenerateGethCoreMsg(sender.Priv, types.EvmTxArgs{
		To:     &recipient,
		Amount: big.NewInt(100),
	})
	suite.Require().NoError(err)

	txConfig := suite.network.App.EVMKeeper.TxConfig(
		suite.network.GetContext(),
		common.Hash{},
	)

	_, err = suite.network.App.EVMKeeper.ApplyMessageWithConfig(
		suite.network.GetContext(),
		*msg,
		nil,
		false,
		config,
		txConfig,
		nil,
		&overrides.BlockOverrides{
			Number: (*hexutil.Big)(overriddenNumber),
		},
	)
	suite.Require().NoError(err)

	// Verify that the SDK context block height is unchanged
	suite.Require().Equal(originalHeight, suite.network.GetContext().BlockHeight(),
		"Block overrides should not modify the SDK context")
}

func (suite *KeeperTestSuite) TestGetProposerAddress() {
	suite.SetupTest()
	address := sdk.ConsAddress(suite.keyring.GetAddr(0).Bytes())
	proposerAddress := sdk.ConsAddress(suite.network.GetContext().BlockHeader().ProposerAddress)
	testCases := []struct {
		msg    string
		addr   sdk.ConsAddress
		expAdr sdk.ConsAddress
	}{
		{
			"proposer address provided",
			address,
			address,
		},
		{
			"nil proposer address provided",
			nil,
			proposerAddress,
		},
		{
			"typed nil proposer address provided",
			sdk.ConsAddress{},
			proposerAddress,
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.Require().Equal(
				tc.expAdr,
				keeper.GetProposerAddress(suite.network.GetContext(), tc.addr),
			)
		})
	}
}
