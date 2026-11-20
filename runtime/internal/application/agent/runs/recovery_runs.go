package runs

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

type recoveryRunTree struct {
	root      rundomain.Run
	runsByID  map[identity.RunID]rundomain.Run
	postorder []identity.RunID
}

func recoveryRunCatalogSessionIDs(values []rundomain.Run) []identity.SessionID {
	sessions := make(map[identity.SessionID]struct{}, len(values))
	for _, value := range values {
		sessions[value.SessionID()] = struct{}{}
	}
	ids := make([]identity.SessionID, 0, len(sessions))
	for sessionID := range sessions {
		ids = append(ids, sessionID)
	}
	slices.SortFunc(ids, identity.SessionID.Compare)
	return ids
}

func groupRecoveryRunTrees(active []rundomain.Run) (map[identity.RunID]recoveryRunTree, error) {
	grouped := make(map[identity.RunID][]rundomain.Run)
	for _, run := range active {
		rootRunID := run.Lineage().TreeRootID(run.ID())
		grouped[rootRunID] = append(grouped[rootRunID], run)
	}

	trees := make(map[identity.RunID]recoveryRunTree, len(grouped))
	for rootRunID, runs := range grouped {
		members := make([]rundomain.TreeMember, 0, len(runs))
		runsByID := make(map[identity.RunID]rundomain.Run, len(runs))
		for _, run := range runs {
			members = append(members, rundomain.TreeMember{RunID: run.ID(), Lineage: run.Lineage()})
			runsByID[run.ID()] = run
		}
		topology, err := rundomain.NewTree(rootRunID, members)
		if err != nil {
			return nil, fmt.Errorf("runs: assemble recovery Run tree %q: %w", rootRunID, err)
		}
		root := runsByID[rootRunID]
		for _, run := range runs {
			if run.SessionID() != root.SessionID() {
				return nil, fmt.Errorf(
					"runs: recovery Run %q belongs to Session %q, want tree Session %q",
					run.ID(),
					run.SessionID(),
					root.SessionID(),
				)
			}
			if run.State() != root.State() {
				return nil, fmt.Errorf(
					"runs: recovery Run %q is %s while root Run %q is %s",
					run.ID(),
					run.State(),
					root.ID(),
					root.State(),
				)
			}
		}
		trees[rootRunID] = recoveryRunTree{root: root, runsByID: runsByID, postorder: topology.Postorder()}
	}
	return trees, nil
}
