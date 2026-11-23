package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
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

type FileChange struct {
	Path         string
	Status       Status
	PreviousPath string
	Added        int
	Removed      int
	Binary       bool
}

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

func ListChanges(ctx context.Context, dir string, maxChanges int) ([]FileChange, error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !repository {
		return nil, ErrNotRepo
	}
	prefixOut, err := run(ctx, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	repositoryPrefix := strings.TrimRight(string(prefixOut), "\r\n")

	statusOut, err := run(ctx, dir, "status", "--ignore-submodules=dirty", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil, err
	}
	changes, order, err := parseStatusZ(statusOut, maxChanges)
	if err != nil {
		return nil, err
	}

	head, err := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(head)) != 0 {
		numOut, err := run(ctx, dir, "diff", "--numstat", "--no-ext-diff", "--no-textconv", "-M", "-z", "HEAD", "--", ".")
		if err != nil {
			return nil, err
		}
		if err := applyNumstatZ(numOut, changes); err != nil {
			return nil, err
		}
	}

	out := make([]FileChange, 0, len(order))
	for _, p := range order {
		change, inside := workspaceChange(*changes[p], repositoryPrefix)
		if !inside {
			continue
		}
		if change.Status == StatusUntracked {
			added, binary, err := untrackedFileStat(ctx, dir, change.Path)
			if err != nil {
				return nil, err
			}
			change.Added, change.Removed, change.Binary = added, 0, binary
		}
		out = append(out, change)
	}
	return out, nil
}

func workspaceChange(change FileChange, repositoryPrefix string) (FileChange, bool) {
	path, pathInside := workspaceRelativeGitPath(change.Path, repositoryPrefix)
	if change.Status != StatusRenamed {
		change.Path = path
		return change, pathInside
	}
	previousPath, previousInside := workspaceRelativeGitPath(change.PreviousPath, repositoryPrefix)
	switch {
	case pathInside && previousInside:
		change.Path, change.PreviousPath = path, previousPath
	case pathInside:
		change.Path, change.PreviousPath, change.Status = path, "", StatusAdded
	case previousInside:
		change.Path, change.PreviousPath, change.Status = previousPath, "", StatusDeleted
	default:
		return change, false
	}
	return change, true
}

// Porcelain v1 with -z ignores status.relativePaths, so paths are mapped back to
// the WorkspaceRef root here.
func workspaceRelativeGitPath(path, repositoryPrefix string) (string, bool) {
	if path == "" {
		return "", false
	}
	if repositoryPrefix == "" {
		return path, true
	}
	relative, ok := strings.CutPrefix(path, repositoryPrefix)
	return relative, ok && relative != ""
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
const statusPathOffset = len("XY ")

func parseStatusZ(out []byte, maxChanges int) (map[string]*FileChange, []string, error) {
	changes := map[string]*FileChange{}
	var order []string
	next := func() ([]byte, bool) {
		if len(out) == 0 {
			return nil, false
		}
		index := bytes.IndexByte(out, 0)
		if index < 0 {
			field := out
			out = nil
			return field, true
		}
		field := out[:index]
		out = out[index+1:]
		return field, true
	}
	for {
		rec, ok := next()
		if !ok {
			break
		}
		if len(rec) <= statusPathOffset || rec[statusPathOffset-1] != ' ' {
			return nil, nil, fmt.Errorf("git: malformed status record %q", rec)
		}
		if maxChanges <= 0 || len(order) >= maxChanges {
			return nil, nil, fmt.Errorf("%w: more than %d workspace changes", ErrResultTooLarge, maxChanges)
		}
		xy, path := string(rec[:statusPathOffset-1]), string(rec[statusPathOffset:])
		fc := &FileChange{Path: path, Status: statusFromXY(xy)}
		if xy[0] == 'R' || xy[1] == 'R' {
			if previous, ok := next(); ok {
				fc.PreviousPath = string(previous)
			}
			fc.Status = StatusRenamed
		}
		changes[path] = fc
		order = append(order, path)
	}
	return changes, order, nil
}

func statusFromXY(xy string) Status {
	switch {
	case xy == "??":
		return StatusUntracked
	case xy[0] == 'A' || xy[1] == 'A':
		return StatusAdded
	case xy[0] == 'D' || xy[1] == 'D':
		return StatusDeleted
	default:
		return StatusModified
	}
}

// Numstat records are "ADDED\tDELETED\tPATH".
const numstatFields = 3

func applyNumstatZ(out []byte, changes map[string]*FileChange) error {
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		line := fields[i]
		if len(line) == 0 {
			continue
		}
		parts := bytes.SplitN(line, []byte{'\t'}, numstatFields)
		if len(parts) < numstatFields {
			return fmt.Errorf("git: malformed numstat record %q", line)
		}
		addS, remS, path := string(parts[0]), string(parts[1]), string(parts[2])
		if path == "" {
			if i+2 < len(fields) {
				path = string(fields[i+2])
				i += 2
			} else {
				return fmt.Errorf("git: malformed rename numstat record %q", line)
			}
		}
		if addS == "-" || remS == "-" {
			if fc := changes[path]; fc != nil {
				fc.Binary = true
			}
			continue
		}
		added, err := strconv.Atoi(addS)
		if err != nil || added < 0 {
			return fmt.Errorf("git: invalid added-line count %q for %q", addS, path)
		}
		removed, err := strconv.Atoi(remS)
		if err != nil || removed < 0 {
			return fmt.Errorf("git: invalid removed-line count %q for %q", remS, path)
		}
		fc := changes[path]
		if fc == nil {
			continue
		}
		fc.Added = added
		fc.Removed = removed
	}
	return nil
}
