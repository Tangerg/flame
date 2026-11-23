package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/capture"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspacegit "github.com/Tangerg/flame/runtime/internal/infra/git"
)

const commonExcludes = "node_modules/\n.venv/\nvenv/\n__pycache__/\ndist/\nbuild/\ntarget/\n.next/\n.DS_Store\n"

const (
	maxSourceAlternatesBytes  = 64 << 10
	maxSourceAlternates       = 256
	maxWorkspaceIdentityBytes = 64 << 10
	workspaceIdentityFile     = "flame-source-workspace"
)

func (s *Store) ensureRepo(ctx context.Context, sessionID, cwd string) (string, error) {
	gitDir := s.gitDir(sessionID, cwd)
	present, err := repoExists(gitDir)
	if err != nil {
		return "", err
	}
	if present {
		matches, err := repositoryMatchesWorkspace(gitDir, cwd)
		if err != nil {
			return "", err
		}
		if !matches {
			return "", errors.New("checkpoint: workspace digest collision")
		}
		return gitDir, nil
	}

	parent := filepath.Dir(gitDir)
	if err := os.MkdirAll(parent, filesystem.SharedDirectoryMode); err != nil {
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

	if err := s.git(ctx, stagingDir, cwd, "init", "-q"); err != nil {
		return "", err
	}
	if err := s.seedFrom(ctx, stagingDir, cwd); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "info", "exclude"), []byte(commonExcludes), filesystem.SharedFileMode); err != nil {
		return "", fmt.Errorf("checkpoint: write excludes: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, workspaceIdentityFile), []byte(cwd), filesystem.PrivateFileMode); err != nil {
		return "", fmt.Errorf("checkpoint: write workspace identity: %w", err)
	}
	if err := publishRepo(stagingDir, gitDir); err != nil {
		return "", err
	}
	published = true
	return gitDir, nil
}

func repositoryMatchesWorkspace(gitDir, cwd string) (bool, error) {
	path := filepath.Join(gitDir, workspaceIdentityFile)
	file, opened, err := filesystem.OpenFile(path, maxWorkspaceIdentityBytes)
	if err != nil {
		return false, fmt.Errorf("checkpoint: open workspace identity: %w", err)
	}
	data, readErr := readWorkspaceIdentity(file)
	verifyErr := filesystem.VerifyPathVersion(file, opened, path)
	closeErr := file.Close()
	if err := errors.Join(readErr, verifyErr, closeErr); err != nil {
		return false, fmt.Errorf("checkpoint: read workspace identity: %w", err)
	}
	return string(data) == cwd, nil
}

func readWorkspaceIdentity(source io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(source, maxWorkspaceIdentityBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxWorkspaceIdentityBytes {
		return nil, fmt.Errorf(
			"%w: workspace identity exceeds %d bytes",
			errSnapshotTooLarge, maxWorkspaceIdentityBytes,
		)
	}
	return data, nil
}

// The staging directory is a sibling of dst so the rename cannot cross
// filesystems.
func publishRepo(stagingDir, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return errors.New("checkpoint: repository destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint: inspect repository destination: %w", err)
	}
	if err := os.Rename(stagingDir, dst); err != nil {
		return fmt.Errorf("checkpoint: publish repository: %w", err)
	}
	return nil
}

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
	return seed.writeAlternates(alternates)
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
	data, err := readSourceAlternates(seed.ctx, alternatesFile(sourceObjects))
	if err != nil {
		return nil, fmt.Errorf("checkpoint: read source alternates: %w", err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if err := errors.Join(seed.ctx.Err(), context.Cause(seed.ctx)); err != nil {
			return nil, err
		}
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
				errSnapshotTooLarge, maxSourceAlternates,
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
	if err := errors.Join(seed.ctx.Err(), context.Cause(seed.ctx)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(alternatesFile(objectDatabase(seed.gitDir))), filesystem.SharedDirectoryMode); err != nil {
		return fmt.Errorf("checkpoint: create alternates directory: %w", err)
	}
	encodedAlternates := []byte(strings.Join(alternates, "\n") + "\n")
	if len(encodedAlternates) > maxSourceAlternatesBytes {
		return fmt.Errorf(
			"%w: resolved source alternates exceed %d bytes",
			errSnapshotTooLarge, maxSourceAlternatesBytes,
		)
	}
	if err := os.WriteFile(alternatesFile(objectDatabase(seed.gitDir)), encodedAlternates, filesystem.SharedFileMode); err != nil {
		return fmt.Errorf("checkpoint: write source alternates: %w", err)
	}
	return nil
}

func readSourceAlternates(ctx context.Context, path string) (data []byte, err error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	file, opened, err := filesystem.OpenFile(path, maxSourceAlternatesBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.Join(ctx.Err(), context.Cause(ctx))
	}
	if err != nil {
		return nil, errors.Join(checkpointSourceError(err), errors.Join(ctx.Err(), context.Cause(ctx)))
	}
	defer func() {
		err = errors.Join(err, file.Close())
		if err != nil {
			data = nil
		}
	}()
	contextReader, err := capture.NewReader(ctx, file)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(contextReader, maxSourceAlternatesBytes+1))
	var capacityErr error
	if len(data) > maxSourceAlternatesBytes {
		capacityErr = fmt.Errorf(
			"%w: source alternates exceed %d bytes",
			errSnapshotTooLarge, maxSourceAlternatesBytes,
		)
	}
	verifyErr := filesystem.VerifyPathVersion(file, opened, path)
	if err := errors.Join(readErr, capacityErr, verifyErr, errors.Join(ctx.Err(), context.Cause(ctx))); err != nil {
		return nil, err
	}
	return data, nil
}

func checkpointSourceError(err error) error {
	if errors.Is(err, filesystem.ErrFileTooLarge) {
		return fmt.Errorf("%w: %w", errSnapshotTooLarge, err)
	}
	return err
}

func repoExists(gitDir string) (bool, error) {
	info, err := os.Stat(gitDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checkpoint: inspect repository directory: %w", err)
	}
	if !info.IsDir() {
		return false, errors.New("checkpoint: repository path is not a directory")
	}
	info, err = os.Stat(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return false, fmt.Errorf("checkpoint: inspect repository head: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("checkpoint: repository head is not a regular file")
	}
	return true, nil
}

// A pending file left by an interrupted detach is completed when the shadow
// store already holds every object, and rolled back otherwise.
func (s *Store) settlePendingAlternates(ctx context.Context, gitDir, alternatesPath, pendingPath string) (detached bool, err error) {
	if _, statErr := os.Stat(alternatesPath); statErr == nil {
		return false, errors.New("checkpoint: both active and pending alternates exist")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return false, fmt.Errorf("checkpoint: inspect active alternates: %w", statErr)
	}
	if s.verifyLocalObjects(ctx, gitDir) == nil {
		if err := os.Remove(pendingPath); err != nil {
			return false, fmt.Errorf("checkpoint: remove detached alternates: %w", err)
		}
		return true, nil
	}
	if err := os.Rename(pendingPath, alternatesPath); err != nil {
		return false, fmt.Errorf("checkpoint: restore interrupted alternates detach: %w", err)
	}
	return false, nil
}

// The pending file closes the crash window: a process that stops after hiding
// alternates is completed or rolled back by the next call.
func (s *Store) materializeAlternates(ctx context.Context, gitDir string) error {
	alternatesPath := alternatesFile(objectDatabase(gitDir))
	pendingPath := alternatesPath + ".pending"

	if _, err := os.Stat(pendingPath); err == nil {
		detached, err := s.settlePendingAlternates(ctx, gitDir, alternatesPath, pendingPath)
		if err != nil || detached {
			return err
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
	if err := s.git(ctx, gitDir, "", "repack", "-q", "-a", "-d"); err != nil {
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
	if err := s.git(ctx, gitDir, "", "fsck", "--connectivity-only", "--no-dangling"); err != nil {
		return fmt.Errorf("checkpoint: verify local object graph: %w", err)
	}
	return nil
}

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

func objectDatabase(gitDir string) string { return filepath.Join(gitDir, "objects") }

func alternatesFile(objects string) string { return filepath.Join(objects, "info", "alternates") }
