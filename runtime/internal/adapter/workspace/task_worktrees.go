package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
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
	taskManifest  = "task.json"
	taskCheckout  = "checkout"
	maxHandoff    = 512 << 10
	maxTaskRecord = 16 << 10
)

type TaskWorktrees struct {
	root     string
	toolDirs []string
	creating sync.RWMutex
}

func NewTaskWorktrees(root string, toolDirs []string) *TaskWorktrees {
	return &TaskWorktrees{root: root, toolDirs: toolDirs}
}

type taskRecord struct {
	ID     string                  `json:"id"`
	Source string                  `json:"source"`
	Common string                  `json:"common"`
	Base   string                  `json:"base"`
	Setup  *work.TaskWorktreeSetup `json:"setup,omitzero"`
}

func (b *TaskWorktrees) Create(ctx context.Context, path, base, setup string) (work.TaskWorktree, error) {
	b.creating.Lock()
	defer b.creating.Unlock()
	source, err := taskGitValue(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return work.TaskWorktree{}, errors.Join(work.ErrVCSUnavailable, err)
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	if relative, err := filepath.Rel(source, b.root); err == nil && filepath.IsLocal(relative) {
		return work.TaskWorktree{}, fmt.Errorf("%w: task storage must be outside the source checkout", work.ErrTaskWorktreeInput)
	}
	common, err := taskCommonDir(ctx, source)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	commit, err := taskGitValue(ctx, source, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return work.TaskWorktree{}, errors.Join(work.ErrTaskWorktreeInput, err)
	}
	if err := os.MkdirAll(b.root, filesystem.PrivateDirectoryMode); err != nil {
		return work.TaskWorktree{}, err
	}
	root, err := os.OpenRoot(b.root)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	defer root.Close()
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return work.TaskWorktree{}, err
	}
	record := taskRecord{ID: hex.EncodeToString(key[:]), Source: source, Common: common, Base: commit}
	if setup != "" {
		record.Setup = &work.TaskWorktreeSetup{Command: setup}
	}
	if err := root.Mkdir(record.ID, filesystem.PrivateDirectoryMode); err != nil {
		return work.TaskWorktree{}, err
	}
	if err := writeTaskRecord(root, record); err != nil {
		return work.TaskWorktree{}, err
	}
	checkout := filepath.Join(b.root, record.ID, taskCheckout)
	if _, err := taskGitValue(ctx, source, "worktree", "add", "--detach", "--quiet", "--", checkout, commit); err != nil {
		return work.TaskWorktree{}, err
	}
	if record.Setup != nil {
		limited, cancel := context.WithTimeout(ctx, 90*time.Second)
		output, runErr := sandbox.RunInPlace(limited, sandbox.Config{RestrictReads: true, ReadOnlyPaths: b.toolDirs}, checkout, toolshell.Input{Cmd: setup})
		cancel()
		log := capture.NewWriter(4096)
		_, _ = log.Write(output.Stdout)
		_, _ = log.Write(output.Stderr)
		if runErr != nil {
			_, _ = log.Write([]byte(runErr.Error()))
		} else if !output.CancellationObserved {
			record.Setup.ExitCode = new(output.ExitCode)
		}
		record.Setup.Output = log.Diagnostic()
		if log.Truncated() {
			record.Setup.Output += "\n... [setup output truncated] ..."
		}
		if err := writeTaskRecord(root, record); err != nil {
			return work.TaskWorktree{}, err
		}
	}
	return b.inspect(ctx, record)
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
		if !entry.IsDir() || !isTaskID(entry.Name()) {
			continue
		}
		task, err := b.Get(ctx, entry.Name())
		if errors.Is(err, work.ErrTaskWorktreeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (b *TaskWorktrees) Get(ctx context.Context, id string) (work.TaskWorktree, error) {
	b.creating.RLock()
	defer b.creating.RUnlock()
	record, err := b.load(id)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	return b.inspect(ctx, record)
}

func (b *TaskWorktrees) load(id string) (taskRecord, error) {
	if !isTaskID(id) {
		return taskRecord{}, work.ErrTaskWorktreeNotFound
	}
	root, err := os.OpenRoot(b.root)
	if errors.Is(err, os.ErrNotExist) {
		return taskRecord{}, work.ErrTaskWorktreeNotFound
	}
	if err != nil {
		return taskRecord{}, err
	}
	defer root.Close()
	file, err := root.Open(filepath.Join(id, taskManifest))
	if errors.Is(err, os.ErrNotExist) {
		return taskRecord{}, work.ErrTaskWorktreeNotFound
	}
	if err != nil {
		return taskRecord{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxTaskRecord+1))
	if err != nil {
		return taskRecord{}, err
	}
	var record taskRecord
	if len(raw) > maxTaskRecord || json.Unmarshal(raw, &record, json.RejectUnknownMembers(true)) != nil ||
		record.ID != id || !filepath.IsAbs(record.Source) || !filepath.IsAbs(record.Common) || !validTaskSHA(record.Base) {
		return taskRecord{}, errors.New("workspace: invalid task worktree record")
	}
	return record, nil
}

func (b *TaskWorktrees) inspect(ctx context.Context, record taskRecord) (work.TaskWorktree, error) {
	directory, err := filepath.EvalSymlinks(filepath.Join(b.root, record.ID))
	if err != nil {
		return work.TaskWorktree{}, err
	}
	checkout := filepath.Join(directory, taskCheckout)
	task := work.TaskWorktree{ID: record.ID, Path: checkout, Source: record.Source, Base: record.Base, Setup: record.Setup}
	info, err := os.Lstat(checkout)
	if errors.Is(err, os.ErrNotExist) {
		return task, nil
	}
	if err != nil {
		return work.TaskWorktree{}, err
	}
	if !info.IsDir() {
		return task, nil
	}
	top, err := taskGitValue(ctx, checkout, "rev-parse", "--show-toplevel")
	if err != nil {
		return work.TaskWorktree{}, err
	}
	current, err := taskCommonDir(ctx, checkout)
	if err != nil {
		return work.TaskWorktree{}, err
	}
	task.Available = current == record.Common && top == checkout
	return task, nil
}

func (b *TaskWorktrees) Preview(ctx context.Context, task work.TaskWorktree) (work.WorktreeHandoff, error) {
	branch, err := taskGitValue(ctx, task.Source, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return work.WorktreeHandoff{}, errors.Join(work.ErrTaskWorktreeChanged, err)
	}
	head, err := taskGitValue(ctx, task.Source, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if head != task.Base {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeChanged
	}
	record, err := b.load(task.ID)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	current, err := taskCommonDir(ctx, task.Source)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if current != record.Common {
		return work.WorktreeHandoff{}, work.ErrTaskWorktreeChanged
	}
	status, err := taskGitBytes(ctx, task.Source, 0, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	if len(status) > 0 {
		return work.WorktreeHandoff{}, fmt.Errorf("%w: destination has local changes", work.ErrTaskWorktreeChanged)
	}
	patch, err := taskPatch(ctx, task)
	if err != nil {
		return work.WorktreeHandoff{}, err
	}
	digest := sha256.New()
	fmt.Fprintf(digest, "%q\n%q\n%q\n%q\n", task.ID, task.Source, branch, head)
	_, _ = digest.Write(patch)
	return work.WorktreeHandoff{Worktree: task, Branch: branch, Head: head, Digest: hex.EncodeToString(digest.Sum(nil)), Patch: string(patch)}, nil
}

func (b *TaskWorktrees) Apply(ctx context.Context, preview work.WorktreeHandoff) (err error) {
	if preview.Patch == "" {
		return nil
	}
	file, err := os.CreateTemp(b.root, "handoff-*.patch")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.Remove(file.Name())) }()
	_, writeErr := file.WriteString(preview.Patch)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	if _, err := taskGitValue(ctx, preview.Worktree.Source, "apply", "--check", "--index", "--binary", "--", file.Name()); err != nil {
		return errors.Join(work.ErrTaskWorktreeChanged, err)
	}
	_, err = taskGitValue(ctx, preview.Worktree.Source, "apply", "--index", "--binary", "--", file.Name())
	return err
}

func (b *TaskWorktrees) Release(ctx context.Context, task work.TaskWorktree) error {
	if task.Available {
		head, err := taskGitValue(ctx, task.Path, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return err
		}
		if head != task.Base {
			return fmt.Errorf("%w: the task worktree contains committed changes", work.ErrTaskWorktreeChanged)
		}
		if _, err := taskGitValue(ctx, task.Source, "worktree", "remove", "--", task.Path); err != nil {
			return errors.Join(work.ErrTaskWorktreeChanged, err)
		}
	} else if _, err := os.Lstat(task.Path); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(work.ErrTaskWorktreeChanged, err)
	}
	root, err := os.OpenRoot(b.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(filepath.Join(task.ID, taskManifest)); err != nil {
		return err
	}
	return root.Remove(task.ID)
}

func taskPatch(ctx context.Context, task work.TaskWorktree) ([]byte, error) {
	patch, err := taskGitBytes(ctx, task.Path, 0, "diff", "--binary", "--no-ext-diff", "--no-textconv", "--no-color", task.Base, "--")
	if err != nil {
		return nil, err
	}
	if len(patch) > maxHandoff {
		return nil, work.ErrVCSResultTooLarge
	}
	untracked, err := taskGitBytes(ctx, task.Path, 0, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for name := range bytes.SplitSeq(untracked, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		if !filepath.IsLocal(string(name)) {
			return nil, work.ErrPathOutsideRoot
		}
		delta, err := taskGitBytes(ctx, task.Path, 1, "diff", "--no-index", "--binary", "--no-ext-diff", "--no-textconv", "--no-color", "--", os.DevNull, string(name))
		if err != nil {
			return nil, err
		}
		if len(delta) > maxHandoff-len(patch) {
			return nil, work.ErrVCSResultTooLarge
		}
		patch = append(patch, delta...)
	}
	return patch, nil
}

func taskCommonDir(ctx context.Context, path string) (string, error) {
	common, err := taskGitValue(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(common)
}

func taskGitValue(ctx context.Context, path string, args ...string) (string, error) {
	raw, err := taskGitBytes(ctx, path, 0, args...)
	return strings.TrimSuffix(string(raw), "\n"), err
}

func taskGitBytes(ctx context.Context, path string, acceptedExit int, args ...string) ([]byte, error) {
	result, err := process.At(ctx, path, args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 && result.ExitCode != acceptedExit {
		return nil, fmt.Errorf("workspace: git %s failed with exit %d: %s", args[0], result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
}

func writeTaskRecord(root *os.Root, record taskRecord) (err error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	directory, err := root.OpenRoot(record.ID)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	pending := taskManifest + ".next"
	file, err := directory.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filesystem.PrivateFileMode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return errors.Join(err, directory.Remove(pending))
	}
	if err := directory.Rename(pending, taskManifest); err != nil {
		return errors.Join(err, directory.Remove(pending))
	}
	entry, err := directory.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(entry.Sync(), entry.Close())
}

func isTaskID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && value == strings.ToLower(value)
}

func validTaskSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && (len(decoded) == 20 || len(decoded) == 32)
}
