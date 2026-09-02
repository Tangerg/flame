package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	workspacegit "github.com/Tangerg/flame/runtime/internal/infra/git"
)

// commonExcludes keep a checkpoint from ballooning into dependency / build
// output that the source repository does not own. Work-tree .gitignore rules
// remain in force too. These are a backstop for no-ignore projects, but they
// must never erase a path already tracked by the source index; stageChanges
// preserves that distinction after it has selected the exact paths.
const commonExcludes = "node_modules/\n.venv/\nvenv/\n__pycache__/\ndist/\nbuild/\ntarget/\n.next/\n.DS_Store\n"

const (
	maxSourceAlternatesBytes = 64 << 10
	maxSourceAlternates      = 256
	maxSourceIndexBytes      = 64 << 20
	workspaceIdentityFile    = "flame-workspace"
)

// ensureRepo lazily initializes the session's shadow repo (idempotent).
func (s *Store) ensureRepo(ctx context.Context, sessionID, cwd string) (string, error) {
	gitDir := s.gitDir(sessionID, cwd)
	if repoExists(gitDir) {
		// A repository with a commit has completed at least one snapshot. An
		// initialized repository without one may be residue from an interrupted
		// first snapshot; rebuild it instead of trusting a possibly partial index
		// or alternates file.
		hasHead, err := s.hasHead(ctx, gitDir)
		if err != nil {
			return "", err
		}
		if hasHead {
			matches, matchErr := repositoryMatchesWorkspace(gitDir, cwd)
			if matchErr != nil {
				return "", matchErr
			}
			if !matches {
				return "", errors.New("checkpoint: workspace digest collision")
			}
			return gitDir, nil
		}
	}

	sessionDir := s.sessionDir(sessionID)
	// Repositories created before workspace-scoped checkpoint storage put HEAD
	// directly in the Session directory. They carry no workspace identity and
	// therefore cannot be restored safely. Retire that legacy representation
	// before publishing the first scoped repository.
	if repoExists(sessionDir) {
		if err := os.RemoveAll(sessionDir); err != nil {
			return "", fmt.Errorf("checkpoint: remove unscoped Session repository: %w", err)
		}
	}
	parent := filepath.Dir(gitDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("checkpoint: create repository parent: %w", err)
	}
	stagingDir, err := os.MkdirTemp(parent, "."+filepath.Base(gitDir)+".init-")
	if err != nil {
		return "", fmt.Errorf("checkpoint: create repository staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stagingDir)
			// Leave no empty Session namespace after a failed first snapshot. An
			// existing namespace may own another workspace repository; os.Remove
			// safely refuses that non-empty case.
			_ = os.Remove(parent)
		}
	}()

	if _, err := s.git(ctx, stagingDir, cwd, "init", "-q"); err != nil {
		return "", err
	}
	if err := s.seedFrom(ctx, stagingDir, cwd); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "info", "exclude"), []byte(commonExcludes), 0o644); err != nil {
		return "", fmt.Errorf("checkpoint: write excludes: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, workspaceIdentityFile), []byte(cwd), 0o600); err != nil {
		return "", fmt.Errorf("checkpoint: write workspace identity: %w", err)
	}
	if err := publishRepo(stagingDir, gitDir); err != nil {
		return "", err
	}
	published = true
	return gitDir, nil
}

func repositoryMatchesWorkspace(gitDir, cwd string) (bool, error) {
	const maximumWorkspaceIdentityBytes = 64 << 10
	path := filepath.Join(gitDir, workspaceIdentityFile)
	file, _, err := fileinput.Open(path, maximumWorkspaceIdentityBytes)
	if err != nil {
		return false, fmt.Errorf("checkpoint: open workspace identity: %w", err)
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return false, fmt.Errorf("checkpoint: read workspace identity: %w", err)
	}
	return string(data) == cwd, nil
}

// publishRepo makes a fully initialized repository visible in one rename. The
// staging directory is a sibling of dst, so the rename cannot cross filesystems.
// An initialized repository without a commit is safe to replace: it has never
// represented a completed checkpoint boundary.
func publishRepo(stagingDir, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		if removeAllErr := os.RemoveAll(dst); removeAllErr != nil {
			return fmt.Errorf("checkpoint: remove incomplete repository: %w", removeAllErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint: inspect repository destination: %w", err)
	}
	if err := os.Rename(stagingDir, dst); err != nil {
		return fmt.Errorf("checkpoint: publish repository: %w", err)
	}
	return nil
}

// seedFrom wires a freshly-initialized shadow repo to reuse the real repo's
// object store and index, so the first snapshot doesn't re-store the whole tree.
// Sharing objects/info/alternates lets `git add` resolve every unchanged blob
// through the real .git instead of writing a copy; seeding the index reuses the
// existing hashes instead of re-hashing every file — the cost that becomes
// significant on large checkouts.
//
// If cwd isn't a git repo, the shadow starts empty and the first `git add` does
// the full work. Once a source repo has been discovered, however, the seed is
// all-or-nothing: publishing an index without every configured object store
// would create a repository that cannot resolve unchanged files.
func (s *Store) seedFrom(ctx context.Context, gitDir, cwd string) error {
	repository, err := workspacegit.IsRepo(ctx, cwd)
	if err != nil {
		return fmt.Errorf("checkpoint: discover source repository: %w", err)
	}
	if !repository {
		return nil
	}
	seed := sourceRepositorySeed{ctx: ctx, gitDir: gitDir, cwd: cwd}
	return seed.apply()
}

type sourceRepositorySeed struct {
	ctx    context.Context
	gitDir string
	cwd    string
}

func (seed sourceRepositorySeed) apply() error {
	sourceObjects, present, err := seed.sourceObjectStore()
	if err != nil || !present {
		return err
	}
	alternates, err := seed.resolveAlternates(sourceObjects)
	if err != nil {
		return err
	}
	if err := seed.writeAlternates(alternates); err != nil {
		return err
	}
	return seed.copyIndex()
}

func (seed sourceRepositorySeed) sourceObjectStore() (string, bool, error) {
	sourceObjects, err := gitIn(seed.ctx, seed.cwd, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return "", false, fmt.Errorf("checkpoint: discover source object store: %w", err)
	}
	if sourceObjects == "" {
		return "", false, errors.New("checkpoint: source repository returned an empty object-store path")
	}
	info, err := os.Stat(sourceObjects)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("checkpoint: inspect source object store: %w", err)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("checkpoint: source object store %q is not a directory", sourceObjects)
	}
	return sourceObjects, true, nil
}

func (seed sourceRepositorySeed) resolveAlternates(sourceObjects string) ([]string, error) {
	// Share the real object DB plus any store it already borrows, keeping only
	// stores that still exist so the chain resolves. Git interprets relative
	// entries relative to the object database that owns the alternates file.
	alternates := []string{sourceObjects}
	data, err := readSourceAlternates(filepath.Join(sourceObjects, "info", "alternates"))
	if errors.Is(err, os.ErrNotExist) {
		return alternates, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checkpoint: read source alternates: %w", err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		alternate, present, err := resolveSourceAlternate(sourceObjects, line)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		if len(alternates) == maxSourceAlternates {
			return nil, fmt.Errorf(
				"%w: source repository has more than %d object stores",
				ErrSnapshotTooLarge, maxSourceAlternates,
			)
		}
		alternates = append(alternates, alternate)
	}
	return alternates, nil
}

func resolveSourceAlternate(sourceObjects, line string) (string, bool, error) {
	alternate := strings.TrimSpace(line)
	if alternate == "" {
		return "", false, nil
	}
	if !filepath.IsAbs(alternate) {
		alternate = filepath.Join(sourceObjects, alternate)
	}
	alternate = filepath.Clean(alternate)
	info, err := os.Stat(alternate)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("checkpoint: inspect source alternate %q: %w", alternate, err)
	}
	return alternate, info.IsDir(), nil
}

func (seed sourceRepositorySeed) writeAlternates(alternates []string) error {
	if err := os.MkdirAll(filepath.Join(seed.gitDir, "objects", "info"), 0o755); err != nil {
		return fmt.Errorf("checkpoint: create alternates directory: %w", err)
	}
	encodedAlternates := []byte(strings.Join(alternates, "\n") + "\n")
	if len(encodedAlternates) > maxSourceAlternatesBytes {
		return fmt.Errorf(
			"%w: resolved source alternates exceed %d bytes",
			ErrSnapshotTooLarge, maxSourceAlternatesBytes,
		)
	}
	if err := os.WriteFile(filepath.Join(seed.gitDir, "objects", "info", "alternates"), encodedAlternates, 0o644); err != nil {
		return fmt.Errorf("checkpoint: write source alternates: %w", err)
	}
	return nil
}

func (seed sourceRepositorySeed) copyIndex() error {
	sourceIndex, err := gitIn(seed.ctx, seed.cwd, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return fmt.Errorf("checkpoint: discover source index: %w", err)
	}
	if err := copyFile(sourceIndex, filepath.Join(seed.gitDir, "index"), maxSourceIndexBytes); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checkpoint: copy source index: %w", err)
	}
	return nil
}

func readSourceAlternates(path string) ([]byte, error) {
	file, _, err := fileinput.Open(path, maxSourceAlternatesBytes)
	if err != nil {
		return nil, checkpointSourceError(err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxSourceAlternatesBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSourceAlternatesBytes {
		return nil, fmt.Errorf(
			"%w: source alternates exceed %d bytes",
			ErrSnapshotTooLarge, maxSourceAlternatesBytes,
		)
	}
	return data, nil
}

func repoExists(gitDir string) bool {
	info, err := os.Stat(filepath.Join(gitDir, "HEAD"))
	return err == nil && info.Mode().IsRegular()
}

// materializeAlternates copies every object reachable from the shadow refs out
// of borrowed object stores, then removes the alternates link. This preserves
// the cheap source-index seed while making completed checkpoints independent of
// future pruning or deletion of the source repository.
//
// The pending file closes the crash window around verification: if a process
// stops after hiding alternates but before deleting the file, the next call
// verifies the local object graph and either completes the detach or restores
// the alternate before retrying.
func (s *Store) materializeAlternates(ctx context.Context, gitDir string) error {
	infoDir := filepath.Join(gitDir, "objects", "info")
	alternatesPath := filepath.Join(infoDir, "alternates")
	pendingPath := filepath.Join(infoDir, "alternates.pending")

	if _, err := os.Stat(pendingPath); err == nil {
		if _, statErr := os.Stat(alternatesPath); statErr == nil {
			return errors.New("checkpoint: both active and pending alternates exist")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("checkpoint: inspect active alternates: %w", statErr)
		}
		if verifyLocalObjectsErr := s.verifyLocalObjects(ctx, gitDir); verifyLocalObjectsErr == nil {
			if removeErr := os.Remove(pendingPath); removeErr != nil {
				return fmt.Errorf("checkpoint: remove detached alternates: %w", removeErr)
			}
			return nil
		}
		if renameErr := os.Rename(pendingPath, alternatesPath); renameErr != nil {
			return fmt.Errorf("checkpoint: restore interrupted alternates detach: %w", renameErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint: inspect pending alternates: %w", err)
	}

	if _, err := os.Stat(alternatesPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checkpoint: inspect alternates: %w", err)
	}
	// Without --local, repack deliberately copies reachable borrowed objects
	// into the shadow object database.
	if _, err := s.git(ctx, gitDir, "", "repack", "-q", "-a", "-d"); err != nil {
		return err
	}
	if err := os.Rename(alternatesPath, pendingPath); err != nil {
		return fmt.Errorf("checkpoint: detach alternates: %w", err)
	}
	if err := s.verifyLocalObjects(ctx, gitDir); err != nil {
		if restoreErr := os.Rename(pendingPath, alternatesPath); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("checkpoint: restore alternates: %w", restoreErr))
		}
		return err
	}
	if err := os.Remove(pendingPath); err != nil {
		return fmt.Errorf("checkpoint: remove detached alternates: %w", err)
	}
	return nil
}

func (s *Store) verifyLocalObjects(ctx context.Context, gitDir string) error {
	if _, err := s.git(ctx, gitDir, "", "fsck", "--connectivity-only", "--no-dangling"); err != nil {
		return fmt.Errorf("checkpoint: verify local object graph: %w", err)
	}
	return nil
}

// tagFor maps a run id to its snapshot ref name, sanitizing any character git
// disallows in a ref so an unusual run id can't break tagging.
func tagFor(runID string) string {
	var b strings.Builder
	b.WriteString("chk/")
	for _, r := range runID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
