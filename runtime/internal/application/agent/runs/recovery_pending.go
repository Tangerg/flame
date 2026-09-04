package runs

import "fmt"

// indexRecoveryPending admits the complete claimed barrier catalog before any
// row can influence boot recovery. The returned map is the sole lookup used by
// tree planning.
func indexRecoveryPending(values []Pending, activeRootSessions map[string]string) (map[string]Pending, error) {
	byRoot := make(map[string]Pending, len(values))
	checkpointOwners := make(map[string]string, len(values))
	for index, pending := range values {
		if err := pending.Validate(); err != nil {
			return nil, fmt.Errorf("runs: recovery Pending[%d]: %w", index, err)
		}
		sessionID, active := activeRootSessions[pending.RootRunID]
		if !active {
			return nil, fmt.Errorf("runs: recovery Pending %q has no claimed active root", pending.RootRunID)
		}
		if pending.SessionID != sessionID {
			return nil, fmt.Errorf(
				"runs: recovery Pending %q belongs to Session %q, want %q",
				pending.RootRunID,
				pending.SessionID,
				sessionID,
			)
		}
		if _, duplicate := byRoot[pending.RootRunID]; duplicate {
			return nil, fmt.Errorf("runs: recovery has duplicate Pending for root Run %q", pending.RootRunID)
		}
		root, _ := pending.RootContinuation()
		if owner, duplicate := checkpointOwners[root.MemberID]; duplicate {
			return nil, fmt.Errorf(
				"runs: recovery checkpoint %q is owned by interrupts %q and %q",
				root.MemberID,
				owner,
				pending.RootRunID,
			)
		}
		checkpointOwners[root.MemberID] = pending.RootRunID
		byRoot[pending.RootRunID] = pending
	}
	return byRoot, nil
}
