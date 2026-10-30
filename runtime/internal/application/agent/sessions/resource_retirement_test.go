package sessions

import (
	"context"
	"errors"
	"testing"
)

func TestResourceRetirementRequiresItsExactCommittedAuthority(t *testing.T) {
	for _, authority := range []string{"absent", "different generation"} {
		t.Run(authority, func(t *testing.T) {
			stores := newMutationStores("")
			requested, err := NewDeletePlan("ses_1")
			if err != nil {
				t.Fatal(err)
			}
			if authority == "different generation" {
				other, err := NewDeletePlan("ses_1")
				if err != nil {
					t.Fatal(err)
				}
				stores.retirements["ses_1"] = other.Retirement()
			}
			coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
				ExecutionReleaser: mutationExecutions{operations: &stores.operations}, Paths: testWorkspaceResolver{},
				Checkpoints: &mutationCheckpoints{operations: &stores.operations}, Sandbox: &mutationSandbox{operations: &stores.operations},
			}))
			err = coordinator.retirer.complete(t.Context(), requested.Retirement())
			if authority == "absent" && err != nil {
				t.Fatal(err)
			}
			if authority == "different generation" && !errors.Is(err, ErrSessionBusy) {
				t.Fatalf("generation mismatch = %v", err)
			}
			if len(stores.operations) != 0 {
				t.Fatalf("uncommitted authority destroyed resources: %v", stores.operations)
			}
		})
	}
}

type ledgerFake struct{ rows map[string]ResourceRetirement }

func (l *ledgerFake) Lookup(_ context.Context, sessionID string) (ResourceRetirement, bool, error) {
	row, found := l.rows[sessionID]
	return row, found, nil
}

func (l *ledgerFake) ListPending(context.Context) ([]ResourceRetirement, error) {
	var rows []ResourceRetirement
	for _, row := range l.rows {
		rows = append(rows, row)
	}
	return rows, nil
}

func (l *ledgerFake) Complete(_ context.Context, retirement ResourceRetirement) error {
	delete(l.rows, retirement.SessionID())
	return nil
}

type claimsFake struct{}

func (claimsFake) AcquireRecoverySession(string) (func(), bool, error) { return func() {}, true, nil }

func TestStartupRecoveryKeepsOnlyTheFailingSessionFenced(t *testing.T) {
	ledger := &ledgerFake{rows: map[string]ResourceRetirement{}}
	for _, sessionID := range []string{"ses_broken", "ses_healthy"} {
		retirement, err := newResourceRetirement(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		ledger.rows[sessionID] = retirement
	}
	refusal := errors.New("sandbox directory is not removable")
	retirer, err := NewResourceRetirer(ledger, Cleanup{Resource: "sandbox", Retire: func(sessionID string) error {
		if sessionID == "ses_broken" {
			return refusal
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := retirer.RecoverPending(t.Context(), claimsFake{}); err != nil {
		t.Fatalf("one Session's cleanup failure stopped startup: %v", err)
	}
	if _, found := ledger.rows["ses_healthy"]; found {
		t.Fatal("a healthy Session stayed fenced behind another Session's failure")
	}
	if _, found := ledger.rows["ses_broken"]; !found {
		t.Fatal("a failed cleanup released its fence")
	}
}
