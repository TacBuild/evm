package liquidstake

//import (
//	"fmt"
//	"math/big"
//	"strings"
//
//	"github.com/ethereum/go-ethereum/accounts/abi"
//	"github.com/ethereum/go-ethereum/core/vm"
//
//	"github.com/cosmos/evm/precompiles/authorization"
//	cmn "github.com/cosmos/evm/precompiles/common"
//
//	sdk "github.com/cosmos/cosmos-sdk/types"
//	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
//	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
//)
//
//const (
//	// DelegationMethod defines the ABI method name for the staking Delegation
//	// query.
//	DelegationMethod = "delegation"
//	// UnbondingDelegationMethod defines the ABI method name for the staking
//	// UnbondingDelegationMethod query.
//	UnbondingDelegationMethod = "unbondingDelegation"
//	// ValidatorMethod defines the ABI method name for the staking
//	// Validator query.
//	ValidatorMethod = "validator"
//	// ValidatorsMethod defines the ABI method name for the staking
//	// Validators query.
//	ValidatorsMethod = "validators"
//	// RedelegationMethod defines the ABI method name for the staking
//	// Redelegation query.
//	RedelegationMethod = "redelegation"
//	// RedelegationsMethod defines the ABI method name for the staking
//	// Redelegations query.
//	RedelegationsMethod = "redelegations"
//)
//
//// Delegation returns the delegation that a delegator has with a specific validator.
//func (p Precompile) Delegation(
//	ctx sdk.Context,
//	_ *vm.Contract,
//	method *abi.Method,
//	args []interface{},
//) ([]byte, error) {
//	req, err := NewDelegationRequest(args)
//	if err != nil {
//		return nil, err
//	}
//
//	queryServer := stakingkeeper.Querier{Keeper: &p.liquidStakeKeeper}
//
//	res, err := queryServer.Delegation(ctx, req)
//	if err != nil {
//		// If there is no delegation found, return the response with zero values.
//		if strings.Contains(err.Error(), fmt.Sprintf(ErrNoDelegationFound, req.DelegatorAddr, req.ValidatorAddr)) {
//			bondDenom, err := p.liquidStakeKeeper.BondDenom(ctx)
//			if err != nil {
//				return nil, err
//			}
//			return method.Outputs.Pack(big.NewInt(0), cmn.Coin{Denom: bondDenom, Amount: big.NewInt(0)})
//		}
//
//		return nil, err
//	}
//
//	out := new(DelegationOutput).FromResponse(res)
//
//	return out.Pack(method.Outputs)
//}

