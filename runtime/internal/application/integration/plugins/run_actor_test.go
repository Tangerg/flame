package plugins

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRunActorAdmissionDistinguishesWithdrawalFromUnknownStorageFailure(t *testing.T) {
	for _, change := range []string{"none", "disabled", "missing", "release", "authority", "storage"} {
		t.Run(change, func(t *testing.T) {
			store, _, id, _ := installationFixture(t)
			snapshot, _, err := store.Get(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			if err := snapshot.Installation.Enable(snapshot.Selected); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(t.Context(), snapshot.Installation); err != nil {
				t.Fatal(err)
			}
			release := snapshot.Selected.Digest()
			authority := snapshot.Installation.ServerAuthority(testsupport.ServerName("server"))
			storageFailure := errors.New("installation read failed")
			switch change {
			case "disabled":
				store.record.Enabled = false
			case "missing":
				store.record = nil
			case "release":
				release = testsupport.Digest("another release")
			case "authority":
				authority = testsupport.Digest("another configuration")
			case "storage":
				store.getErr = storageFailure
			}
			actor, err := operation.NewActor(id.String(), release.String(), authority.String(), "server")
			if err != nil {
				t.Fatal(err)
			}
			admission, err := NewRunActorAdmission(store)
			if err != nil {
				t.Fatal(err)
			}
			err = admission.Admit(t.Context(), actor)
			switch change {
			case "none":
				if err != nil {
					t.Fatal(err)
				}
			case "storage":
				if !errors.Is(err, storageFailure) || errors.Is(err, runs.ErrInstallationChanged) {
					t.Fatalf("unknown read failure became withdrawal: %v", err)
				}
			default:
				if !errors.Is(err, runs.ErrInstallationChanged) {
					t.Fatalf("known withdrawal was not classified: %v", err)
				}
			}
		})
	}
}
