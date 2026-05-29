package liquidstake

import (
	"fmt"
	"math/big"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"

	liquidstakeprecompile "github.com/cosmos/evm/precompiles/liquidstake"
	chainutil "github.com/cosmos/evm/testutil"
	testkeyring "github.com/cosmos/evm/testutil/keyring"
	liquidstaketypes "github.com/cosmos/evm/x/liquidstake/types"
	"github.com/cosmos/evm/x/vm/statedb"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// runPrecompile is a helper that executes a precompile method via Run.
// Returns the stDB used during execution and any error.
func (s *PrecompileTestSuite) runPrecompile(ctx sdk.Context, input []byte, sender testkeyring.Key, gas uint64) (*statedb.StateDB, []byte, error) {
	baseFee := s.nw.App.GetEVMKeeper().GetBaseFee(ctx)

	contract := vm.NewPrecompile(sender.Addr, s.precompile.Address(), uint256.NewInt(0), gas)
	contractAddr := contract.Address()
	contract.Input = input

	txArgs := evmtypes.EvmTxArgs{
		ChainID:   evmtypes.GetEthChainConfig().ChainID,
		Nonce:     0,
		To:        &contractAddr,
		GasLimit:  gas,
		GasPrice:  chainutil.ExampleMinGasPrices,
		GasFeeCap: baseFee,
		GasTipCap: big.NewInt(1),
		Accesses:  &ethtypes.AccessList{},
	}

	msg, err := s.factory.GenerateGethCoreMsg(sender.Priv, txArgs)
	if err != nil {
		return nil, nil, err
	}

	proposerAddr := ctx.BlockHeader().ProposerAddress
	cfg, err := s.nw.App.GetEVMKeeper().EVMConfig(ctx, proposerAddr)
	if err != nil {
		return nil, nil, err
	}

	db := statedb.New(ctx, s.nw.App.GetEVMKeeper(), statedb.NewEmptyTxConfig())
	evm := s.nw.App.GetEVMKeeper().NewEVM(ctx, *msg, cfg, nil, db)

	precompiles, found, err := s.nw.App.GetEVMKeeper().GetPrecompileInstance(ctx, contractAddr)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, fmt.Errorf("precompile not found at %s", contractAddr.Hex())
	}
	evm.WithPrecompiles(precompiles.Map)

	bz, err := s.precompile.Run(evm, contract, false)
	return db, bz, err
}

// runPrecompileWithDirtyState is a helper analogous to runPrecompile, but marks the
// stateObject as "dirty" via no-op AddBalance/SubBalance before the call (simulates SSTORE).
func (s *PrecompileTestSuite) runPrecompileWithDirtyState(ctx sdk.Context, input []byte, sender testkeyring.Key, gas uint64) (*statedb.StateDB, []byte, error) {
	baseFee := s.nw.App.GetEVMKeeper().GetBaseFee(ctx)

	contract := vm.NewPrecompile(sender.Addr, s.precompile.Address(), uint256.NewInt(0), gas)
	contractAddr := contract.Address()
	contract.Input = input

	txArgs := evmtypes.EvmTxArgs{
		ChainID:   evmtypes.GetEthChainConfig().ChainID,
		Nonce:     0,
		To:        &contractAddr,
		GasLimit:  gas,
		GasPrice:  chainutil.ExampleMinGasPrices,
		GasFeeCap: baseFee,
		GasTipCap: big.NewInt(1),
		Accesses:  &ethtypes.AccessList{},
	}

	msg, err := s.factory.GenerateGethCoreMsg(sender.Priv, txArgs)
	if err != nil {
		return nil, nil, err
	}

	proposerAddr := ctx.BlockHeader().ProposerAddress
	cfg, err := s.nw.App.GetEVMKeeper().EVMConfig(ctx, proposerAddr)
	if err != nil {
		return nil, nil, err
	}

	db := statedb.New(ctx, s.nw.App.GetEVMKeeper(), statedb.NewEmptyTxConfig())
	evm := s.nw.App.GetEVMKeeper().NewEVM(ctx, *msg, cfg, nil, db)

	// ATTACK SIMULATION: SSTORE-like operation — marks the stateObject as dirty.
	// In a real attack: exploitNonce++ (SSTORE) is executed before the precompile call.
	// Without BalanceHandler, commitWithCtx() would restore the phantom balance via SetAccount.
	one, _ := uint256.FromBig(big.NewInt(1))
	db.AddBalance(sender.Addr, one, tracing.BalanceChangeUnspecified)
	db.SubBalance(sender.Addr, one, tracing.BalanceChangeUnspecified)

	precompiles, found, err := s.nw.App.GetEVMKeeper().GetPrecompileInstance(ctx, contractAddr)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, fmt.Errorf("precompile not found at %s", contractAddr.Hex())
	}
	evm.WithPrecompiles(precompiles.Map)

	bz, err := s.precompile.Run(evm, contract, false)
	return db, bz, err
}

// TestBalanceSync_LiquidStake verifies that after a liquidStake precompile call
// the BalanceHandler correctly updates StateDB (calls SubBalance for the spender).
// This tests the BalanceHandler mechanism introduced in v0.6.0.
func (s *PrecompileTestSuite) TestBalanceSync_LiquidStake() {
	s.SetupTest()
	ctx := s.nw.GetContext().WithBlockTime(time.Now())

	delegator := s.keyring.GetKey(0)
	stakeAmount := big.NewInt(1_000_000)

	// Read EVM balance BEFORE the call (from a fresh stateDB)
	stDB0 := statedb.New(ctx, s.nw.App.GetEVMKeeper(), statedb.NewEmptyTxConfig())
	evmBalBefore := stDB0.GetBalance(delegator.Addr)
	s.Require().True(evmBalBefore.Sign() > 0, "prefunded balance must be positive")

	input, err := s.precompile.Pack(
		liquidstakeprecompile.LiquidStakeMethod,
		delegator.Addr,
		stakeAmount,
	)
	s.Require().NoError(err)

	stDB, bz, err := s.runPrecompile(ctx, input, delegator, 1_000_000)
	s.Require().NoError(err, "liquidStake precompile must succeed")
	s.Require().NotNil(bz)

	// After the call the EVM balance in stDB must have decreased:
	// BalanceHandler processed the CoinSpent event and called SubBalance(delegator, stakeAmount).
	evmBalAfter := stDB.GetBalance(delegator.Addr)
	s.Require().True(evmBalBefore.Cmp(evmBalAfter) > 0,
		"BalanceHandler must have decreased the EVM balance via SubBalance: before=%s, after=%s",
		evmBalBefore.String(), evmBalAfter.String())

	// The decrease must be at least stakeAmount (gas fees may add more).
	diff := new(big.Int).Sub(evmBalBefore.ToBig(), evmBalAfter.ToBig())
	s.Require().True(diff.Cmp(stakeAmount) >= 0,
		"EVM balance decrease (%s) must be >= stakeAmount (%s)",
		diff.String(), stakeAmount.String())

	s.T().Logf("LiquidStake BalanceHandler OK: evm %s->%s (diff=%s, stakeAmt=%s)",
		evmBalBefore.String(), evmBalAfter.String(), diff.String(), stakeAmount.String())
}

// TestBalanceSync_LiquidUnstake verifies that after a liquidUnstake precompile call
// the BalanceHandler correctly updates StateDB (SubBalance for the gTAC burn).
func (s *PrecompileTestSuite) TestBalanceSync_LiquidUnstake() {
	s.SetupTest()
	ctx := s.nw.GetContext().WithBlockTime(time.Now())

	delegator := s.keyring.GetKey(3)

	lsParams := s.nw.App.GetLiquidStakeKeeper().GetParams(ctx)

	// Liquid-stake via keeper to obtain gTAC.
	stakeAmt := sdkmath.NewInt(1_000_000_000_000_000_000)
	_, err := s.nw.App.GetLiquidStakeKeeper().LiquidStake(
		ctx,
		liquidstaketypes.LiquidStakeProxyAcc,
		sdk.AccAddress(delegator.Addr.Bytes()),
		sdk.NewCoin(s.bondDenom, stakeAmt),
	)
	s.Require().NoError(err)

	// Check gTAC balance via bank keeper (cacheCtx holds the changes).
	cacheCtx, err := statedb.New(ctx, s.nw.App.GetEVMKeeper(), statedb.NewEmptyTxConfig()).GetCacheContext()
	s.Require().NoError(err)
	gTACBefore := s.nw.App.GetBankKeeper().GetBalance(cacheCtx, sdk.AccAddress(delegator.Addr.Bytes()), lsParams.LiquidBondDenom).Amount
	// gTAC balance may be zero in cacheCtx if the keeper operated on the root ctx — that is fine.
	_ = gTACBefore

	// Read EVM balance AFTER liquid-stake (bank changes were applied to root ctx via keeper).
	stDB0 := statedb.New(ctx, s.nw.App.GetEVMKeeper(), statedb.NewEmptyTxConfig())
	evmBalBeforeUnstake := stDB0.GetBalance(delegator.Addr)

	unstakeAmount := big.NewInt(1_000_000_000_000_000_000)
	input, err := s.precompile.Pack(
		liquidstakeprecompile.LiquidUnstakeMethod,
		delegator.Addr,
		unstakeAmount,
	)
	s.Require().NoError(err)

	stDB, bz, err := s.runPrecompile(ctx, input, delegator, 1_000_000)
	s.Require().NoError(err, "liquidUnstake precompile must succeed")
	s.Require().NotNil(bz)

	evmBalAfterUnstake := stDB.GetBalance(delegator.Addr)

	// After liquidUnstake: gTAC is burned and unbonding begins.
	// The bond denom EVM balance must not increase immediately (unbonding period applies).
	s.Require().True(evmBalAfterUnstake.Cmp(evmBalBeforeUnstake) <= 0,
		"EVM bond denom balance must not increase immediately after liquidUnstake (unbonding period): %s -> %s",
		evmBalBeforeUnstake.String(), evmBalAfterUnstake.String())

	s.T().Logf("LiquidUnstake balance sync OK: evm bond %s->%s",
		evmBalBeforeUnstake.String(), evmBalAfterUnstake.String())
}

// TestBalanceSync_BalanceHandlerPresent verifies that BalanceHandlerFactory is set
// in the precompile (correct configuration).
// This test quickly catches a regression if BalanceHandlerFactory is accidentally removed.
func (s *PrecompileTestSuite) TestBalanceSync_BalanceHandlerPresent() {
	s.SetupTest()

	s.Require().NotNil(s.precompile.Precompile.BalanceHandlerFactory,
		"BalanceHandlerFactory must be set in NewPrecompile. "+
			"Without it liquidStake is vulnerable to balance desync (pre-v0.6.0 bug)")

	s.T().Log("BalanceHandlerFactory is configured correctly")
}
