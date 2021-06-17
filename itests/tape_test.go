package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/itests/kit2"
	"github.com/filecoin-project/lotus/node"
	"github.com/stretchr/testify/require"
)

func TestTapeFix(t *testing.T) {
	kit2.QuietMiningLogs()

	var blocktime = 2 * time.Millisecond

	// The "before" case is disabled, because we need the builder to mock 32 GiB sectors to accurately repro this case
	// TODO: Make the mock sector size configurable and reenable this
	// t.Run("before", func(t *testing.T) { testTapeFix(t, b, blocktime, false) })
	t.Run("after", func(t *testing.T) { testTapeFix(t, blocktime, true) })
}

func testTapeFix(t *testing.T, blocktime time.Duration, after bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upgradeSchedule := stmgr.UpgradeSchedule{{
		Network:   build.ActorUpgradeNetworkVersion,
		Height:    1,
		Migration: stmgr.UpgradeActorsV2,
	}}
	if after {
		upgradeSchedule = append(upgradeSchedule, stmgr.Upgrade{
			Network: network.Version5,
			Height:  2,
		})
	}

	nopts := kit2.ConstructorOpts(node.Override(new(stmgr.UpgradeSchedule), upgradeSchedule))
	_, miner, ens := kit2.EnsembleMinimal(t, kit2.MockProofs(), nopts)
	ens.InterconnectAll().BeginMining(blocktime)

	sid, err := miner.PledgeSector(ctx)
	require.NoError(t, err)

	t.Log("All sectors is fsm")

	// If before, we expect the precommit to fail
	successState := api.SectorState(sealing.CommitFailed)
	failureState := api.SectorState(sealing.Proving)
	if after {
		// otherwise, it should succeed.
		successState, failureState = failureState, successState
	}

	for {
		st, err := miner.SectorsStatus(ctx, sid.Number, false)
		require.NoError(t, err)
		if st.State == successState {
			break
		}
		require.NotEqual(t, failureState, st.State)
		build.Clock.Sleep(100 * time.Millisecond)
		t.Log("WaitSeal")
	}
}
