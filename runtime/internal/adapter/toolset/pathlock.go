package toolset

import (
	"context"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Scope schedules one execution tree. These resolver-owned locks coordinate
// reads and mutations of the same physical path across concurrent Runs.
func withPathLock(inner toolcontract.Tool, locker *keylock.Set, root *filesystemRoot) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		paths, err := resolvedMutationPaths(inner, invocation, root)
		if err != nil {
			return chat.ToolOutput{}, err
		}
		for i, path := range paths {
			paths[i] = filepath.Join(root.identity, path)
		}
		release, err := locker.AcquireAll(ctx, paths...)
		if err != nil {
			return chat.ToolOutput{}, err
		}
		defer release()
		return inner.Call(ctx, invocation)
	})
}
