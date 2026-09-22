package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

// protectedDirs are directory names the agent must never modify, even when
// they sit inside the workspace. Mutating .git (a hook, the
// config) is a remote-code-execution / repo-hijack vector, so the VCS
// metadata directory is carved read-only regardless of approval mode — the
// standard invariant enforced on writable roots. A model that needs
// to change version-control state uses the shell/git tooling, not a raw
// file mutation. Kept as a list so other state dirs can join if needed.
var protectedDirs = []string{".git"}

// withPathGuard wraps a file-mutating tool so a target whose
// resolved path lies inside a [protectedDirs] directory is refused with a
// model-facing Scope rejection instead of executed. A definite refusal keeps
// the model able to adapt without publishing successful execution or unknown
// effects. Resolution uses the canonical physical path, so a
// traversal or symlink that lands in a protected directory is caught too. Apply it as
// the OUTERMOST wrap so the check gates before any staleness/diagnostics work.
//
// It also translates the filesystem capability's immutable workspace boundary
// into a model-facing refusal before any staleness or diagnostics work begins.
// Shell/git capabilities remain the explicit route for operations outside the
// workspace.
func withPathGuard(inner toolcontract.Tool, root *filesystemRoot) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			return chat.ToolOutput{}, fmt.Errorf("inspect mutation paths: %w", err)
		}
		for _, path := range paths {
			if refusal, ok := guardMutationPath(root, path); !ok {
				failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
					Kind: toolcontract.FailureKindRejected, Output: chat.NewTextToolOutput(refusal),
				})
				if err != nil {
					return chat.ToolOutput{}, err
				}
				return chat.ToolOutput{}, failure
			}
		}
		return inner.Call(ctx, invocation)
	})
}

// guardMutationPath checks protected directories through the pinned root.
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

// protectedDirHit returns the [protectedDirs] name when abs lies inside one
// (any path component matches), else "". Walks up via filepath.Base/Dir so
// it is separator-agnostic and matches a protected dir at any depth (a
// nested repo's .git included).
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
