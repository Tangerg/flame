package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestAcceptedViewReadDoesNotRetainAdmissionOrAuthorizeSubsequentReads(t *testing.T) {
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "page", Views: []plugin.ViewDeclaration{{ID: "trajectory", Title: "Trajectory", Kind: plugin.SessionTrajectory, Entry: "views/trajectory.html"}}})
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
	coordinator, err := New(t.Context(), admittedInstallationStore{store, dependencies}, catalog, installationPackages{}, installationConnections{reconcile: func(_ context.Context, _ []mcpserver.ID) error { return nil }}, dependencies, nil)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = coordinator.WithView(t.Context(), installation.ID(), release.Digest(), "trajectory", plugin.SessionTrajectory, func(snapshot *plugin.Installation, view plugin.ViewDeclaration) error {
		called = true
		if dependencies.held || !snapshot.Active() || view.ID != "trajectory" {
			t.Fatal("accepted read retained admission or lost its authorized snapshot")
		}
		_, err := coordinator.Disable(t.Context(), installation.ID())
		return err
	})
	if err != nil || !called {
		t.Fatalf("accepted read = %v, called = %t", err, called)
	}
	err = coordinator.WithView(t.Context(), installation.ID(), release.Digest(), "trajectory", plugin.SessionTrajectory, func(*plugin.Installation, plugin.ViewDeclaration) error {
		t.Fatal("withdrawn installation authorized another read")
		return nil
	})
	if !errors.Is(err, plugin.ErrUnapproved) {
		t.Fatalf("subsequent read = %v", err)
	}
}
