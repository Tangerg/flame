package sessions

import (
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
			err = coordinator.retireCommittedResources(t.Context(), requested.Retirement())
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
