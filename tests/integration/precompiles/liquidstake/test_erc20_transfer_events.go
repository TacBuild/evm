package liquidstake

import (
	"math/big"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"

	liquidstake "github.com/cosmos/evm/precompiles/liquidstake"
	chainutil "github.com/cosmos/evm/testutil"
	testkeyring "github.com/cosmos/evm/testutil/keyring"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
	"github.com/cosmos/evm/x/vm/statedb"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// erc20TransferEventID is the keccak256 of "Transfer(address,address,uint256)".
var erc20TransferEventID = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

// findERC20TransferLog returns the first log emitted by addr matching Transfer event.
func (s *PrecompileTestSuite) findERC20TransferLog(stDB *statedb.StateDB, tokenAddr common.Address) *ethtypes.Log {
	for _, l := range stDB.Logs() {
		if l.Address == tokenAddr && len(l.Topics) == 3 && l.Topics[0] == erc20TransferEventID {
			return l
		}
	}
	return nil
}

// runPrecompileTx runs a precompile transaction and returns the resulting statedb.
func (s *PrecompileTestSuite) runPrecompileTx(ctx sdk.Context, delegator testkeyring.Key, input []byte, gas uint64) (*statedb.StateDB, error) {
	baseFee := s.nw.App.GetEVMKeeper().GetBaseFee(ctx)
	contractAddr := s.precompile.Address()

	contract := vm.NewPrecompile(delegator.Addr, contractAddr, uint256.NewInt(0), gas)
	contract.Input = input

	txArgs := evmtypes.EvmTxArgs{
		ChainID:   evmtypes.GetEthChainConfig().ChainID,
		Nonce:     0,
		To:        &contractAddr,
		Amount:    nil,
		GasLimit:  gas,
		GasPrice:  chainutil.ExampleMinGasPrices,
		GasFeeCap: baseFee,
		GasTipCap: big.NewInt(1),
		Accesses:  &ethtypes.AccessList{},
	}

	msg, err := s.factory.GenerateGethCoreMsg(delegator.Priv, txArgs)
	s.Require().NoError(err)

	proposerAddress := ctx.BlockHeader().ProposerAddress
	cfg, err := s.nw.App.GetEVMKeeper().EVMConfig(ctx, proposerAddress)
	s.Require().NoError(err)

	stDB := statedb.New(ctx, s.nw.App.GetEVMKeeper(), statedb.NewEmptyTxConfig())
	evm := s.nw.App.GetEVMKeeper().NewEVM(ctx, *msg, cfg, nil, stDB)

	precompiles, found, err := s.nw.App.GetEVMKeeper().GetPrecompileInstance(ctx, contractAddr)
	s.Require().NoError(err)
	s.Require().True(found)
	evm.WithPrecompiles(precompiles.Map)

	_, err = s.precompile.Run(evm, contract, false)
	return stDB, err
}

// TestLiquidStakeEmitsERC20TransferEvent verifies that LiquidStake emits
// a Transfer(address(0) -> delegator, mintedAmount) event on the liquidBondDenom ERC-20.
func (s *PrecompileTestSuite) TestLiquidStakeEmitsERC20TransferEvent() {
	s.SetupTest()
	ctx := s.nw.GetContext().WithBlockTime(time.Now())
	delegator := s.keyring.GetKey(0)

	stakeAmount := big.NewInt(1_000_000)
	input, err := s.precompile.Pack(liquidstake.LiquidStakeMethod, delegator.Addr, stakeAmount)
	s.Require().NoError(err)

	stDB, err := s.runPrecompileTx(ctx, delegator, input, 1_000_000)
	s.Require().NoError(err)

	log := s.findERC20TransferLog(stDB, s.liquidBondERC20Addr)
	s.Require().NotNil(log, "expected ERC-20 Transfer log for liquidBondDenom")

	// topics[1] = from (address(0) for mint), topics[2] = to (delegator)
	s.Require().Equal(common.Hash{}, log.Topics[1], "expected Transfer from zero address (mint)")
	s.Require().Equal(delegator.Addr, common.BytesToAddress(log.Topics[2].Bytes()), "expected Transfer to delegator")

	// value must be positive
	value := new(big.Int).SetBytes(log.Data)
	s.Require().True(value.Sign() > 0, "expected minted amount > 0")
}

// TestLiquidUnstakeEmitsERC20TransferEvent verifies that LiquidUnstake emits
// a Transfer(delegator -> address(0), burnedAmount) event on the liquidBondDenom ERC-20.
func (s *PrecompileTestSuite) TestLiquidUnstakeEmitsERC20TransferEvent() {
	s.SetupTest()
	ctx := s.nw.GetContext().WithBlockTime(time.Now())
	delegator := s.keyring.GetKey(0)

	// First liquid-stake to get some liquidBondDenom tokens
	lsKeeper := s.nw.App.GetLiquidStakeKeeper()
	_, err := lsKeeper.LiquidStake(
		ctx,
		liquidstaketypes.LiquidStakeProxyAcc,
		delegator.AccAddr,
		sdk.NewCoin(s.bondDenom, sdkmath.NewInt(1_000_000_000_000_000_000)),
	)
	s.Require().NoError(err)

	unstakeAmount := big.NewInt(1_000_000_000_000_000_000)
	input, err := s.precompile.Pack(liquidstake.LiquidUnstakeMethod, delegator.Addr, unstakeAmount)
	s.Require().NoError(err)

	stDB, err := s.runPrecompileTx(ctx, delegator, input, 1_000_000)
	s.Require().NoError(err)

	log := s.findERC20TransferLog(stDB, s.liquidBondERC20Addr)
	s.Require().NotNil(log, "expected ERC-20 Transfer log for liquidBondDenom")

	// topics[1] = from (delegator for burn), topics[2] = to (address(0))
	s.Require().Equal(delegator.Addr, common.BytesToAddress(log.Topics[1].Bytes()), "expected Transfer from delegator")
	s.Require().Equal(common.Hash{}, log.Topics[2], "expected Transfer to zero address (burn)")

	value := new(big.Int).SetBytes(log.Data)
	s.Require().True(value.Sign() > 0, "expected burned amount > 0")
}
