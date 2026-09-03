package skillauthoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

// SubmitProposal validates and stages proposal in its name-owned review slot.
// A new revision supersedes the previous pending revision of that name. It
// returns the exact public file identities changed by the call; replaying the
// same proposal is idempotent and returns no identities.
func (s *Store) SubmitProposal(ctx context.Context, proposal skills.Proposal) (skills.ProposalRef, []string, error) {
	if !s.Enabled() {
		return skills.ProposalRef{}, nil, errors.New("skillauthoring: no scoped skills root configured")
	}
	if err := proposal.Validate(); err != nil {
		return skills.ProposalRef{}, nil, err
	}
	if proposal.Scope != s.scope {
		return skills.ProposalRef{}, nil, fmt.Errorf("skillauthoring: proposal scope %q does not match store scope %q", proposal.Scope, s.scope)
	}
	if issue := proposal.SafetyIssue(); issue != skills.ProposalSafe {
		return skills.ProposalRef{}, nil, proposalSafetyError(proposal.Name, issue)
	}
	content, err := renderProposal(proposal)
	if err != nil {
		return skills.ProposalRef{}, nil, err
	}
	ref := skills.NewProposalRef(s.scope, proposal.Name, content)
	if contextErrorErr := contextError(ctx, "save proposal"); contextErrorErr != nil {
		return skills.ProposalRef{}, nil, contextErrorErr
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "submit proposal")
	if err != nil {
		return skills.ProposalRef{}, nil, err
	}
	defer cleanup()

	if err := root.MkdirAll(proposalsSubdir, 0o755); err != nil {
		return skills.ProposalRef{}, nil, fmt.Errorf("skillauthoring: create proposal area: %w", err)
	}
	proposalDir := s.proposalDir(ref)
	info, statErr := root.Lstat(proposalDir)
	switch {
	case statErr == nil && !info.IsDir():
		return skills.ProposalRef{}, nil, fmt.Errorf("%w: proposal slot %q is not a directory", skills.ErrConflict, proposal.Name)
	case errors.Is(statErr, fs.ErrNotExist):
		pending, listErr := proposalSlotNames(root)
		if listErr != nil {
			return skills.ProposalRef{}, nil, listErr
		}
		if len(pending) >= skills.MaxPendingProposalsPerScope {
			return skills.ProposalRef{}, nil, fmt.Errorf(
				"%w: scope %q already has %d pending names",
				skills.ErrProposalQueueFull,
				s.scope,
				len(pending),
			)
		}
	case statErr != nil:
		return skills.ProposalRef{}, nil, fmt.Errorf("skillauthoring: inspect proposal slot %q: %w", proposal.Name, statErr)
	}
	if existing, found, readErr := readSkill(root, proposalDir); readErr != nil {
		return skills.ProposalRef{}, nil, readErr
	} else if found {
		if bytes.Equal(existing, content) {
			return ref, nil, nil
		}
	}
	if err := stageProposal(ctx, root, proposalDir, content); err != nil {
		return skills.ProposalRef{}, nil, err
	}
	return ref, s.skillIdentities(proposalDir), nil
}

// ApproveProposal publishes exactly the immutable proposal represented by
// handle. A newer pending revision of the same name makes an older handle
// stale, while the exact revision remains content-bound throughout review.
// A different active skill is a conflict unless the proposal explicitly
// revises it. Returned identities report every public file change committed
// before return, including partial changes on error.
func (s *Store) ApproveProposal(ctx context.Context, ref skills.ProposalRef) ([]string, error) {
	if err := s.validateRef(ref); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "approve proposal")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	content, found, err := s.readProposal(root, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("skillauthoring: no proposal %q at revision %q: %w", ref.Name, ref.Revision, skills.ErrNotFound)
	}
	if validateSkillErr := validateSkill(ref.Name, content); validateSkillErr != nil {
		return nil, validateSkillErr
	}
	// A revision replaces the active skill of the same name (archiving the old
	// version) rather than conflicting; it also handles its own archive slot, so
	// it runs before the archived-conflict guard below.
	if revises, proposalRevisesErr := proposalRevises(content); proposalRevisesErr != nil {
		return nil, proposalRevisesErr
	} else if revises {
		return s.replaceActive(ctx, root, ref, content)
	}
	if _, statErr := root.Lstat(s.archiveDir(ref.Name)); statErr == nil {
		return nil, fmt.Errorf("%w: archived skill %q", skills.ErrConflict, ref.Name)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillauthoring: inspect archived skill %q: %w", ref.Name, statErr)
	}

	activeDir := s.activeDir(ref.Name)
	if active, exists, readErr := readSkill(root, activeDir); readErr != nil {
		return nil, readErr
	} else if exists {
		if !bytes.Equal(active, content) {
			return nil, fmt.Errorf("%w: active skill %q", skills.ErrConflict, ref.Name)
		}
		removed, removeProposalErr := removeProposal(root, ref)
		identities := identitiesIf(removed, s.skillIdentities(s.proposalDir(ref)))
		if errors.Is(removeProposalErr, skills.ErrProposalChanged) {
			return nil, nil
		}
		if removeProposalErr != nil {
			return identities, fmt.Errorf("skillauthoring: remove replayed proposal %q: %w", ref.Name, removeProposalErr)
		}
		return identities, nil
	}
	if _, statErr := root.Lstat(activeDir); statErr == nil {
		return nil, fmt.Errorf("%w: active path %q", skills.ErrConflict, ref.Name)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillauthoring: inspect active skill %q: %w", ref.Name, statErr)
	}
	if ensureManagedSkillCapacityErr := ensureManagedSkillCapacity(root); ensureManagedSkillCapacityErr != nil {
		return nil, ensureManagedSkillCapacityErr
	}
	if contextErrorErr := contextError(ctx, "approve proposal"); contextErrorErr != nil {
		return nil, contextErrorErr
	}
	if stageSkillErr := stageSkill(ctx, root, activeDir, content); stageSkillErr != nil {
		return nil, stageSkillErr
	}
	identities := s.skillIdentities(activeDir)
	removed, err := removeProposal(root, ref)
	if errors.Is(err, skills.ErrProposalChanged) {
		// A writer outside the governed Store lease replaced the current
		// proposal. The approved bytes are durable and those newer review bytes
		// must remain pending.
		return identities, nil
	}
	if removed {
		identities = append(identities, s.skillIdentities(s.proposalDir(ref))...)
	}
	if err != nil {
		return distinctPaths(identities), fmt.Errorf("skillauthoring: remove approved proposal %q: %w", ref.Name, err)
	}
	return distinctPaths(identities), nil
}

// proposalRevises reports whether staged content is marked as a revision of the
// active skill of the same name (frontmatter metadata revises: "true").
func proposalRevises(content []byte) (bool, error) {
	skill, err := skillspec.Parse(content)
	if err != nil {
		return false, fmt.Errorf("skillauthoring: parse proposal frontmatter: %w", err)
	}
	return skill.Metadata[metadataRevises] == metadataTrue, nil
}

// replaceActive installs a revising proposal as the active skill, archiving the
// version it supersedes. It OVERWRITES any older archived version of the same
// name — the single-slot history the module keeps by design (no per-version
// archive; that would be the semver theater the skill model rejects). An
// identical active skill is an idempotent no-op; a revision whose target has
// since vanished simply installs as the current version.
func (s *Store) replaceActive(ctx context.Context, root *os.Root, ref skills.ProposalRef, content []byte) ([]string, error) {
	activeDir := s.activeDir(ref.Name)
	active, exists, err := readSkill(root, activeDir)
	if err != nil {
		return nil, err
	}
	if exists && bytes.Equal(active, content) {
		removed, removeProposalErr := removeProposal(root, ref)
		identities := identitiesIf(removed, s.skillIdentities(s.proposalDir(ref)))
		if errors.Is(removeProposalErr, skills.ErrProposalChanged) {
			return nil, nil
		}
		if removeProposalErr != nil {
			return identities, fmt.Errorf("skillauthoring: remove replayed proposal %q: %w", ref.Name, removeProposalErr)
		}
		return identities, nil
	}
	if contextErrorErr := contextError(ctx, "replace skill"); contextErrorErr != nil {
		return nil, contextErrorErr
	}
	needsSlot := !exists
	if exists {
		if _, statErr := root.Lstat(s.archiveDir(ref.Name)); errors.Is(statErr, fs.ErrNotExist) {
			needsSlot = true
		} else if statErr != nil {
			return nil, fmt.Errorf("skillauthoring: inspect archived revision slot for %q: %w", ref.Name, statErr)
		}
	}
	if needsSlot {
		if ensureManagedSkillCapacityErr := ensureManagedSkillCapacity(root); ensureManagedSkillCapacityErr != nil {
			return nil, ensureManagedSkillCapacityErr
		}
	}
	var identities []string
	if exists {
		archived, archiveActiveErr := s.archiveActive(root, ref.Name)
		identities = append(identities, archived...)
		if archiveActiveErr != nil {
			return identities, archiveActiveErr
		}
	}
	if stageSkillErr := stageSkill(ctx, root, activeDir, content); stageSkillErr != nil {
		return identities, fmt.Errorf("skillauthoring: install revised skill %q: %w", ref.Name, stageSkillErr)
	}
	identities = append(identities, s.skillIdentities(activeDir)...)
	removed, err := removeProposal(root, ref)
	if errors.Is(err, skills.ErrProposalChanged) {
		return distinctPaths(identities), nil
	}
	if removed {
		identities = append(identities, s.skillIdentities(s.proposalDir(ref))...)
	}
	if err != nil {
		return distinctPaths(identities), fmt.Errorf("skillauthoring: remove revised proposal %q: %w", ref.Name, err)
	}
	return distinctPaths(identities), nil
}

// ListProposals enumerates the one current proposal for each name under
// _proposals/. The queue is bounded, so this complete, non-paginated read has a
// finite document and collection cost. Unparseable or path/name-mismatched
// content is skipped; an over-capacity queue fails closed instead of silently
// truncating the review surface. Ordering follows skill name.
func (s *Store) ListProposals(ctx context.Context) ([]skills.ProposalReview, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if err := contextError(ctx, "list proposals"); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "list proposals")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	names, err := proposalSlotNames(root)
	if err != nil {
		return nil, err
	}
	var out []skills.ProposalReview
	for _, name := range names {
		content, found, err := readSkill(root, filepath.Join(proposalsSubdir, name))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		skill, err := skillspec.Parse(content)
		if err != nil {
			continue
		}
		origin := skills.ProposalOrigin(skill.Metadata[metadataOrigin])
		if origin != "" && origin.Validate() != nil {
			continue
		}
		ref := skills.NewProposalRef(s.scope, skill.Name, content)
		if ref.Name != name {
			continue
		}
		out = append(out, skills.ProposalReview{
			Ref:           ref,
			Description:   skill.Description,
			Instructions:  skill.Instructions,
			Origin:        origin,
			SourceSession: skill.Metadata[metadataSourceSession],
			Revises:       skill.Metadata[metadataRevises] == metadataTrue,
		})
	}
	return out, nil
}

// proposalSlotNames reads at most the queue capacity plus one directory entry.
// Counting every entry (including malformed external additions) prevents junk
// from bypassing the finite scan; only valid named directories become reviews.
func proposalSlotNames(root *os.Root) ([]string, error) {
	directory, _, err := fileinput.OpenDirectoryAt(root, proposalsSubdir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list proposals: %w", err)
	}
	entries, readErr := directory.ReadDir(skills.MaxPendingProposalsPerScope + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("skillauthoring: list proposals: %w", errors.Join(readErr, closeErr))
	}
	if len(entries) > skills.MaxPendingProposalsPerScope {
		return nil, fmt.Errorf(
			"%w: scope contains more than %d entries",
			skills.ErrProposalQueueFull,
			skills.MaxPendingProposalsPerScope,
		)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && validName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// RejectProposal removes only the immutable proposal represented by handle and
// returns its changed public file identity. A missing proposal is already
// discarded; changed bytes are never deleted.
func (s *Store) RejectProposal(ctx context.Context, ref skills.ProposalRef) ([]string, error) {
	if err := s.validateRef(ref); err != nil {
		return nil, err
	}
	if err := contextError(ctx, "reject proposal"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "reject proposal")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	_, found, err := s.readProposal(root, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	removed, err := removeProposal(root, ref)
	identities := identitiesIf(removed, s.skillIdentities(s.proposalDir(ref)))
	if err != nil {
		return identities, fmt.Errorf("skillauthoring: reject proposal %q: %w", ref.Name, err)
	}
	return identities, nil
}

func (s *Store) validateRef(ref skills.ProposalRef) error {
	if !s.Enabled() {
		return errors.New("skillauthoring: no scoped skills root configured")
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("skillauthoring: invalid proposal reference: %w", err)
	}
	if ref.Scope != s.scope {
		return fmt.Errorf("skillauthoring: proposal scope %q does not match store scope %q", ref.Scope, s.scope)
	}
	if !validName(ref.Name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", ref.Name)
	}
	return nil
}

func (s *Store) proposalDir(ref skills.ProposalRef) string {
	return filepath.Join(proposalsSubdir, ref.Name)
}

func (s *Store) readProposal(root *os.Root, ref skills.ProposalRef) ([]byte, bool, error) {
	content, found, err := readSkill(root, s.proposalDir(ref))
	if err != nil || !found {
		return content, found, err
	}
	if !ref.Matches(content) {
		return nil, false, fmt.Errorf("%w: %q revision %q", skills.ErrProposalChanged, ref.Name, ref.Revision)
	}
	return content, true, nil
}
