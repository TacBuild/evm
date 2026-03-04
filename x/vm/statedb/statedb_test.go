package statedb_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/evm/testutil/constants"
	"github.com/cosmos/evm/x/vm/statedb"
	"github.com/cosmos/evm/x/vm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// init configures global test EVM coin info (used by types.AdjustExtraDecimals*)
// to avoid panics when statedb balance helpers are used.
func init() {
	configurator := types.NewEVMConfigurator()
	_ = configurator.WithEVMCoinInfo(constants.ExampleAttoDenom, uint8(types.EighteenDecimals)).Configure()
}

var (
	address       common.Address   = common.BigToAddress(big.NewInt(101))
	address2      common.Address   = common.BigToAddress(big.NewInt(102))
	address3      common.Address   = common.BigToAddress(big.NewInt(103))
	blockHash     common.Hash      = common.BigToHash(big.NewInt(9999))
	emptyTxConfig statedb.TxConfig = statedb.NewEmptyTxConfig(blockHash)
)

type StateDBTestSuite struct {
	suite.Suite
}

func (suite *StateDBTestSuite) TestAccount() {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(2))
	key2 := common.BigToHash(big.NewInt(3))
	value2 := common.BigToHash(big.NewInt(4))
	testCases := []struct {
		name     string
		malleate func(*statedb.StateDB)
	}{
		{"non-exist account", func(db *statedb.StateDB) {
			suite.Require().Equal(false, db.Exist(address))
			suite.Require().Equal(true, db.Empty(address))
			suite.Require().Equal(common.U2560, db.GetBalance(address))
			suite.Require().Equal([]byte(nil), db.GetCode(address))
			suite.Require().Equal(common.Hash{}, db.GetCodeHash(address))
			suite.Require().Equal(uint64(0), db.GetNonce(address))
		}},
		{"empty account", func(db *statedb.StateDB) {
			db.CreateAccount(address)
			suite.Require().NoError(db.Commit())

			keeper := db.Keeper().(*MockKeeper)
			acct := keeper.accounts[address]
			suite.Require().Equal(statedb.NewEmptyAccount(), &acct.account)
			suite.Require().Empty(acct.states)
			suite.Require().False(acct.account.IsContract())

			db = statedb.New(sdk.Context{}, keeper, emptyTxConfig)
			suite.Require().Equal(true, db.Exist(address))
			suite.Require().Equal(true, db.Empty(address))
			suite.Require().Equal(common.U2560, db.GetBalance(address))
			suite.Require().Equal([]byte(nil), db.GetCode(address))
			suite.Require().Equal(common.BytesToHash(emptyCodeHash), db.GetCodeHash(address))
			suite.Require().Equal(uint64(0), db.GetNonce(address))
		}},
		{"suicide", func(db *statedb.StateDB) {
			// non-exist account.
			db.SelfDestruct(address)
			suite.Require().False(db.HasSelfDestructed(address))

			// create a contract account
			db.CreateAccount(address)
			db.SetCode(address, []byte("hello world"))
			db.AddBalance(address, uint256.NewInt(100))
			db.SetState(address, key1, value1)
			db.SetState(address, key2, value2)
			suite.Require().NoError(db.Commit())

			// suicide
			db = statedb.New(sdk.Context{}, db.Keeper(), emptyTxConfig)
			suite.Require().False(db.HasSelfDestructed(address))
			db.SelfDestruct(address)

			// check dirty state
			suite.Require().True(db.HasSelfDestructed(address))
			// balance is cleared
			suite.Require().Equal(common.U2560, db.GetBalance(address))
			// but code and state are still accessible in dirty state
			suite.Require().Equal(value1, db.GetState(address, key1))
			suite.Require().Equal([]byte("hello world"), db.GetCode(address))

			suite.Require().NoError(db.Commit())

			// not accessible from StateDB anymore
			db = statedb.New(sdk.Context{}, db.Keeper(), emptyTxConfig)
			suite.Require().False(db.Exist(address))

			// and cleared in keeper too
			keeper := db.Keeper().(*MockKeeper)
			suite.Require().Empty(keeper.accounts)
			suite.Require().Empty(keeper.codes)
		}},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			keeper := NewMockKeeper()
			db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
			tc.malleate(db)
		})
	}
}

func (suite *StateDBTestSuite) TestAccountOverride() {
	keeper := NewMockKeeper()
	db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
	// test balance carry over when overwritten
	amount := uint256.NewInt(1)

	// init an EOA account, account overridden only happens on EOA account.
	db.AddBalance(address, amount)
	db.SetNonce(address, 1)

	// override
	db.CreateAccount(address)

	// check balance is not lost
	suite.Require().Equal(amount, db.GetBalance(address))
	// but nonce is reset
	suite.Require().Equal(uint64(0), db.GetNonce(address))
}

func (suite *StateDBTestSuite) TestDBError() {
	testCases := []struct {
		name     string
		malleate func(vm.StateDB)
	}{
		{"set account", func(db vm.StateDB) {
			db.SetNonce(errAddress, 1)
		}},
		{"delete account", func(db vm.StateDB) {
			db.SetNonce(errAddress, 1)
			db.SelfDestruct(errAddress)
			suite.Require().True(db.HasSelfDestructed(errAddress))
		}},
	}
	for _, tc := range testCases {
		db := statedb.New(sdk.Context{}, NewMockKeeper(), emptyTxConfig)
		tc.malleate(db)
		suite.Require().Error(db.Commit())
	}
}

func (suite *StateDBTestSuite) TestBalance() {
	// NOTE: no need to test overflow/underflow, that is guaranteed by evm implementation.
	testCases := []struct {
		name       string
		malleate   func(*statedb.StateDB)
		expBalance *uint256.Int
	}{
		{"add balance", func(db *statedb.StateDB) {
			db.AddBalance(address, uint256.NewInt(10))
		}, uint256.NewInt(10)},
		{"sub balance", func(db *statedb.StateDB) {
			db.AddBalance(address, uint256.NewInt(10))
			// get dirty balance
			suite.Require().Equal(uint256.NewInt(10), db.GetBalance(address))
			db.SubBalance(address, uint256.NewInt(2))
		}, uint256.NewInt(8)},
		{"add zero balance", func(db *statedb.StateDB) {
			db.AddBalance(address, uint256.NewInt(0))
		}, uint256.NewInt(0)},
		{"sub zero balance", func(db *statedb.StateDB) {
			db.SubBalance(address, uint256.NewInt(0))
		}, uint256.NewInt(0)},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			keeper := NewMockKeeper()
			db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
			tc.malleate(db)

			// check dirty state
			suite.Require().Equal(tc.expBalance, db.GetBalance(address))
			suite.Require().NoError(db.Commit())
			// check committed balance too
			suite.Require().Equal(tc.expBalance, keeper.accounts[address].account.Balance)
		})
	}
}

func (suite *StateDBTestSuite) TestState() {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(1))
	testCases := []struct {
		name      string
		malleate  func(*statedb.StateDB)
		expStates statedb.Storage
	}{
		{"empty state", func(_ *statedb.StateDB) {
		}, nil},
		{"set empty value", func(db *statedb.StateDB) {
			db.SetState(address, key1, common.Hash{})
		}, statedb.Storage{}},
		{"set state even if same as original value (due to possible reverts within precompile calls)", func(db *statedb.StateDB) {
			db.SetState(address, key1, value1)
			db.SetState(address, key1, common.Hash{})
		}, statedb.Storage{
			key1: common.Hash{},
		}},
		{"set state", func(db *statedb.StateDB) {
			// check empty initial state
			suite.Require().Equal(common.Hash{}, db.GetState(address, key1))
			suite.Require().Equal(common.Hash{}, db.GetCommittedState(address, key1))

			// set state
			db.SetState(address, key1, value1)
			// query dirty state
			suite.Require().Equal(value1, db.GetState(address, key1))
			// check committed state is still not exist
			suite.Require().Equal(common.Hash{}, db.GetCommittedState(address, key1))

			// set same value again, should be noop
			db.SetState(address, key1, value1)
			suite.Require().Equal(value1, db.GetState(address, key1))
		}, statedb.Storage{
			key1: value1,
		}},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			keeper := NewMockKeeper()
			db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
			tc.malleate(db)
			suite.Require().NoError(db.Commit())

			// check committed states in keeper
			suite.Require().Equal(tc.expStates, keeper.accounts[address].states)

			// check ForEachStorage
			db = statedb.New(sdk.Context{}, keeper, emptyTxConfig)
			collected := CollectContractStorage(db)
			if len(tc.expStates) > 0 {
				suite.Require().Equal(tc.expStates, collected)
			} else {
				suite.Require().Empty(collected)
			}
		})
	}
}

func (suite *StateDBTestSuite) TestCode() {
	code := []byte("hello world")
	codeHash := crypto.Keccak256Hash(code)

	testCases := []struct {
		name        string
		malleate    func(vm.StateDB)
		expCode     []byte
		expCodeHash common.Hash
	}{
		{"non-exist account", func(vm.StateDB) {}, nil, common.Hash{}},
		{"empty account", func(db vm.StateDB) {
			db.CreateAccount(address)
		}, nil, common.BytesToHash(emptyCodeHash)},
		{"set code", func(db vm.StateDB) {
			db.SetCode(address, code)
		}, code, codeHash},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			keeper := NewMockKeeper()
			db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
			tc.malleate(db)

			// check dirty state
			suite.Require().Equal(tc.expCode, db.GetCode(address))
			suite.Require().Equal(len(tc.expCode), db.GetCodeSize(address))
			suite.Require().Equal(tc.expCodeHash, db.GetCodeHash(address))

			suite.Require().NoError(db.Commit())

			// check again
			db = statedb.New(sdk.Context{}, keeper, emptyTxConfig)
			suite.Require().Equal(tc.expCode, db.GetCode(address))
			suite.Require().Equal(len(tc.expCode), db.GetCodeSize(address))
			suite.Require().Equal(tc.expCodeHash, db.GetCodeHash(address))
		})
	}
}

func (suite *StateDBTestSuite) TestRevertSnapshot() {
	v1 := common.BigToHash(big.NewInt(1))
	v2 := common.BigToHash(big.NewInt(2))
	v3 := common.BigToHash(big.NewInt(3))
	testCases := []struct {
		name     string
		malleate func(vm.StateDB)
	}{
		{"set state", func(db vm.StateDB) {
			db.SetState(address, v1, v3)
		}},
		{"set nonce", func(db vm.StateDB) {
			db.SetNonce(address, 10)
		}},
		{"change balance", func(db vm.StateDB) {
			db.AddBalance(address, uint256.NewInt(10))
			db.SubBalance(address, uint256.NewInt(5))
		}},
		{"override account", func(db vm.StateDB) {
			db.CreateAccount(address)
		}},
		{"set code", func(db vm.StateDB) {
			db.SetCode(address, []byte("hello world"))
		}},
		{"suicide", func(db vm.StateDB) {
			db.SetState(address, v1, v2)
			db.SetCode(address, []byte("hello world"))
			db.SelfDestruct(address)
			suite.Require().True(db.HasSelfDestructed(address))
		}},
		{"add log", func(db vm.StateDB) {
			db.AddLog(&ethtypes.Log{
				Address: address,
			})
		}},
		{"add refund", func(db vm.StateDB) {
			db.AddRefund(10)
			db.SubRefund(5)
		}},
		{"access list", func(db vm.StateDB) {
			db.AddAddressToAccessList(address)
			db.AddSlotToAccessList(address, v1)
		}},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			ctx := sdk.Context{}
			keeper := NewMockKeeper()

			{
				// do some arbitrary changes to the storage
				db := statedb.New(ctx, keeper, emptyTxConfig)
				db.SetNonce(address, 1)
				db.AddBalance(address, uint256.NewInt(100))
				db.SetCode(address, []byte("hello world"))
				db.SetState(address, v1, v2)
				db.SetNonce(address2, 1)
				suite.Require().NoError(db.Commit())
			}

			originalKeeper := keeper.Clone()

			// run test
			db := statedb.New(ctx, keeper, emptyTxConfig)
			rev := db.Snapshot()
			tc.malleate(db)
			db.RevertToSnapshot(rev)

			// check empty states after revert
			suite.Require().Zero(db.GetRefund())
			suite.Require().Empty(db.Logs())

			suite.Require().NoError(db.Commit())

			// check keeper should stay the same
			suite.Require().Equal(originalKeeper, keeper)
		})
	}
}

func (suite *StateDBTestSuite) TestNestedSnapshot() {
	key := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(1))
	value2 := common.BigToHash(big.NewInt(2))

	db := statedb.New(sdk.Context{}, NewMockKeeper(), emptyTxConfig)

	rev1 := db.Snapshot()
	db.SetState(address, key, value1)

	rev2 := db.Snapshot()
	db.SetState(address, key, value2)
	suite.Require().Equal(value2, db.GetState(address, key))

	db.RevertToSnapshot(rev2)
	suite.Require().Equal(value1, db.GetState(address, key))

	db.RevertToSnapshot(rev1)
	suite.Require().Equal(common.Hash{}, db.GetState(address, key))
}

func (suite *StateDBTestSuite) TestInvalidSnapshotId() {
	db := statedb.New(sdk.Context{}, NewMockKeeper(), emptyTxConfig)
	suite.Require().Panics(func() {
		db.RevertToSnapshot(1)
	})
}

func (suite *StateDBTestSuite) TestAccessList() {
	value1 := common.BigToHash(big.NewInt(1))
	value2 := common.BigToHash(big.NewInt(2))

	testCases := []struct {
		name     string
		malleate func(vm.StateDB)
	}{
		{"add address", func(db vm.StateDB) {
			suite.Require().False(db.AddressInAccessList(address))
			db.AddAddressToAccessList(address)
			suite.Require().True(db.AddressInAccessList(address))

			addrPresent, slotPresent := db.SlotInAccessList(address, value1)
			suite.Require().True(addrPresent)
			suite.Require().False(slotPresent)

			// add again, should be no-op
			db.AddAddressToAccessList(address)
			suite.Require().True(db.AddressInAccessList(address))
		}},
		{"add slot", func(db vm.StateDB) {
			addrPresent, slotPresent := db.SlotInAccessList(address, value1)
			suite.Require().False(addrPresent)
			suite.Require().False(slotPresent)
			db.AddSlotToAccessList(address, value1)
			addrPresent, slotPresent = db.SlotInAccessList(address, value1)
			suite.Require().True(addrPresent)
			suite.Require().True(slotPresent)

			// add another slot
			db.AddSlotToAccessList(address, value2)
			addrPresent, slotPresent = db.SlotInAccessList(address, value2)
			suite.Require().True(addrPresent)
			suite.Require().True(slotPresent)

			// add again, should be noop
			db.AddSlotToAccessList(address, value2)
			addrPresent, slotPresent = db.SlotInAccessList(address, value2)
			suite.Require().True(addrPresent)
			suite.Require().True(slotPresent)
		}},
		{"prepare access list", func(db vm.StateDB) {
			al := ethtypes.AccessList{{
				Address:     address3,
				StorageKeys: []common.Hash{value1},
			}}

			rules := ethparams.Rules{
				ChainID:          big.NewInt(1000),
				IsHomestead:      true,
				IsEIP150:         true,
				IsEIP155:         true,
				IsEIP158:         true,
				IsByzantium:      true,
				IsConstantinople: true,
				IsPetersburg:     true,
				IsIstanbul:       true,
				IsBerlin:         true,
				IsLondon:         true,
				IsMerge:          true,
				IsShanghai:       true,
				IsCancun:         true,
			}
			db.Prepare(rules, address, common.Address{}, &address2, vm.PrecompiledAddressesBerlin, al)

			// check sender and dst
			suite.Require().True(db.AddressInAccessList(address))
			suite.Require().True(db.AddressInAccessList(address2))
			// check precompiles
			suite.Require().True(db.AddressInAccessList(common.BytesToAddress([]byte{1})))
			// check AccessList
			suite.Require().True(db.AddressInAccessList(address3))
			addrPresent, slotPresent := db.SlotInAccessList(address3, value1)
			suite.Require().True(addrPresent)
			suite.Require().True(slotPresent)
			addrPresent, slotPresent = db.SlotInAccessList(address3, value2)
			suite.Require().True(addrPresent)
			suite.Require().False(slotPresent)
		}},
	}

	for _, tc := range testCases {
		db := statedb.New(sdk.Context{}, NewMockKeeper(), emptyTxConfig)
		tc.malleate(db)
	}
}

func (suite *StateDBTestSuite) TestLog() {
	txHash := common.BytesToHash([]byte("tx"))
	// use a non-default tx config
	txConfig := statedb.NewTxConfig(
		blockHash,
		txHash,
		1, 1,
	)
	db := statedb.New(sdk.Context{}, NewMockKeeper(), txConfig)
	data := []byte("hello world")
	db.AddLog(&ethtypes.Log{
		Address:     address,
		Topics:      []common.Hash{},
		Data:        data,
		BlockNumber: 1,
	})
	suite.Require().Equal(1, len(db.Logs()))
	expecedLog := &ethtypes.Log{
		Address:     address,
		Topics:      []common.Hash{},
		Data:        data,
		BlockNumber: 1,
		BlockHash:   blockHash,
		TxHash:      txHash,
		TxIndex:     1,
		Index:       1,
	}
	suite.Require().Equal(expecedLog, db.Logs()[0])

	db.AddLog(&ethtypes.Log{
		Address:     address,
		Topics:      []common.Hash{},
		Data:        data,
		BlockNumber: 1,
	})
	suite.Require().Equal(2, len(db.Logs()))
	expecedLog.Index++
	suite.Require().Equal(expecedLog, db.Logs()[1])
}

func (suite *StateDBTestSuite) TestRefund() {
	testCases := []struct {
		name      string
		malleate  func(vm.StateDB)
		expRefund uint64
		expPanic  bool
	}{
		{"add refund", func(db vm.StateDB) {
			db.AddRefund(uint64(10))
		}, 10, false},
		{"sub refund", func(db vm.StateDB) {
			db.AddRefund(uint64(10))
			db.SubRefund(uint64(5))
		}, 5, false},
		{"negative refund counter", func(db vm.StateDB) {
			db.AddRefund(uint64(5))
			db.SubRefund(uint64(10))
		}, 0, true},
	}
	for _, tc := range testCases {
		db := statedb.New(sdk.Context{}, NewMockKeeper(), emptyTxConfig)
		if !tc.expPanic {
			tc.malleate(db)
			suite.Require().Equal(tc.expRefund, db.GetRefund())
		} else {
			suite.Require().Panics(func() {
				tc.malleate(db)
			})
		}
	}
}

func (suite *StateDBTestSuite) TestIterateStorage() {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(2))
	key2 := common.BigToHash(big.NewInt(3))
	value2 := common.BigToHash(big.NewInt(4))

	keeper := NewMockKeeper()
	db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
	db.SetState(address, key1, value1)
	db.SetState(address, key2, value2)

	// ForEachStorage only iterate committed state
	suite.Require().Empty(CollectContractStorage(db))

	suite.Require().NoError(db.Commit())

	storage := CollectContractStorage(db)
	suite.Require().Equal(2, len(storage))
	suite.Require().Equal(keeper.accounts[address].states, storage)

	// break early iteration
	storage = make(statedb.Storage)
	err := db.ForEachStorage(address, func(k, v common.Hash) bool {
		storage[k] = v
		// return false to break early
		return false
	})
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(storage))
}

func (suite *StateDBTestSuite) TestSetBalance() {
	testCases := []struct {
		name     string
		setup    func(*statedb.StateDB)
		addr     common.Address
		amount   *uint256.Int
		expExist bool
	}{
		{
			"set balance on new account",
			func(db *statedb.StateDB) {},
			address,
			uint256.NewInt(500),
			true,
		},
		{
			"set balance on existing account",
			func(db *statedb.StateDB) {
				db.AddBalance(address, uint256.NewInt(100))
			},
			address,
			uint256.NewInt(300),
			true,
		},
		{
			"set balance to zero on existing account",
			func(db *statedb.StateDB) {
				db.AddBalance(address, uint256.NewInt(100))
				db.SetNonce(address, 1)
			},
			address,
			uint256.NewInt(0),
			true, // account still exists due to nonce
		},
		{
			"set balance on different account",
			func(db *statedb.StateDB) {
				db.AddBalance(address, uint256.NewInt(100))
			},
			address2,
			uint256.NewInt(200),
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			db := statedb.New(sdk.Context{}, NewMockKeeper(), emptyTxConfig)
			tc.setup(db)
			db.SetBalance(tc.addr, tc.amount)

			suite.Require().Equal(tc.amount, db.GetBalance(tc.addr))
			suite.Require().Equal(tc.expExist, db.Exist(tc.addr))
		})
	}
}

func (suite *StateDBTestSuite) TestSetBalanceCommit() {
	keeper := NewMockKeeper()
	db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)

	// Create account with initial balance
	db.AddBalance(address, uint256.NewInt(100))
	suite.Require().NoError(db.Commit())

	// Override balance
	db = statedb.New(sdk.Context{}, keeper, emptyTxConfig)
	db.SetBalance(address, uint256.NewInt(999))
	suite.Require().Equal(uint256.NewInt(999), db.GetBalance(address))
	suite.Require().NoError(db.Commit())

	// Verify after re-read
	db = statedb.New(sdk.Context{}, keeper, emptyTxConfig)
	suite.Require().Equal(uint256.NewInt(999), db.GetBalance(address))
}

func (suite *StateDBTestSuite) TestSetStorage() {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(10))
	key2 := common.BigToHash(big.NewInt(2))
	value2 := common.BigToHash(big.NewInt(20))
	key3 := common.BigToHash(big.NewInt(3))
	value3 := common.BigToHash(big.NewInt(30))

	testCases := []struct {
		name    string
		setup   func(*statedb.StateDB)
		storage map[common.Hash]common.Hash
		checkFn func(*statedb.StateDB)
	}{
		{
			"set storage on new account",
			func(db *statedb.StateDB) {},
			map[common.Hash]common.Hash{key1: value1, key2: value2},
			func(db *statedb.StateDB) {
				suite.Require().Equal(value1, db.GetState(address, key1))
				suite.Require().Equal(value2, db.GetState(address, key2))
				suite.Require().Equal(common.Hash{}, db.GetState(address, key3), "non-overridden key should be zero")
			},
		},
		{
			"set storage replaces existing storage",
			func(db *statedb.StateDB) {
				// pre-set some storage
				db.SetState(address, key1, common.BigToHash(big.NewInt(999)))
				db.SetState(address, key3, value3)
			},
			map[common.Hash]common.Hash{key1: value1, key2: value2},
			func(db *statedb.StateDB) {
				suite.Require().Equal(value1, db.GetState(address, key1), "overridden key should have new value")
				suite.Require().Equal(value2, db.GetState(address, key2), "new key should be set")
				// key3 was in old storage but NOT in new override, should be zero due to stateOverriden
				suite.Require().Equal(common.Hash{}, db.GetState(address, key3), "old key not in override should be zero")
			},
		},
		{
			"set storage inherits code and nonce and balance",
			func(db *statedb.StateDB) {
				db.SetNonce(address, 5)
				db.AddBalance(address, uint256.NewInt(1000))
				db.SetCode(address, []byte("contract code"))
			},
			map[common.Hash]common.Hash{key1: value1},
			func(db *statedb.StateDB) {
				suite.Require().Equal(value1, db.GetState(address, key1))
				suite.Require().Equal(uint64(5), db.GetNonce(address), "nonce should be inherited")
				suite.Require().Equal(uint256.NewInt(1000), db.GetBalance(address), "balance should be inherited")
				suite.Require().Equal([]byte("contract code"), db.GetCode(address), "code should be inherited")
			},
		},
		{
			"set empty storage on existing account",
			func(db *statedb.StateDB) {
				db.SetState(address, key1, value1)
				db.SetState(address, key2, value2)
			},
			map[common.Hash]common.Hash{},
			func(db *statedb.StateDB) {
				// All keys should return zero since storage is replaced with empty map
				suite.Require().Equal(common.Hash{}, db.GetState(address, key1))
				suite.Require().Equal(common.Hash{}, db.GetState(address, key2))
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			db := statedb.New(sdk.Context{}, NewMockKeeper(), emptyTxConfig)
			tc.setup(db)
			db.SetStorage(address, tc.storage)
			tc.checkFn(db)
		})
	}
}

func (suite *StateDBTestSuite) TestSetStorageWithCommittedState() {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(10))
	key2 := common.BigToHash(big.NewInt(2))
	value2 := common.BigToHash(big.NewInt(20))
	key3 := common.BigToHash(big.NewInt(3))
	value3 := common.BigToHash(big.NewInt(30))

	keeper := NewMockKeeper()

	// First: commit some state
	{
		db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
		db.SetNonce(address, 1)
		db.AddBalance(address, uint256.NewInt(500))
		db.SetCode(address, []byte("old code"))
		db.SetState(address, key1, value1)
		db.SetState(address, key2, value2)
		suite.Require().NoError(db.Commit())
	}

	// Second: override storage with SetStorage (only key3)
	{
		db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
		db.SetStorage(address, map[common.Hash]common.Hash{key3: value3})

		// key3 should have the new value
		suite.Require().Equal(value3, db.GetState(address, key3))
		// key1 and key2 were committed but NOT in override — stateOverriden makes them zero
		suite.Require().Equal(common.Hash{}, db.GetState(address, key1), "committed key not in override should be zero")
		suite.Require().Equal(common.Hash{}, db.GetState(address, key2), "committed key not in override should be zero")

		// Metadata should be inherited
		suite.Require().Equal(uint64(1), db.GetNonce(address))
		suite.Require().Equal(uint256.NewInt(500), db.GetBalance(address))
		suite.Require().Equal([]byte("old code"), db.GetCode(address))
	}
}

func (suite *StateDBTestSuite) TestSetStorageRevert() {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(10))
	key2 := common.BigToHash(big.NewInt(2))
	value2 := common.BigToHash(big.NewInt(20))

	keeper := NewMockKeeper()

	// Commit initial state
	{
		db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
		db.SetNonce(address, 1)
		db.AddBalance(address, uint256.NewInt(100))
		db.SetState(address, key1, value1)
		suite.Require().NoError(db.Commit())
	}

	// Snapshot, SetStorage, then revert
	{
		db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)

		rev := db.Snapshot()

		db.SetStorage(address, map[common.Hash]common.Hash{key2: value2})
		suite.Require().Equal(value2, db.GetState(address, key2))

		db.RevertToSnapshot(rev)

		// After revert, original committed state should be accessible
		suite.Require().Equal(value1, db.GetState(address, key1), "original key should be restored after revert")
	}
}

func (suite *StateDBTestSuite) TestSetBalanceRevert() {
	keeper := NewMockKeeper()

	// Commit initial balance
	{
		db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)
		db.AddBalance(address, uint256.NewInt(100))
		suite.Require().NoError(db.Commit())
	}

	// Snapshot, SetBalance, then revert
	{
		db := statedb.New(sdk.Context{}, keeper, emptyTxConfig)

		rev := db.Snapshot()
		db.SetBalance(address, uint256.NewInt(999))
		suite.Require().Equal(uint256.NewInt(999), db.GetBalance(address))

		db.RevertToSnapshot(rev)
		suite.Require().Equal(uint256.NewInt(100), db.GetBalance(address), "balance should be restored after revert")
	}
}

func CollectContractStorage(db vm.StateDB) statedb.Storage {
	storage := make(statedb.Storage)
	stDB, ok := db.(*statedb.StateDB)
	if !ok {
		panic(fmt.Sprintf("invalid stateDB type %T", db))
	}
	err := stDB.ForEachStorage(address, func(k, v common.Hash) bool {
		storage[k] = v
		return true
	})
	if err != nil {
		return nil
	}

	return storage
}

func TestStateDBTestSuite(t *testing.T) {
	suite.Run(t, &StateDBTestSuite{})
}
