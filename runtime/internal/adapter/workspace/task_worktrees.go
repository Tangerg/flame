package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
	toolshell "github.com/Tangerg/scope/tools/shell"
)

const (
	taskRecordSuffix     = ".json"
	taskPendingSuffix    = ".next"
	taskIDBytes          = 16
	taskSetupTimeout     = 90 * time.Second
	taskSetupOutputLimit = 4096
)

type TaskWorktrees struct {
	root       string
	toolDirs   []string
	creatingMu sync.RWMutex
	creating   map[string]struct{}
}

func NewTaskWorktrees(root string, toolDirs []string) (*TaskWorktrees, error) {
	canonical, err := filesystem.CanonicalPath(root)
	if err != nil {
		return nil, err
	}
	return &TaskWorktrees{root: canonical, toolDirs: toolDirs, creating: make(map[string]struct{})}, nil
}

type taskRecord struct {
	ID        string                  `json:"-"`
	Source    string                  `json:"source"`
	CommonDir string                  `json:"common"`
	Base      string                  `json:"base"`
	Setup     *work.TaskWorktreeSetup `json:"setup,omitzero"`
}

type taskCheckout struct {
	path   string
	record taskRecord
}

func taskRecordName(id string) string { return id + taskRecordSuffix }

func (b *TaskWorktrees) checkoutPath(id string) string { return filepath.Join(b.root, id) }

func (b *TaskWorktrees) CheckoutPath(id string) (string, error) {
	if !isTaskID(id) {
		return "", work.ErrTaskWorktreeNotFound
	}
	return b.checkoutPath(id), nil
}

func (b *TaskWorktrees) PrepareCreate(ctx context.Context, path, base, setup string) (func(context.Context) (work.TaskWorktree, error), error) {
	source, err := taskGitValue(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, errors.Join(work.ErrVCSUnavailable, err)
	}
	source, err = (filesystem.DirectoryResolver{}).ResolveExistingDir(source)
	if err != nil {
		return nil, err
	}
	if _, inside := relativeInTree(source, b.root); inside {
		return nil, fmt.Errorf("%w: task storage must be outside the source checkout", work.ErrTaskWorktreeInput)
	}
	common, err := taskCommonDir(ctx, source)
	if err != nil {
		return nil, err
	}
	commit, err := taskGitValue(ctx, source, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return nil, errors.Join(work.ErrTaskWorktreeInput, err)
	}
	var key [taskIDBytes]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	record := taskRecord{ID: hex.EncodeToString(key[:]), Source: source, CommonDir: common, Base: commit}
	if setup != "" {
		record.Setup = &work.TaskWorktreeSetup{Command: setup}
	}
	return func(registration context.Context) (work.TaskWorktree, error) {
		// Setup remains cancellable after the caller has authorized registration.
		return b.registerTask(registration, ctx, record)
	}, nil
}

func (b *TaskWorktrees) registerTask(ctx, setupCtx context.Context, record taskRecord) (work.TaskWorktree, error) {
	if err := os.MkdirAll(b.root, filesystem.PrivateDirectoryMode); err != nil {
		return work.TaskWorktree{}, err
	}
	root, err := os.OpenRoot(b.root)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	defer root.Close()
	// The manifest must survive interruption. A live creation still owns the
	// checkout until its setup and registration can be published together.
	b.creatingMu.Lock()
	b.creating[record.ID] = struct{}{}
	b.creatingMu.Unlock()
	defer func() {
		b.creatingMu.Lock()
		delete(b.creating, record.ID)
		b.creatingMu.Unlock()
	}()
	checkout := b.checkoutPath(record.ID)
	// The source checkout can be released after its base has been captured.
	if _, err := taskGitValue(ctx, record.CommonDir, "worktree", "add", "--detach", "--quiet", "--", checkout, record.Base); err != nil {
		return work.TaskWorktree{}, errors.Join(work.ErrVCSUnavailable, err)
	}
	if err := writeTaskRecord(root, record); err != nil {
		return work.TaskWorktree{}, err
	}
	gitDir, err := taskGitValue(ctx, checkout, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return work.TaskWorktree{}, err
	}
	// Git removes the registration and its custody stamp together. A new
	// checkout can reuse both the working path and Git's registration name.
	if err := os.WriteFile(filepath.Join(gitDir, taskCustody), []byte(record.ID), filesystem.PrivateFileMode); err != nil {
		return work.TaskWorktree{}, fmt.Errorf("workspace: record task worktree custody: %w", err)
	}
	if record.Setup != nil {
		b.runSetup(setupCtx, checkout, record.Setup)
		if err := writeTaskRecord(root, record); err != nil {
			return work.TaskWorktree{}, err
		}
	}
	return (&taskCheckout{path: checkout, record: record}).Inspect(ctx)
}

func (b *TaskWorktrees) runSetup(ctx context.Context, checkout string, setup *work.TaskWorktreeSetup) {
	limited, cancel := context.WithTimeout(ctx, taskSetupTimeout)
	defer cancel()
	output, err := sandbox.RunInPlace(limited, sandbox.Config{RestrictReads: true, ReadOnlyPaths: b.toolDirs}, checkout, toolshell.Input{Cmd: setup.Command})
	log := capture.NewWriter(taskSetupOutputLimit)
	_, _ = log.Write(output.Stdout)
	_, _ = log.Write(output.Stderr)
	if err != nil {
		_, _ = log.Write([]byte(err.Error()))
	} else if !output.CancellationObserved {
		setup.ExitCode = new(output.ExitCode)
	}
	setup.Output = log.Diagnostic()
	if log.Truncated() {
		setup.Output += "\n... [setup output truncated] ..."
	}
}

func (b *TaskWorktrees) List(ctx context.Context) ([]work.TaskWorktree, error) {
	entries, err := os.ReadDir(b.root)
	if errors.Is(err, os.ErrNotExist) {
		return []work.TaskWorktree{}, nil
	}
	if err != nil {
		return nil, err
	}
	tasks := make([]work.TaskWorktree, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id, record := strings.CutSuffix(entry.Name(), taskRecordSuffix)
		if entry.IsDir() || !record {
			continue
		}
		checkout, err := b.OpenCheckout(id)
		if errors.Is(err, work.ErrTaskWorktreeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		task, err := checkout.Inspect(ctx)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (b *TaskWorktrees) OpenCheckout(id string) (work.TaskCheckout, error) {
	path, err := b.CheckoutPath(id)
	if err != nil {
		return nil, err
	}
	b.creatingMu.RLock()
	defer b.creatingMu.RUnlock()
	if _, creating := b.creating[id]; creating {
		return nil, work.ErrTaskWorktreeNotFound
	}
	root, err := os.OpenRoot(b.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, work.ErrTaskWorktreeNotFound
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(taskRecordName(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, work.ErrTaskWorktreeNotFound
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	record := taskRecord{ID: id}
	if err := json.UnmarshalRead(file, &record, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("workspace: decode task worktree record: %w", err)
	}
	if !filepath.IsAbs(record.Source) || !filepath.IsAbs(record.CommonDir) || !validTaskSHA(record.Base) {
		return nil, errors.New("workspace: invalid task worktree record")
	}
	return &taskCheckout{path: path, record: record}, nil
}

func (t *taskCheckout) SourcePath() string { return t.record.Source }

func (t *taskCheckout) Inspect(ctx context.Context) (work.TaskWorktree, error) {
	registration, err := t.checkoutRegistration(ctx)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	return t.view(registration), nil
}

func (t *taskCheckout) view(registration *taskRegistration) work.TaskWorktree {
	return work.TaskWorktree{
		ID: t.record.ID, Path: t.path, Source: t.record.Source, Base: t.record.Base,
		Setup: t.record.Setup, Available: registration != nil,
	}
}

func taskCommonDir(ctx context.Context, path string) (string, error) {
	common, err := taskGitValue(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return (filesystem.DirectoryResolver{}).ResolveExistingDir(common)
}

func taskGitValue(ctx context.Context, path string, args ...string) (string, error) {
	raw, err := taskGitBytes(ctx, path, args...)
	return strings.TrimSuffix(string(raw), "\n"), err
}

func taskGitBytes(ctx context.Context, path string, args ...string) ([]byte, error) {
	result, err := process.At(ctx, path, args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("workspace: git %s failed with exit %d: %s", args[0], result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
}

func writeTaskRecord(root *os.Root, record taskRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	name := taskRecordName(record.ID)
	pending := name + taskPendingSuffix
	file, err := root.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filesystem.PrivateFileMode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return errors.Join(err, root.Remove(pending))
	}
	if err := root.Rename(pending, name); err != nil {
		return errors.Join(err, root.Remove(pending))
	}
	entry, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(entry.Sync(), entry.Close())
}

func isTaskID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == taskIDBytes && value == strings.ToLower(value)
}

func validTaskSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && (len(decoded) == 20 || len(decoded) == 32)
}
