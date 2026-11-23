package toolset

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

type cwdTools struct {
	readSearch []toolcontract.Tool
	edit       toolcontract.Tool
	applyPatch toolcontract.Tool
	close      func() error
}

func openCWDTools(cwd string, ci CodeIntelligence, tracker *readTracker, locker *keylock.Set) (_ cwdTools, err error) {
	root, fsExec, release, err := openFilesystem(cwd)
	if err != nil {
		return cwdTools{}, fmt.Errorf("toolset: construct filesystem executor: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, release())
		}
	}()
	searchTools := newRuntimeSearchTools(cwd)
	readTool, err := newRuntimeReadTool(fsExec)
	if err != nil {
		return cwdTools{}, err
	}
	mutations := recordingExecutor{LocalExecutor: fsExec}
	applyPatchTool, err := fs.NewApplyPatchTool(mutations)
	if err != nil {
		return cwdTools{}, fmt.Errorf("toolset: construct apply-patch tool: %w", err)
	}
	editTool, err := fs.NewEditTool(mutations)
	if err != nil {
		return cwdTools{}, fmt.Errorf("toolset: construct edit tool: %w", err)
	}

	// Guard order matters, innermost to outermost: format, diagnostics,
	// read/staleness, per-path lock, path guard.
	applyPatch := guardedMutation(applyPatchTool, ci, tracker, locker, root, fsExec)
	edit := guardedMutation(editTool, ci, tracker, locker, root, fsExec)

	families := cwdTools{
		readSearch: []toolcontract.Tool{
			withDefiniteOutcome(withPathLock(withReadTracking(readTool, tracker, root), locker, root)),
			withWorkspacePath(searchTools.glob, root),
			withWorkspacePath(searchTools.grep, root),
		},
		edit:       edit,
		applyPatch: applyPatch,
		close:      release,
	}
	return families, nil
}

func guardedMutation(tool toolcontract.Tool, ci CodeIntelligence, tracker *readTracker, locker *keylock.Set, root *filesystemRoot, executor *fs.LocalExecutor) toolcontract.Tool {
	return withPathGuard(
		withPathLock(
			withMutationGuard(
				withCodeIntelligence(tool, ci, root, executor),
				tracker, root,
			), locker, root,
		), root,
	)
}
