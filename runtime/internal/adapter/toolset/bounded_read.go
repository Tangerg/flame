package toolset

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
	"github.com/samber/lo"
)

const (
	maxRuntimeReadFileBytes   int64 = 8 << 20
	maxRuntimeReadOutputBytes       = 1 << 20
	maxRuntimeReadLineBytes         = 1 << 20
)

var errRuntimeReadFileTooLarge = fmt.Errorf(
	"toolset: read file exceeds the %d MiB limit", maxRuntimeReadFileBytes/filesystem.MiB)

type runtimeReadExecutor struct {
	next fs.Reader
}

func newRuntimeReadTool(reader fs.Reader) (*fs.ReadTool, error) {
	if lo.IsNil(reader) {
		return nil, errors.New("toolset: read executor is required")
	}
	tool, err := fs.NewReadTool(runtimeReadExecutor{next: reader})
	if err != nil {
		return nil, fmt.Errorf("toolset: construct read tool: %w", err)
	}
	return tool, nil
}

// Only wrap a tool that mutates nothing: an unclassified failure leaves the Host
// unable to prove the operation did not happen, so it settles the Run tree as
// lost. Wrap outermost.
func withDefiniteOutcome(inner toolcontract.Tool) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, next toolcontract.Caller, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		output, err := next.Call(ctx, invocation)
		if err == nil {
			return output, nil
		}
		if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
			return output, errors.Join(err, cause)
		}
		return output, pluginsdk.DefiniteToolFailure(err)
	})
}

func (r runtimeReadExecutor) Read(ctx context.Context, input fs.ReadInput) (fs.ReadOutput, error) {
	if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
		return fs.ReadOutput{}, cause
	}
	input.MaxInputBytes = maxRuntimeReadFileBytes
	input.MaxLineBytes = maxRuntimeReadLineBytes
	input.MaxOutputBytes = maxRuntimeReadOutputBytes
	result, err := r.next.Read(ctx, input)
	err = errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx)))
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrFileTooLarge):
			return fs.ReadOutput{}, fmt.Errorf("%w: %w", errRuntimeReadFileTooLarge, err)
		case errors.Is(err, fs.ErrLineTooLarge):
			return fs.ReadOutput{}, fmt.Errorf(
				"toolset: read %s: line %d exceeds the %d MiB limit: %w", input.Path, fs.LineNumber(err), maxRuntimeReadLineBytes/filesystem.MiB, err,
			)
		default:
			return fs.ReadOutput{}, err
		}
	}
	return result, nil
}
