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

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

const skillFile = "SKILL.md"

func readSkill(root *os.Root, dir string) ([]byte, bool, error) {
	path := filepath.Join(dir, skillFile)
	content, found, err := readBoundedFile(root, path)
	if err != nil {
		return nil, false, fmt.Errorf("skillauthoring: read %q: %w", dir, err)
	}
	return content, found, nil
}

func readBoundedFile(root *os.Root, path string) ([]byte, bool, error) {
	file, opened, err := fileinput.OpenAt(root, path, skills.MaxAuthoredSkillDocumentBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if errors.Is(err, fileinput.ErrNotRegular) {
		return nil, false, errors.New("skill document is not a regular file")
	}
	if errors.Is(err, fileinput.ErrTooLarge) {
		return nil, false, fmt.Errorf(
			"%w: exceeds %d bytes",
			skills.ErrDocumentTooLarge,
			skills.MaxAuthoredSkillDocumentBytes,
		)
	}
	if err != nil {
		return nil, false, err
	}
	content, readErr := io.ReadAll(io.LimitReader(file, skills.MaxAuthoredSkillDocumentBytes+1))
	verifyErr := fileinput.VerifyAtVersion(file, opened, root, path)
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

func stageProposal(ctx context.Context, root *os.Root, destination string, content []byte) (err error) {
	createdSlot := false
	if mkdirErr := root.Mkdir(destination, 0o755); mkdirErr == nil {
		createdSlot = true
	} else if !errors.Is(mkdirErr, fs.ErrExist) {
		return fmt.Errorf("skillauthoring: create proposal slot: %w", mkdirErr)
	}
	temporary := filepath.Join(destination, ".stage-"+rand.Text())
	published := false
	defer func() {
		if cleanupErr := root.Remove(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("skillauthoring: clean proposal staging file: %w", cleanupErr))
		}
		if !published && createdSlot {
			if cleanupErr := root.Remove(destination); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("skillauthoring: clean empty proposal slot: %w", cleanupErr))
			}
		}
	}()
	if err := writeFile(root, temporary, content); err != nil {
		return err
	}
	if err := contextError(ctx, "publish proposal"); err != nil {
		return err
	}
	if err := root.Rename(temporary, filepath.Join(destination, skillFile)); err != nil {
		existing, found, readErr := readSkill(root, destination)
		if readErr == nil && found && bytes.Equal(existing, content) {
			published = true
			return nil
		}
		return fmt.Errorf("skillauthoring: publish proposal %q: %w", filepath.Base(destination), errors.Join(err, readErr))
	}
	published = true
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
	if err := writeFile(root, filepath.Join(temporary, skillFile), content); err != nil {
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

// removeProposal conditionally removes the current proposal only when its
// bytes still match ref.Revision. The caller owns the scoped library lease, so
// the compare and removal are one cross-process linearized decision.
func removeProposal(root *os.Root, ref skills.ProposalRef) (bool, error) {
	directory := filepath.Join(proposalsSubdir, ref.Name)
	content, found, err := readSkill(root, directory)
	if err != nil || !found {
		return false, err
	}
	if !ref.Matches(content) {
		return false, fmt.Errorf("%w: %q revision %q", skills.ErrProposalChanged, ref.Name, ref.Revision)
	}
	if err := root.RemoveAll(directory); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("skillauthoring: remove proposal %q: %w", ref.Name, err)
	}
	return true, nil
}
