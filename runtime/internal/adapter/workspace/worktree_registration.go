package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

const (
	gitWorktreeRegistrations = "worktrees"
	gitWorktreeBacklink      = "gitdir"
	taskCustody              = "flame-task-id"
)

type taskRegistration struct {
	directory string
	checkout  string
}

func (r taskRecord) registration() (*taskRegistration, error) {
	common, err := os.OpenRoot(r.Common)
	if err != nil {
		return nil, fmt.Errorf("%w: open task Git registry: %w", work.ErrTaskWorktreeChanged, err)
	}
	defer common.Close()
	registry, err := common.Open(gitWorktreeRegistrations)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer registry.Close()
	entries, err := registry.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	registryPath := filepath.Join(r.Common, gitWorktreeRegistrations)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		registration, err := r.registrationAt(filepath.Join(registryPath, entry.Name()))
		if err != nil || registration != nil {
			return registration, err
		}
	}
	return nil, nil
}

func (r taskRecord) registrationAt(path string) (*taskRegistration, error) {
	if filepath.Base(filepath.Dir(path)) != gitWorktreeRegistrations {
		return nil, nil
	}
	directory, err := os.OpenRoot(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	owner, err := directory.ReadFile(taskCustody)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: read task worktree custody: %w", err)
	}
	if string(owner) != r.ID {
		return nil, nil
	}
	backlink, err := directory.ReadFile(gitWorktreeBacklink)
	if err != nil {
		return nil, fmt.Errorf("workspace: read task Git registration: %w", err)
	}
	// Git retains this backlink when the checkout disappears.
	link := strings.TrimSuffix(string(backlink), "\n")
	if filepath.Base(link) != git.MetadataDirectory {
		return nil, errors.New("workspace: invalid task Git registration backlink")
	}
	if !filepath.IsAbs(link) {
		link = filepath.Join(path, link)
	}
	return &taskRegistration{directory: path, checkout: filepath.Dir(link)}, nil
}

func (r taskRecord) registrationForRelease(ctx context.Context, checkout string) (*taskRegistration, error) {
	gitDir, err := checkoutGitDirectory(ctx, checkout)
	if errors.Is(err, os.ErrNotExist) {
		return r.registration()
	}
	if err != nil {
		return nil, err
	}
	registration, err := r.registrationAt(gitDir)
	if err == nil && registration == nil {
		return nil, fmt.Errorf("%w: the task path belongs to another checkout or directory", work.ErrTaskWorktreeChanged)
	}
	return registration, err
}

func (b *TaskWorktrees) Release(ctx context.Context, task work.TaskWorktree, complete operation.Reply[struct{}]) error {
	record, err := b.load(task.ID)
	if err != nil {
		return err
	}
	registration, err := record.registrationForRelease(ctx, task.Path)
	if err != nil {
		return err
	}
	if registration != nil {
		if registration.checkout != task.Path {
			return fmt.Errorf("%w: Git moved the task worktree to %q", work.ErrTaskWorktreeChanged, registration.checkout)
		}
		head, err := taskGitValue(ctx, registration.directory, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return err
		}
		if head != record.Base {
			return fmt.Errorf("%w: the task worktree contains committed changes", work.ErrTaskWorktreeChanged)
		}
	}
	return complete(ctx, func(work context.Context) (struct{}, error) {
		return struct{}{}, b.removeTask(work, record.ID, registration)
	})
}

func (b *TaskWorktrees) removeTask(ctx context.Context, id string, registration *taskRegistration) error {
	if registration != nil {
		common := filepath.Dir(filepath.Dir(registration.directory))
		if _, err := taskGitValue(ctx, common, "worktree", "remove", "--", registration.checkout); err != nil {
			return errors.Join(work.ErrTaskWorktreeChanged, err)
		}
	}
	root, err := os.OpenRoot(b.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(filepath.Join(id, taskManifest)); err != nil {
		return err
	}
	return root.Remove(id)
}
