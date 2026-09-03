package skillauthoring

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

// List returns active and archived skills from one library snapshot. Directory
// encounter order is preserved; Application owns public catalog order.
func (s *Store) List(ctx context.Context) ([]skills.Entry, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if err := contextError(ctx, "list managed skills"); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "list managed skills")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	activeNames, archivedNames, err := managedSkillNames(root)
	if err != nil {
		return nil, err
	}
	active, err := managedEntries(ctx, root, ".", activeNames, skills.Active)
	if err != nil {
		return nil, err
	}
	archived, err := managedEntries(ctx, root, archivedSubdir, archivedNames, skills.Archived)
	if err != nil {
		return nil, err
	}
	return append(active, archived...), nil
}

func managedEntries(ctx context.Context, root *os.Root, directory string, names []string, lifecycle skills.Lifecycle) ([]skills.Entry, error) {
	out := make([]skills.Entry, 0, len(names))
	for _, name := range names {
		if err := contextError(ctx, "list managed skills"); err != nil {
			return nil, err
		}
		content, found, err := readSkill(root, filepath.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("skillauthoring: list %s skill %q: %w", lifecycle, name, err)
		}
		if !found {
			continue
		}
		skill, err := skillspec.Parse(content)
		if err != nil || skill.Name != name {
			continue
		}
		out = append(out, skills.Entry{Name: name, Description: skill.Description, Lifecycle: lifecycle})
	}
	return out, nil
}

func ensureManagedSkillCapacity(root *os.Root) error {
	active, archived, err := managedSkillNames(root)
	if err != nil {
		return err
	}
	if len(active)+len(archived) >= skills.MaxSkillsPerSource {
		return fmt.Errorf(
			"%w: scope already contains %d managed Skills",
			skills.ErrLibraryCapacity,
			len(active)+len(archived),
		)
	}
	return nil
}

func managedSkillNames(root *os.Root) ([]string, []string, error) {
	active, err := managedSkillNamesAt(root, ".")
	if err != nil {
		return nil, nil, err
	}
	archived, err := managedSkillNamesAt(root, archivedSubdir)
	if err != nil {
		return nil, nil, err
	}
	if len(active)+len(archived) > skills.MaxSkillsPerSource {
		return nil, nil, fmt.Errorf(
			"%w: scope contains %d active and archived Skills; limit is %d",
			skills.ErrLibraryCapacity,
			len(active)+len(archived),
			skills.MaxSkillsPerSource,
		)
	}
	return active, archived, nil
}

func managedSkillNamesAt(root *os.Root, path string) ([]string, error) {
	directory, _, err := fileinput.OpenDirectoryAt(root, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list managed skills at %q: %w", path, err)
	}
	entries, readErr := directory.ReadDir(skills.MaxSkillDirectoryEntries + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("skillauthoring: list managed skills at %q: %w", path, errors.Join(readErr, closeErr))
	}
	if len(entries) > skills.MaxSkillDirectoryEntries {
		return nil, fmt.Errorf(
			"%w: %q contains more than %d directory entries",
			skills.ErrLibraryCapacity,
			path,
			skills.MaxSkillDirectoryEntries,
		)
	}
	names := make([]string, 0, min(len(entries), skills.MaxSkillsPerSource))
	for _, entry := range entries {
		if !entry.IsDir() || !validName(entry.Name()) {
			continue
		}
		if len(names) == skills.MaxSkillsPerSource {
			return nil, fmt.Errorf(
				"%w: %q contains more than %d Skills",
				skills.ErrLibraryCapacity,
				path,
				skills.MaxSkillsPerSource,
			)
		}
		names = append(names, entry.Name())
	}
	return names, nil
}
