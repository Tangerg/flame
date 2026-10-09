package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestActionAdmissionRechecksWithdrawalAfterResourceVerification(t *testing.T) {
	for _, state := range []string{"available", "unavailable", "withdrawn"} {
		t.Run(state, func(t *testing.T) {
			release := testsupport.Release(t, "1", plugin.Declaration{Name: "actions", Actions: []plugin.Action{{ID: "rename", Title: "Rename", Operation: plugin.RenameSession}}})
			installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
			if err != nil {
				t.Fatal(err)
			}
			if err = installation.Approve(release); err != nil {
				t.Fatal(err)
			}
			if err = installation.Enable(release); err != nil {
				t.Fatal(err)
			}
			catalog := releaseMemory{release.Digest(): release}
			store := &installationMemory{catalog: catalog}
			if err = store.Save(t.Context(), installation); err != nil {
				t.Fatal(err)
			}
			dependencies := &installationDependencies{}
			var coordinator *Coordinator
			packages := installationPackages{realize: func(ctx context.Context, snapshot *plugin.Installation) (Realization, error) {
				if dependencies.held {
					t.Fatal("filesystem verification retained installation admission")
				}
				if state == "withdrawn" && snapshot.Active() {
					if _, err := coordinator.Disable(ctx, installation.ID()); err != nil {
						return Realization{}, err
					}
				}
				availability := ReleaseAvailable
				if state == "unavailable" {
					availability = ReleaseUnavailable
				}
				return Realization{Release: availability}, nil
			}}
			coordinator, err = New(t.Context(), admittedInstallationStore{store, dependencies}, catalog, packages, installationConnections{reconcile: func(context.Context, []mcpserver.ID) error { return nil }}, dependencies, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = coordinator.AuthorizeAction(t.Context(), installation.ID(), release.Digest(), "rename", plugin.RenameSession)
			var want error
			if state == "withdrawn" {
				want = plugin.ErrUnapproved
			}
			if state == "unavailable" {
				want = plugin.ErrUnavailable
			}
			if !errors.Is(err, want) {
				t.Fatalf("admission: %v, want %v", err, want)
			}
			if dependencies.held {
				t.Fatal("accepted command retained installation admission")
			}
		})
	}
}
