package toolset

import (
	"context"

	"github.com/Tangerg/scope/tools/fs"
)

type mutationRecorderKey struct{}

// WithMutationRecorder observes acknowledged filesystem effects. A Tool's
// potential targets are approval/locking inputs, not proof of a mutation.
func WithMutationRecorder(ctx context.Context, record func([]string)) context.Context {
	if record == nil {
		return ctx
	}
	return context.WithValue(ctx, mutationRecorderKey{}, record)
}

type recordingExecutor struct{ *fs.LocalExecutor }

func (e recordingExecutor) Edit(ctx context.Context, request fs.EditRequest) (fs.EditResponse, error) {
	response, err := e.LocalExecutor.Edit(ctx, request)
	if err == nil {
		recordMutations(ctx, []string{request.Path})
	}
	return response, err
}

func (e recordingExecutor) ApplyPatch(ctx context.Context, request fs.ApplyPatchRequest) (fs.ApplyPatchResponse, error) {
	response, err := e.LocalExecutor.ApplyPatch(ctx, request)
	paths := make([]string, 0, len(response.Files)*2)
	for _, file := range response.Files {
		paths = append(paths, file.Path, file.MovedFrom)
	}
	// Scope reports acknowledged partial effects even when a later commit fails.
	recordMutations(ctx, cleanPathList(paths))
	return response, err
}

func recordMutations(ctx context.Context, paths []string) {
	if record, ok := ctx.Value(mutationRecorderKey{}).(func([]string)); ok && len(paths) > 0 {
		record(paths)
	}
}
