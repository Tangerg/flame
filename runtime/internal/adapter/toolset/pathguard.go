package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/infra/git"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

// Mutating Git metadata (a hook, the config) is a remote-code-execution vector,
// so it stays read-only regardless of approval mode.
var protectedDirs = []string{git.MetadataDirectory}

func withPathGuard(inner toolcontract.Tool, root *filesystemRoot) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, next toolcontract.Caller, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			err = fmt.Errorf("inspect mutation paths: %w", err)
			return toolcontract.Result{}, beforeCallFailure(ctx, err)
		}
		for _, path := range paths {
			if refusal, ok := guardMutationPath(root, path); !ok {
				failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
					Kind: toolcontract.FailureKindRejected, Output: toolcontract.NewTextOutput(refusal),
				})
				if err != nil {
					return toolcontract.Result{}, beforeCallFailure(ctx, err)
				}
				return toolcontract.Result{}, beforeCallFailure(ctx, failure)
			}
		}
		return next.Call(ctx, invocation)
	})
}

func guardMutationPath(root *filesystemRoot, path string) (refusal string, ok bool) {
	resolved, err := resolveRootPath(root, path)
	if errors.Is(err, fs.ErrPathOutsideRoot) {
		return fmt.Sprintf("Refused: %q is outside this workspace. Filesystem tools may only modify files inside their workspace root.", path), false
	}
	if err != nil {
		return fmt.Sprintf("Refused: %q could not be resolved safely (%v).", path, err), false
	}
	if dir := protectedDirHit(filepath.Join(root.identity, resolved)); dir != "" {
		return fmt.Sprintf("Refused: %q is inside the protected %q directory, which is read-only to the agent. Use the shell/git tooling if you need to change version-control state.", path, dir), false
	}
	return "", true
}

func protectedDirHit(abs string) string {
	for p := abs; ; {
		if base := filepath.Base(p); slices.Contains(protectedDirs, base) {
			return base
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
}
