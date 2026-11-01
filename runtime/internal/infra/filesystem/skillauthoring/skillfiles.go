package skillauthoring

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	skillspec "github.com/Tangerg/scope/skills"
)

func readSkill(root *os.Root, dir string) ([]byte, bool, error) {
	path := filepath.Join(dir, skillspec.SkillFile)
	content, found, err := readBoundedFile(root, path)
	if err != nil {
		return nil, false, fmt.Errorf("skillauthoring: read %q: %w", dir, err)
	}
	return content, found, nil
}

func readBoundedFile(root *os.Root, path string) ([]byte, bool, error) {
	file, opened, err := filesystem.OpenFileAt(root, path, skills.MaxAuthoredSkillDocumentBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if errors.Is(err, filesystem.ErrNotRegular) {
		return nil, false, fmt.Errorf("skill document admission: %w", err)
	}
	if errors.Is(err, filesystem.ErrFileTooLarge) {
		return nil, false, fmt.Errorf("%w: document admission: %w", skills.ErrDocumentTooLarge, err)
	}
	if err != nil {
		return nil, false, err
	}
	content, readErr := io.ReadAll(io.LimitReader(file, skills.MaxAuthoredSkillDocumentBytes+1))
	verifyErr := filesystem.VerifyAtVersion(file, opened, root, path)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, false, errors.Join(readErr, verifyErr, closeErr)
	}
	if len(content) > skills.MaxAuthoredSkillDocumentBytes {
		return nil, false, fmt.Errorf(
			"%w: exceeds %d bytes",
			skills.ErrDocumentTooLarge,
			skills.MaxAuthoredSkillDocumentBytes,
		)
	}
	if verifyErr != nil {
		return nil, false, verifyErr
	}
	return content, true, nil
}

// writeFile creates path (which must not exist) and writes+fsyncs content. It
// backs both proposal staging and the usage sidecar, so its messages name the
// operation neutrally; callers add the proposal/usage context.
func writeFile(root *os.Root, path string, content []byte) (err error) {
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("skillauthoring: create %q: %w", path, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("skillauthoring: write %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("skillauthoring: sync %q: %w", path, err)
	}
	return nil
}

func stageSkill(ctx context.Context, root *os.Root, destination string, content []byte) (err error) {
	temporary := ".skill-stage-" + rand.Text()
	if mkdirErr := root.Mkdir(temporary, 0o755); mkdirErr != nil {
		return fmt.Errorf("skillauthoring: create skill staging directory: %w", mkdirErr)
	}
	defer func() {
		if cleanupErr := root.RemoveAll(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("skillauthoring: clean skill staging directory: %w", cleanupErr))
		}
	}()
	if err := writeFile(root, filepath.Join(temporary, skillspec.SkillFile), content); err != nil {
		return err
	}
	if err := contextError(ctx, "publish skill"); err != nil {
		return err
	}
	if err := root.Rename(temporary, destination); err != nil {
		existing, found, readErr := readSkill(root, destination)
		if readErr == nil && found && bytes.Equal(existing, content) {
			return nil
		}
		return fmt.Errorf("skillauthoring: publish skill %q: %w", filepath.Base(destination), errors.Join(err, readErr))
	}
	return nil
}
