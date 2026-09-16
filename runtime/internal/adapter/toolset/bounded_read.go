package toolset

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/toolfailure"
)

const (
	maxRuntimeReadFileBytes   int64 = 8 << 20
	maxRuntimeReadOutputBytes       = 1 << 20
	maxRuntimeReadLineBytes         = 1 << 20
)

// The limit is spelled once, by the constant; the model reads this text and
// acts on the number in it.
var errRuntimeReadFileTooLarge = fmt.Errorf(
	"toolset: read file exceeds the %d MiB limit", maxRuntimeReadFileBytes>>20)

// runtimeReadExecutor declares Runtime's model-facing read envelope while the
// filesystem reader remains the sole owner of path authority and bounded I/O.
type runtimeReadExecutor struct {
	next fs.Reader
}

func newRuntimeReadTool(root string, reader fs.Reader) (*fs.ReadTool, error) {
	if reader == nil {
		executor, err := fs.NewLocalExecutor(root)
		if err != nil {
			return nil, fmt.Errorf("toolset: construct read executor: %w", err)
		}
		reader = executor
	}
	tool, err := fs.NewReadTool(runtimeReadExecutor{next: reader})
	if err != nil {
		return nil, fmt.Errorf("toolset: construct read tool: %w", err)
	}
	return tool, nil
}

// withDefiniteOutcome settles a failed call as this call's definite outcome.
// Only wrap a tool that mutates nothing: reading is the whole contract of the
// tools that use it, so a path the model got wrong, a file past the size cap, an
// over-long line, a stamp that says to read the file again, or a Skill that is
// not there is a failed call and nothing more — the same reason the local
// searches classify theirs. Unclassified, the Host cannot prove the operation
// did not happen and settles the Run tree as lost, which would let a mistyped
// path end the Session.
//
// Wrap outermost, so a guard stack in front of the tool is covered by one rule
// rather than one per layer.
func withDefiniteOutcome(inner toolcontract.Tool) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		output, err := inner.Call(ctx, invocation)
		if err == nil || ctx.Err() != nil {
			return output, err
		}
		return output, toolfailure.Definite(err)
	})
}

func (r runtimeReadExecutor) Read(ctx context.Context, input fs.ReadInput) (fs.ReadOutput, error) {
	if cause := context.Cause(ctx); cause != nil {
		return fs.ReadOutput{}, cause
	}
	input.MaxInputBytes = maxRuntimeReadFileBytes
	input.MaxLineBytes = maxRuntimeReadLineBytes
	input.MaxOutputBytes = maxRuntimeReadOutputBytes
	result, err := r.next.Read(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrFileTooLarge):
			return fs.ReadOutput{}, fmt.Errorf("%w: %w", errRuntimeReadFileTooLarge, err)
		case errors.Is(err, fs.ErrLineTooLarge):
			return fs.ReadOutput{}, fmt.Errorf(
				"toolset: read %s: line %d exceeds the 1 MiB limit", input.Path, fs.ReadLineNumber(err),
			)
		default:
			return fs.ReadOutput{}, err
		}
	}
	return result, nil
}
