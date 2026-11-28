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
	common, err := os.OpenRoot(r.CommonDir)
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
	registryPath := filepath.Join(r.CommonDir, gitWorktreeRegistrations)
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

func (t *taskCheckout) checkoutRegistration(ctx context.Context) (*taskRegistration, error) {
	gitDir, err := checkoutGitDirectory(ctx, t.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	registration, err := t.record.registrationAt(gitDir)
	if registration != nil && registration.checkout != t.path {
		return nil, nil
	}
	return registration, err
}

func (t *taskCheckout) PrepareRelease(ctx context.Context) (operation.ExternalExecution, error) {
	registration, err := t.record.registrationForRelease(ctx, t.path)
	if err != nil {
		return nil, err
	}
	write := releaseWrite{Manifest: filepath.Join(filepath.Dir(t.path), taskRecordName(t.record.ID))}
	if registration != nil {
		if registration.checkout != t.path {
			return nil, fmt.Errorf("%w: Git moved the task worktree to %q", work.ErrTaskWorktreeChanged, registration.checkout)
		}
		head, err := taskGitValue(ctx, registration.directory, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return nil, err
		}
		if head != t.record.Base {
			return nil, fmt.Errorf("%w: the task worktree contains committed changes", work.ErrTaskWorktreeChanged)
		}
		write.Registration = &checkoutRemoval{
			Repository: filepath.Dir(filepath.Dir(registration.directory)),
			Checkout:   registration.checkout,
		}
	}
	return t.writer.prepare(writeRelease, write)
}

func checkoutGitDirectory(ctx context.Context, checkout string) (string, error) {
	info, err := os.Lstat(checkout)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", nil
	}
	if repository, err := git.IsRepo(ctx, checkout); err != nil || !repository {
		return "", err
	}
	top, err := taskGitValue(ctx, checkout, "rev-parse", "--show-toplevel")
	if err != nil || top != checkout {
		return "", err
	}
	return taskGitValue(ctx, checkout, "rev-parse", "--absolute-git-dir")
}
