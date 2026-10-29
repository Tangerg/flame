package toolset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

func observedGuardedEdit(t *testing.T, dir string) (toolcontract.Tool, *int) {
	t.Helper()
	executor := mustLocalExecutor(t, dir)
	base, err := fs.NewEditTool(recordingExecutor{LocalExecutor: executor})
	if err != nil {
		t.Fatal(err)
	}
	called := new(int)
	inner := decorateCall(base, func(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		(*called)++
		return base.Call(ctx, invocation)
	})
	guarded := guardedMutation(inner, nil, newReadTracker(), keylock.NewSet(), mustRoot(t, dir), executor)
	return guarded, called
}

func TestMutationDirectoryRefusalDoesNotCallInnerTool(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "target"), 0o700); err != nil {
		t.Fatal(err)
	}
	guarded, called := observedGuardedEdit(t, dir)
	_, err := callTextTool(t.Context(), guarded, editArguments(t, "target", "old", "new"))
	if failure, known := toolcontract.ClassifyFailure(err); !known || failure.Kind() != toolcontract.FailureKindFailed || *called != 0 {
		t.Fatalf("directory admission: failure=%v known=%t called=%d error=%v", failure, known, *called, err)
	}
}

func TestMutationDispatchUncertaintyIsNotAnAdmissionRefusal(t *testing.T) {
	dir := t.TempDir()
	executor := mustLocalExecutor(t, dir)
	base := mustApplyPatchTool(t, recordingExecutor{LocalExecutor: executor})
	lost := errors.New("write response lost")
	inner := decorateCall(base, func(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		result, err := base.Call(ctx, invocation)
		if err != nil {
			return result, err
		}
		return result, lost
	})
	guarded := guardedMutation(inner, nil, newReadTracker(), keylock.NewSet(), mustRoot(t, dir), executor)
	_, err := callTextTool(t.Context(), guarded, patchArguments(t, "created.txt", "", "written\n"))
	if _, known := toolcontract.ClassifyFailure(err); known || !errors.Is(err, lost) {
		t.Fatalf("dispatch uncertainty changed: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "created.txt")); err != nil || string(content) != "written\n" {
		t.Fatalf("actual dispatch: %q, %v", content, err)
	}
}

func TestMutationAdmissionPreservesOwnerCancellation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.txt"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	guarded, called := observedGuardedEdit(t, dir)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("owner retired")
	cancel(cause)
	_, err := callTextTool(ctx, guarded, editArguments(t, "target.txt", "old", "new"))
	if _, known := toolcontract.ClassifyFailure(err); known || *called != 0 || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("owner cancellation: known=%t called=%d error=%v", known, *called, err)
	}
}
