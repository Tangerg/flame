package skillauthoring

import (
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

func writeFile(root *os.Root, path string, content []byte, mode fs.FileMode) (err error) {
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("skillauthoring: create %q: %w", path, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("skillauthoring: write %q: %w", path, err)
	}
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("skillauthoring: set permissions for %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("skillauthoring: sync %q: %w", path, err)
	}
	return nil
}

const replacementPrefix = ".skill-replace-"

func replaceFile(root *os.Root, destination string, content []byte, mode fs.FileMode) (err error) {
	temporary := filepath.Join(filepath.Dir(destination), replacementPrefix+rand.Text())
	defer func() {
		if cleanupErr := root.Remove(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if err := writeFile(root, temporary, content, mode); err != nil {
		return err
	}
	return root.Rename(temporary, destination)
}
