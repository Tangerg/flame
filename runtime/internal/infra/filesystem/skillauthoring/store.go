// Package skillauthoring owns the governed write side of one Agent Skills
// library. Each proposal name has one atomically replaced current document;
// review references remain immutable content digests. Active lifecycle moves
// never overwrite a destination except the explicit single archive slot.
package skillauthoring

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/advisorylock"
)

// Store serializes writes to one scoped skills root. The same instance must be
// shared by every in-process consumer of that root. A scoped directory lease
// linearizes capacity checks, replacement, lifecycle moves, and review
// decisions across Runtime processes that share the library.
type Store struct {
	root  string
	scope skills.Scope
	mu    sync.RWMutex
}

// NewStore roots the authoring store at one project or user Skill library.
// The absolute root keeps every operation bound to the configured library.
func NewStore(root string, scope skills.Scope) (*Store, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("skillauthoring: skills root must be absolute")
	}
	if err := scope.Validate(); err != nil {
		return nil, fmt.Errorf("skillauthoring: store scope: %w", err)
	}
	return &Store{root: root, scope: scope}, nil
}

func (s *Store) openRoot() (*os.Root, error) {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return nil, fmt.Errorf("skillauthoring: create skills root: %w", err)
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: open skills root: %w", err)
	}
	return root, nil
}

func (s *Store) openLeasedRoot(ctx context.Context, operation string) (*os.Root, func(), error) {
	root, err := s.openRoot()
	if err != nil {
		return nil, nil, err
	}
	lease, err := advisorylock.AcquireDirectory(ctx, s.root)
	if err != nil {
		_ = root.Close()
		return nil, nil, fmt.Errorf("skillauthoring: %s: acquire library lease: %w", operation, err)
	}
	cleanup := func() {
		_ = lease.Release()
		_ = root.Close()
	}
	return root, cleanup, nil
}

func (s *Store) activeDir(name string) string { return name }

func (s *Store) archiveDir(name string) string {
	return filepath.Join(archivedSubdir, name)
}

func (s *Store) skillIdentities(directories ...string) []string {
	identities := make([]string, 0, len(directories))
	for _, directory := range directories {
		identities = append(identities, filepath.Join(s.root, directory, skillspec.SkillFile))
	}
	return distinctPaths(identities)
}

func distinctPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" && !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	return out
}

func identitiesIf(changed bool, identities []string) []string {
	if !changed {
		return nil
	}
	return identities
}

func removeSkillTree(root *os.Root, directory string) (bool, error) {
	_, existed, err := readSkill(root, directory)
	if err != nil {
		return false, err
	}
	removeErr := root.RemoveAll(directory)
	if removeErr == nil || errors.Is(removeErr, fs.ErrNotExist) {
		return existed, nil
	}
	_, remains, inspectErr := readSkill(root, directory)
	return existed && !remains, errors.Join(removeErr, inspectErr)
}

func (s *Store) lifecycleDir(lifecycle skills.Lifecycle, name string) (string, error) {
	switch lifecycle {
	case skills.Active:
		return s.activeDir(name), nil
	case skills.Archived:
		return s.archiveDir(name), nil
	default:
		return "", fmt.Errorf("skillauthoring: unknown lifecycle %q", lifecycle)
	}
}

func validateSkill(name string, content []byte) error {
	if len(content) > skills.MaxAuthoredSkillDocumentBytes {
		return fmt.Errorf(
			"skillauthoring: validate skill %q: %w: %d bytes exceeds %d",
			name,
			skills.ErrDocumentTooLarge,
			len(content),
			skills.MaxAuthoredSkillDocumentBytes,
		)
	}
	skill, err := skillspec.Parse(content)
	if err != nil {
		return fmt.Errorf("skillauthoring: parse skill %q: %w", name, err)
	}
	if strings.TrimSpace(skill.Instructions) == "" {
		return fmt.Errorf("skillauthoring: validate skill %q: skill instructions are required", name)
	}
	if skill.Name != name {
		return fmt.Errorf("skillauthoring: skill name mismatch: frontmatter %q, path %q", skill.Name, name)
	}
	proposal := skills.Proposal{Name: skill.Name, Description: skill.Description, Instructions: skill.Instructions}
	if issue := proposal.SafetyIssue(); issue != skills.ProposalSafe {
		return proposalSafetyError(name, issue)
	}
	return nil
}

func proposalSafetyError(name string, issue skills.ProposalSafetyIssue) error {
	switch issue {
	case skills.ProposalDangerousInstruction:
		return fmt.Errorf("skillauthoring: reject skill %q: dangerous instruction", name)
	default:
		return fmt.Errorf("skillauthoring: reject skill %q: unknown safety issue", name)
	}
}

func contextError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("skillauthoring: %s: %w", operation, err)
	}
	return nil
}

func validName(name string) bool {
	return skillspec.ValidateName(name) == nil
}
