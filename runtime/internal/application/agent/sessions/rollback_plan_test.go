package sessions

import (
	"testing"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

func TestRollbackPlanOwnsResolvedBoundaryAndCheckpointScope(t *testing.T) {
	boundary := transcript.Boundary{
		KeepMessageMark: 4,
		Dropped:         []transcript.RunNode{{ID: "run_1"}, {ID: "run_2"}},
	}
	checkpointRoots := []string{"member_1"}
	rollback, err := NewRollbackPlan("ses_1", boundary, checkpointRoots, nil)
	if err != nil {
		t.Fatal(err)
	}
	boundary.Dropped[0].ID = "run_changed"
	checkpointRoots[0] = "member_changed"
	mark, known := rollback.TruncationMark()
	if mark != 4 || !known || rollback.SessionID() != "ses_1" {
		t.Fatalf("rollback coordinate = session:%q mark:%d known:%t", rollback.SessionID(), mark, known)
	}
	runIDs := rollback.DropRunIDs()
	rootIDs := rollback.CheckpointRootIDs()
	if len(runIDs) != 2 || runIDs[0] != "run_1" || len(rootIDs) != 1 || rootIDs[0] != "member_1" {
		t.Fatalf("rollback identities = runs:%v roots:%v", runIDs, rootIDs)
	}
	runIDs[0], rootIDs[0] = "run_changed", "member_changed"
	if err := rollback.Validate(); err != nil || rollback.DropRunIDs()[0] != "run_1" || rollback.CheckpointRootIDs()[0] != "member_1" {
		t.Fatalf("returned identities mutated rollback: %v", err)
	}

	unknown, err := NewRollbackPlan("ses_1", transcript.Boundary{
		KeepMessageMark: rundomain.UnknownMessageMark,
		Dropped:         []transcript.RunNode{{ID: "run_1"}},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mark, known := unknown.TruncationMark(); mark != rundomain.UnknownMessageMark || known {
		t.Fatalf("unknown coordinate = %d, %t", mark, known)
	}
}

func TestRollbackPlanRejectsInvalidWriteSets(t *testing.T) {
	validBoundary := transcript.Boundary{
		KeepMessageMark: 0,
		Dropped:         []transcript.RunNode{{ID: "run_1"}},
	}
	tests := []struct {
		name        string
		sessionID   string
		boundary    transcript.Boundary
		checkpoints []string
		replacement *plan.Replacement
	}{
		{name: "session", sessionID: "", boundary: validBoundary},
		{name: "message mark", sessionID: "ses_1", boundary: transcript.Boundary{KeepMessageMark: -2, Dropped: validBoundary.Dropped}},
		{name: "no dropped Runs", sessionID: "ses_1", boundary: transcript.Boundary{}},
		{name: "invalid Run", sessionID: "ses_1", boundary: transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run bad"}}}},
		{name: "repeated Run", sessionID: "ses_1", boundary: transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run_1"}, {ID: "run_1"}}}},
		{name: "invalid checkpoint", sessionID: "ses_1", boundary: validBoundary, checkpoints: []string{""}},
		{name: "repeated checkpoint", sessionID: "ses_1", boundary: validBoundary, checkpoints: []string{"member_1", "member_1"}},
		{name: "invalid Plan replacement", sessionID: "ses_1", boundary: validBoundary, replacement: &plan.Replacement{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRollbackPlan(test.sessionID, test.boundary, test.checkpoints, test.replacement); err == nil {
				t.Fatal("NewRollbackPlan accepted an invalid write-set")
			}
		})
	}
	if err := (RollbackPlan{}).Validate(); err == nil {
		t.Fatal("zero RollbackPlan is valid")
	}
}
