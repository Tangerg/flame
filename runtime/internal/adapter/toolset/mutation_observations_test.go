package toolset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

func TestMutationCannotAdvanceReadEvidence(t *testing.T) {
	for _, name := range []string{"edit", "apply_patch"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "notes.txt")
			if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			root := mustRoot(t, dir)
			executor := mustLocalExecutor(t, dir)
			tracker := newReadTracker()
			read := withReadTracking(mustRuntimeReadTool(t, executor), tracker, root)
			base := toolcontract.Tool(mustApplyPatchTool(t, recordingExecutor{LocalExecutor: executor}))
			arguments := patchArguments(t, "notes.txt", "before\n", "applied\n")
			next := patchArguments(t, "notes.txt", "external\n", "overwritten\n")
			if name == "edit" {
				edit, err := fs.NewEditTool(recordingExecutor{LocalExecutor: executor})
				if err != nil {
					t.Fatal(err)
				}
				base = edit
				arguments = editArguments(t, "notes.txt", "before", "applied")
				next = editArguments(t, "notes.txt", "external", "overwritten")
			}
			interleaved := decorateCall(base, func(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
				output, err := base.Call(ctx, invocation)
				if err == nil {
					err = os.WriteFile(path, []byte("external\n"), 0o600)
				}
				return output, err
			})
			mutation := guardedMutation(interleaved, nil, tracker, keylock.NewSet(), root, executor)
			if _, err := callTextTool(t.Context(), read, readArguments("notes.txt")); err != nil {
				t.Fatal(err)
			}
			if _, err := callTextTool(t.Context(), mutation, arguments); err != nil {
				t.Fatalf("acknowledged mutation: %v", err)
			}
			refusal := callRejectedTool(t, t.Context(), mutation, next)
			if !strings.Contains(refusal, "must read") {
				t.Fatalf("unobserved external contents were authorized: %q", refusal)
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != "external\n" {
				t.Fatalf("external content = %q, %v", content, err)
			}
		})
	}
}

func TestMutationPreservesAcknowledgedOutputAfterCancellation(t *testing.T) {
	dir := t.TempDir()
	root := mustRoot(t, dir)
	executor := mustLocalExecutor(t, dir)
	base := mustApplyPatchTool(t, recordingExecutor{LocalExecutor: executor})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	completed := decorateCall(base, func(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		output, err := base.Call(ctx, invocation)
		if err == nil {
			cancel()
		}
		return output, err
	})
	mutation := guardedMutation(completed, nil, newReadTracker(), keylock.NewSet(), root, executor)
	output, err := callTextTool(ctx, mutation, patchArguments(t, "created.txt", "", "committed\n"))
	if err != nil || output == "" {
		t.Fatalf("committed output was lost to a later observation: %q, %v", output, err)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "created.txt")); err != nil || string(content) != "committed\n" {
		t.Fatalf("committed content = %q, %v", content, err)
	}
}

func TestUnresolvedMutationConsumesReadEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := mustRoot(t, dir)
	executor := mustLocalExecutor(t, dir)
	tracker := newReadTracker()
	read := withReadTracking(mustRuntimeReadTool(t, executor), tracker, root)
	base := mustApplyPatchTool(t, recordingExecutor{LocalExecutor: executor})
	cause := errors.New("commit acknowledgement unavailable")
	unresolved := decorateCall(base, func(context.Context, toolcontract.Invocation) (chat.ToolOutput, error) {
		return chat.ToolOutput{}, cause
	})
	mutation := guardedMutation(unresolved, nil, tracker, keylock.NewSet(), root, executor)
	if _, err := callTextTool(t.Context(), read, readArguments("notes.txt")); err != nil {
		t.Fatal(err)
	}
	arguments := patchArguments(t, "notes.txt", "before\n", "after\n")
	if _, err := callTextTool(t.Context(), mutation, arguments); !errors.Is(err, cause) {
		t.Fatalf("unresolved cause = %v", err)
	}
	refusal := callRejectedTool(t, t.Context(), guardedMutation(base, nil, tracker, keylock.NewSet(), root, executor), arguments)
	if !strings.Contains(refusal, "must read") {
		t.Fatalf("unresolved mutation retained authorization: %q", refusal)
	}
}
