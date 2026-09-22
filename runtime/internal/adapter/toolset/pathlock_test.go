package toolset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestPathLockUsesOneCanonicalMutationIdentity(t *testing.T) {
	cwd := t.TempDir()
	realPath := filepath.Join(cwd, "real.txt")
	if err := os.WriteFile(realPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "other.txt"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}

	executor := mustLocalExecutor(t, cwd)
	read := mustReadTool(t, executor)
	readPaths, err := resolvedMutationPaths(read, mustTestInvocation(t, read, readArguments(realPath)), mustRoot(t, cwd))
	if err != nil {
		t.Fatal(err)
	}
	mutation := withApplyPatchMutationPaths(mustApplyPatchTool(t, executor))
	mutationPaths, err := resolvedMutationPaths(
		mutation,
		mustTestInvocation(t, mutation, patchArguments(t, "real.txt", "content", "next")), mustRoot(t, cwd),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(readPaths) != 1 || len(mutationPaths) != 1 || readPaths[0] != mutationPaths[0] {
		t.Fatalf("same-file identities = %v, %v; want one canonical path", readPaths, mutationPaths)
	}
}

func TestPathLockUsesPhysicalIdentityForSymlinkAlias(t *testing.T) {
	cwd := t.TempDir()
	realPath := filepath.Join(cwd, "real.txt")
	if err := os.WriteFile(realPath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(cwd, "alias.txt")
	if err := os.Symlink("real.txt", aliasPath); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	executor := mustLocalExecutor(t, cwd)
	read := mustReadTool(t, executor)
	realPaths, err := resolvedMutationPaths(read, mustTestInvocation(t, read, readArguments(realPath)), mustRoot(t, cwd))
	if err != nil {
		t.Fatal(err)
	}
	mutation := withApplyPatchMutationPaths(mustApplyPatchTool(t, executor))
	aliasPaths, err := resolvedMutationPaths(
		mutation,
		mustTestInvocation(t, mutation, patchArguments(t, "alias.txt", "content", "next")), mustRoot(t, cwd),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(realPaths) != 1 || len(aliasPaths) != 1 || realPaths[0] != aliasPaths[0] {
		t.Fatalf("symlink alias identities = %v, %v; want one physical path", realPaths, aliasPaths)
	}
}

// TestAssembledMutationToolsStillReportWhatTheyMutate pins the wrapping chain
// through the real stack. A guarded mutation tool has several decorators, and
// everything above it asks the OUTERMOST tool what the call will touch — the
// approval gate renders that blast radius, and locks reserve every prospective target. A layer that stops being a WrappingTool ends the chain and
// silently answers "nothing", which reads as a safe tool rather than a broken
// lookup.
//
// Both mutation tools are pinned because they learn the answer differently:
// apply_patch declares it from the patch text, edit leaves it to the path
// argument. The guards must not care which.
func TestAssembledMutationToolsStillReportWhatTheyMutate(t *testing.T) {
	cwd := t.TempDir()
	target := filepath.Join(cwd, "real.txt")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The composition the resolver exposes, not a hand-assembled stand-in.
	tools, err := openCWDTools(cwd, nil, newReadTracker(), keylock.NewSet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tools.close(); err != nil {
			t.Error(err)
		}
	})

	for _, testCase := range []struct {
		name      string
		mutation  toolcontract.Tool
		arguments string
	}{
		{"apply_patch", tools.applyPatch, patchArguments(t, "real.txt", "content", "next")},
		{"edit", tools.edit, editArguments(t, "real.txt", "content", "next")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			paths, err := mutationPaths(testCase.mutation, mustTestInvocation(t, testCase.mutation, testCase.arguments))
			if err != nil {
				t.Fatalf("mutationPaths: %v", err)
			}
			if len(paths) != 1 || filepath.Base(paths[0]) != "real.txt" {
				t.Fatalf("mutationPaths = %v, want the file the call changes", paths)
			}
		})
	}
}

func editArguments(t *testing.T, path, oldString, newString string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"path": path, "old_string": oldString, "new_string": newString,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func readArguments(path string) string {
	encoded, _ := json.Marshal(map[string]string{"path": path})
	return string(encoded)
}
