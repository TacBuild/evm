package liquidstake

import (
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/evm/x/liquidstake/types"
)

func (s *KeeperTestSuite) TestInitGenesis() {
	genState := *types.DefaultGenesisState()
	s.keeper.InitGenesis(s.ctx(), genState)
	got := s.keeper.ExportGenesis(s.ctx())
	s.Require().Equal(genState, *got)
}

func (s *KeeperTestSuite) TestImportExportGenesis() {
	// Use existing bonded validators from the network
	_, valOpers := s.setupWhitelistedValidators(2, 0)

	ctx := s.ctx()
	// Ensure exactly the 2 whitelisted validators are set
	params := s.keeper.GetParams(ctx)
	params.WhitelistedValidators = []types.WhitelistedValidator{
		{ValidatorAddress: valOpers[0].String(), TargetWeight: sdkmath.NewInt(5000)},
		{ValidatorAddress: valOpers[1].String(), TargetWeight: sdkmath.NewInt(5000)},
	}
	params.ModulePaused = false
	s.Require().NoError(s.keeper.SetParams(ctx, params))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	s.Require().NoError(s.liquidStaking(s.delAddrs[0], sdkmath.NewInt(100_000_000)))

	ctx = s.ctx()
	lvs := s.keeper.GetAllLiquidValidators(ctx)
	s.Require().Len(lvs, 2)

	lvStates := s.keeper.GetAllLiquidValidatorStates(ctx)
	genState := s.keeper.ExportGenesis(ctx)

	bz := s.nw.App.AppCodec().MustMarshalJSON(genState)
	var genState2 types.GenesisState
	s.nw.App.AppCodec().MustUnmarshalJSON(bz, &genState2)
	s.keeper.InitGenesis(ctx, genState2)
	genState3 := s.keeper.ExportGenesis(ctx)

	s.Require().Equal(*genState, genState2)
	s.Require().Equal(genState2, *genState3)

	lvs = s.keeper.GetAllLiquidValidators(ctx)
	s.Require().Len(lvs, 2)

	lvStates3 := s.keeper.GetAllLiquidValidatorStates(ctx)
	s.Require().EqualValues(lvStates, lvStates3)
}

func (s *KeeperTestSuite) TestImportExportGenesisEmpty() {
	ctx := s.ctx()
	s.Require().NoError(s.keeper.SetParams(ctx, types.DefaultParams()))
	s.keeper.UpdateLiquidValidatorSet(ctx, true)

	genState := s.keeper.ExportGenesis(ctx)
	bz := s.nw.App.AppCodec().MustMarshalJSON(genState)

	var genState2 types.GenesisState
	s.nw.App.AppCodec().MustUnmarshalJSON(bz, &genState2)
	s.keeper.InitGenesis(ctx, genState2)

	genState3 := s.keeper.ExportGenesis(ctx)
	s.Require().Equal(*genState, genState2)
	s.Require().Equal(genState2, *genState3)
}
