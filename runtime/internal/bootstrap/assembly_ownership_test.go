package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestWorkingTreeAdmissionUsesTheWorkspacePhysicalIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root := t.TempDir()
		alias := filepath.Join(t.TempDir(), "workspace")
		if err := os.Symlink(root, alias); err != nil {
			t.Fatal(err)
		}
		gate, err := ownership.NewGate(ownership.GateConfig{ResolveTree: filesystem.CanonicalPath, RetirementPending: testsupport.NoResourceRetirements})
		if err != nil {
			t.Fatal(err)
		}
		first, acquired, err := gate.AcquireRun(t.Context(), "ses_1", root)
		if !acquired || err != nil || !first.Admit("run_1") {
			t.Fatalf("first Run admission = %t, %v", acquired, err)
		}
		second, acquired, err := gate.AcquireRun(t.Context(), "ses_2", alias)
		if !acquired || err != nil {
			t.Fatalf("shared working-tree admission = %t, %v", acquired, err)
		}
		maintenance, ok := gate.BeginMaintenance("run_1")
		if !ok {
			t.Fatal("Run did not enter maintenance")
		}
		second.Release()
		if release, acquired, err := gate.AcquireWorkingTreeMutation(alias); release != nil || acquired || err != nil {
			t.Fatalf("alias crossed terminal maintenance: acquired=%t err=%v", acquired, err)
		}
		maintenance()
		release, acquired, err := gate.AcquireWorkingTreeMutation(alias)
		if !acquired || err != nil {
			t.Fatalf("mutation after maintenance = %t, %v", acquired, err)
		}
		defer release()
		if admission, acquired, err := gate.AcquireRun(t.Context(), "ses_3", root); acquired || err != nil {
			admission.Release()
			t.Fatalf("Run crossed aliased mutation = %t, %v", acquired, err)
		}
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		waiting := make(chan error, 1)
		go func() { waiting <- gate.WaitRunStartable(ctx, "ses_3", root) }()
		synctest.Wait()
		select {
		case err := <-waiting:
			t.Fatalf("startability ignored aliased mutation: %v", err)
		default:
		}
		cause := errors.New("waiting Run withdrawn")
		cancel(cause)
		if err := <-waiting; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatalf("startability cancellation = %v", err)
		}
		release()
		if err := gate.WaitRunStartable(t.Context(), "ses_3", root); err != nil {
			t.Fatal(err)
		}
		admission, acquired, err := gate.AcquireRun(t.Context(), "ses_3", root)
		admission.Release()
		if !acquired || err != nil {
			t.Fatalf("Run after aliased mutation = %t, %v", acquired, err)
		}
	})
}

func TestWorkingTreeIdentityFailureRemainsAnError(t *testing.T) {
	gate, err := ownership.NewGate(ownership.GateConfig{ResolveTree: filesystem.CanonicalPath, RetirementPending: testsupport.NoResourceRetirements})
	if err != nil {
		t.Fatal(err)
	}
	loop := filepath.Join(t.TempDir(), "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Fatal(err)
	}
	admission, acquired, err := gate.AcquireRun(t.Context(), "ses_1", loop)
	admission.Release()
	if acquired || err == nil {
		t.Fatalf("cyclic path acquisition = (%t, %v), want identity failure", acquired, err)
	}
}

func TestMissingWorkingTreeRetainsOnePhysicalAdmissionIdentity(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	gate, err := ownership.NewGate(ownership.GateConfig{ResolveTree: filesystem.CanonicalPath, RetirementPending: testsupport.NoResourceRetirements})
	if err != nil {
		t.Fatal(err)
	}
	admission, acquired, err := gate.AcquireRun(t.Context(), "ses_1", filepath.Join(root, "removed"))
	if !acquired || err != nil {
		t.Fatalf("missing working-tree admission = %t, %v", acquired, err)
	}
	defer admission.Release()
	if release, acquired, err := gate.AcquireWorkingTreeMutation(filepath.Join(alias, "removed")); release != nil || acquired || err != nil {
		t.Fatalf("missing alias crossed Run admission = %t, %v", acquired, err)
	}
}
