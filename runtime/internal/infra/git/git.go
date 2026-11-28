package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

var (
	ErrUnavailable    = errors.New("git: binary not available")
	ErrNotRepo        = errors.New("git: not a repository")
	ErrResultTooLarge = errors.New("git: result too large")
)

// MetadataDirectory is where a working tree keeps repository state; a
// submodule records it as a file instead of a directory.
const MetadataDirectory = ".git"

type Status string

const (
	StatusAdded     Status = "added"
	StatusModified  Status = "modified"
	StatusDeleted   Status = "deleted"
	StatusRenamed   Status = "renamed"
	StatusUntracked Status = "untracked"
)

var availableOnce struct {
	sync.Once
	ok bool
}

const fatalExitCode = 128

func Available() bool {
	availableOnce.Do(func() {
		_, err := exec.LookPath("git")
		availableOnce.ok = err == nil
	})
	return availableOnce.ok
}

func IsRepo(ctx context.Context, dir string) (bool, error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return false, fmt.Errorf("git rev-parse --is-inside-work-tree: %w", err)
	}
	if !Available() {
		return false, ErrUnavailable
	}
	args := []string{"rev-parse", "--is-inside-work-tree"}
	// The only expected negative result is identified from Git's stable English
	// diagnostic. Exit 128 alone is not enough: unsafe ownership, corrupt
	// metadata, and an unreadable repository use the same status and must remain
	// observable to the caller.
	result, err := process.At(ctx, dir, args...)
	if err != nil {
		return false, gitProcessError(args, err)
	}
	if result.ExitCode != 0 {
		diagnostic := strings.TrimSpace(result.Stderr)
		if result.ExitCode == fatalExitCode && isNotRepositoryDiagnostic(diagnostic) {
			return false, nil
		}
		return false, gitExitError(args, result.ExitCode, diagnostic)
	}
	return bytes.Equal(bytes.TrimSpace(result.Stdout), []byte("true")), nil
}

func isNotRepositoryDiagnostic(diagnostic string) bool {
	return strings.HasPrefix(diagnostic, "fatal: not a git repository")
}

func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return runAllowingExitCode(ctx, dir, -1, args...)
}

func runAllowingExitCode(ctx context.Context, dir string, allowedExitCode int, args ...string) ([]byte, error) {
	// Workspace VCS operations are observations. Suppress Git's optional index
	// refreshes so commands such as status do not take index.lock merely to
	// improve a later read. Some Git commands still perform mandatory metadata
	// refreshes; the workspace watcher compares semantic Git state before it
	// publishes and therefore does not expose those implementation writes.
	result, err := process.At(ctx, dir, args...)
	return readCommandOutput(args, result, err, allowedExitCode)
}

func readCommandOutput(args []string, result process.Result, err error, allowedExitCode int) ([]byte, error) {
	if err != nil {
		return nil, gitProcessError(args, err)
	}
	if result.ExitCode == 0 || result.ExitCode == allowedExitCode {
		return result.Stdout, nil
	}
	return result.Stdout, gitExitError(args, result.ExitCode, strings.TrimSpace(result.Stderr))
}

func gitProcessError(args []string, err error) error {
	command := strings.Join(args, " ")
	if errors.Is(err, exec.ErrNotFound) {
		err = errors.Join(ErrUnavailable, err)
	}
	if errors.Is(err, process.ErrOutputTooLarge) {
		err = errors.Join(ErrResultTooLarge, err)
	}
	return fmt.Errorf("git %s: %w", command, err)
}

func gitExitError(args []string, exitCode int, diagnostic string) error {
	command := strings.Join(args, " ")
	if diagnostic != "" {
		return fmt.Errorf("git %s: %s: exit code %d", command, diagnostic, exitCode)
	}
	return fmt.Errorf("git %s: exit code %d", command, exitCode)
}

func ListFiles(ctx context.Context, dir, relPath string, maxFiles int) ([]string, error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !repository {
		return nil, ErrNotRepo
	}
	args := []string{"ls-files", "--cached", "--deduplicate", "--others", "--exclude-standard", "-z"}
	scope, err := workspacePathspec(dir, relPath)
	if err != nil {
		return nil, err
	}
	args = append(args, "--", scope)
	out, err := run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	var files []string
	for path := range bytes.SplitSeq(out, []byte{0}) {
		if len(path) != 0 {
			if maxFiles <= 0 || len(files) >= maxFiles {
				return nil, fmt.Errorf("%w: more than %d workspace files", ErrResultTooLarge, maxFiles)
			}
			files = append(files, string(path))
		}
	}
	return files, nil
}

// Porcelain v1 records read "XY PATH": two status letters, a space, then the path.
