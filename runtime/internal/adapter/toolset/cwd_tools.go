package toolset

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/codeintel"
	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

// openCWDTools acquires the working-directory-bound filesystem capabilities,
// all anchored at cwd. Each resolution owns a Scope directory authority until
// its manifest closes after execution drains. The filesystem tools need no
// credentials. The shell family is built over shared exec.Shells in shell.Build
// and reads cwd per call.
//
// Every mutation is wrapped so a successful change is type-checked by the
// code-intelligence analyzer and any new problems are folded into the tool
// result. ci may be nil — the wrap is then a no-op.
// locker is owner-scoped: resolver-owned builds reuse one locker so
// read/check/mutate stays atomic across concurrent Runs, not merely across the
// tools resolved for one Run.
//
// edit and apply_patch are one mutation vocabulary under one guard stack. They
// differ only in how a call names what it changes: edit carries the path as an
// argument, while a patch has to be parsed to learn it. Both end at the same
// read-before-write stamp, path lock, protected-directory refusal, formatter,
// and diagnostics.
type cwdTools struct {
	readSearch []toolcontract.Tool
	edit       toolcontract.Tool
	applyPatch toolcontract.Tool
	close      func() error
}

func openCWDTools(cwd string, ci *codeintel.Analyzer, tracker *readTracker, locker *keylock.Set) (_ cwdTools, err error) {
	root, fsExec, close, err := openFilesystem(cwd)
	if err != nil {
		return cwdTools{}, fmt.Errorf("toolset: construct filesystem executor: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, close())
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

	// Guard stack, innermost → outermost: auto-format the applied
	// change; diagnostics type-check it; read/staleness guard gates before the
	// change and refreshes the read stamp after; per-path lock serializes
	// concurrent mutations to the same file; path guard refuses protected dirs.
	// apply_patch declares its own paths because they are inside the patch text;
	// edit needs no declaration, since the guards read its path argument.
	applyPatch := guardedMutation(withApplyPatchMutationPaths(applyPatchTool), ci, tracker, locker, root, fsExec)
	edit := guardedMutation(editTool, ci, tracker, locker, root, fsExec)

	families := cwdTools{
		readSearch: []toolcontract.Tool{
			withDefiniteOutcome(withPathLock(withReadTracking(readTool, tracker, root), locker, root)),
			withWorkspacePath(searchTools.glob, root),
			withWorkspacePath(searchTools.grep, root),
		},
		edit:       edit,
		applyPatch: applyPatch,
		close:      close,
	}
	return families, nil
}

func guardedMutation(tool toolcontract.Tool, ci *codeintel.Analyzer, tracker *readTracker, locker *keylock.Set, root *filesystemRoot, executor *fs.LocalExecutor) toolcontract.Tool {
	return withPathGuard(
		withPathLock(
			withMutationGuard(
				withMutationDiagnostics(withAutoFormat(tool, root, executor), ci, root),
				tracker, root,
			), locker, root,
		), root,
	)
}
