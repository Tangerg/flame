//go:build unix

package toolset

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestMutationRefusesUnsupportedFileModeBeforeDispatch(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "target"), 0o600); err != nil {
		t.Fatal(err)
	}
	guarded, called := observedGuardedEdit(t, dir)
	_, err := callTextTool(t.Context(), guarded, editArguments(t, "target", "old", "new"))
	if failure, known := toolcontract.ClassifyFailure(err); !known || failure.Kind() != toolcontract.FailureKindFailed || *called != 0 {
		t.Fatalf("unsupported mode: known=%t called=%d error=%v", known, *called, err)
	}
}

func TestMutationFingerprintAdmissionFollowsFileReadAuthority(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.WriteFile(path, []byte("old\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	_, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrPermission) {
		t.Fatal(readErr)
	}
	guarded, called := observedGuardedEdit(t, dir)
	_, err := callTextTool(t.Context(), guarded, editArguments(t, "target", "old", "new"))
	want := toolcontract.FailureKindRejected
	if readErr != nil {
		want = toolcontract.FailureKindFailed
	}
	failure, known := toolcontract.ClassifyFailure(err)
	if !known || failure.Kind() != want || *called != 0 {
		t.Fatalf("fingerprint admission: known=%t called=%d error=%v", known, *called, err)
	}
	if readErr != nil {
		if !errors.Is(failure.Cause(), os.ErrPermission) {
			t.Fatalf("lost fingerprint read cause: %v", err)
		}
		t.Log("OS read refusal retained as the fingerprint admission cause")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "old\n" {
		t.Fatalf("refused file changed: %q, %v", content, err)
	}
}
