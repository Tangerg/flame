package ownership

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

func TestGateExcludesOverlappingRunAndMutationScopes(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{root, child}, {child, root}, {child, alias}} {
		for _, phase := range []string{"pending", "live", "maintenance"} {
			t.Run(phase+"/"+pair[0]+"/"+pair[1], func(t *testing.T) {
				gate, err := NewGate(filesystem.CanonicalPath)
				if err != nil {
					t.Fatal(err)
				}
				admission, ok, err := gate.AcquireRun(t.Context(), "ses_run", pair[0])
				if err != nil || !ok {
					t.Fatalf("Run admission = %t, %v", ok, err)
				}
				release := admission.Release
				if phase != "pending" {
					if !admission.Admit("run_active") {
						t.Fatal("Run did not become live")
					}
					release = func() {
						finish, ok := gate.BeginMaintenance("run_active")
						if !ok {
							t.Fatal("Run did not enter maintenance")
						}
						finish()
					}
					if phase == "maintenance" {
						release, ok = gate.BeginMaintenance("run_active")
						if !ok {
							t.Fatal("Run did not enter maintenance")
						}
					}
				}
				if _, ok, err := gate.AcquireWorkingTreeMutation(pair[1]); err != nil || ok {
					t.Fatalf("overlapping mutation = %t, %v", ok, err)
				}
				release()
				finish, ok, err := gate.AcquireWorkingTreeMutation(pair[1])
				if err != nil || !ok {
					t.Fatalf("mutation after release = %t, %v", ok, err)
				}
				finish()
			})
		}
	}
}

func TestGateMutationFencesIntersectingAdmissionsOnly(t *testing.T) {
	for _, pair := range [][2]string{{"/repo", "/repo/a"}, {"/repo/a", "/repo"}} {
		gate := newTestGate(t)
		release, ok, err := gate.AcquireWorkingTreeMutation(pair[0])
		if err != nil || !ok {
			t.Fatalf("mutation admission = %t, %v", ok, err)
		}
		if _, ok, err := gate.AcquireRun(t.Context(), "ses_child", pair[1]); err != nil || ok {
			t.Fatalf("overlapping Run admission = %t, %v", ok, err)
		}
		if _, ok, err := gate.AcquireWorkingTreeMutation(pair[1]); err != nil || ok {
			t.Fatalf("overlapping mutation = %t, %v", ok, err)
		}
		release()
	}
	gate := newTestGate(t)
	release, _, err := gate.AcquireWorkingTreeMutation("/repo/a")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, path := range []string{"/repo/ab", "/repo/b"} {
		admission, ok, err := gate.AcquireRun(t.Context(), "ses_independent", path)
		if err != nil || !ok {
			t.Fatalf("disjoint Run at %s = %t, %v", path, ok, err)
		}
		admission.Release()
		finish, ok, err := gate.AcquireWorkingTreeMutation(path)
		if err != nil || !ok {
			t.Fatalf("disjoint mutation at %s = %t, %v", path, ok, err)
		}
		finish()
	}
}

func TestGateStartabilityWaitsForOverlappingMutation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := newTestGate(t)
		release, _, err := gate.AcquireWorkingTreeMutation("/repo")
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- gate.WaitRunStartable(t.Context(), "ses_child", "/repo/a") }()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("wait crossed an overlapping mutation: %v", err)
		default:
		}
		release()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
