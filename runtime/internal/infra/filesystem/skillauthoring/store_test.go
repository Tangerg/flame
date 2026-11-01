package skillauthoring_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

const (
	archiveSubdir = "_archive"
)

func newStore(t *testing.T, root string, scope skills.Scope) *skillauthoring.Store {
	t.Helper()
	store, err := skillauthoring.NewStore(root, scope)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStoreRequiresLibraryRootAndScope(t *testing.T) {
	for _, test := range []struct {
		name  string
		root  string
		scope skills.Scope
	}{
		{name: "missing root", scope: skills.ScopeUser},
		{name: "relative root", root: "skills", scope: skills.ScopeUser},
		{name: "missing scope", root: t.TempDir()},
		{name: "invalid scope", root: t.TempDir(), scope: skills.Scope("invalid")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if store, err := skillauthoring.NewStore(test.root, test.scope); err == nil || store != nil {
				t.Fatalf("NewStore = (%v, %v), want construction failure", store, err)
			}
		})
	}
	for _, scope := range []skills.Scope{skills.ScopeUser, skills.ScopeProject} {
		store := newStore(t, filepath.Join(t.TempDir(), "skills"), scope)
		proposals, err := store.List(t.Context())
		if err != nil || len(proposals) != 0 {
			t.Fatalf("empty %s library = (%+v, %v), want an empty proposal list", scope, proposals, err)
		}
	}
}

func publish(t *testing.T, store *skillauthoring.Store, p skills.Proposal) ([]string, error) {
	t.Helper()
	base, err := store.Preimage(t.Context(), p)
	if err != nil {
		return nil, err
	}
	return store.Publish(t.Context(), p, base, fmt.Sprintf("%x", sha256.Sum256([]byte(p.Name+" "+p.Instructions))), t.TempDir())
}
func approve(t *testing.T, store *skillauthoring.Store, name string) {
	t.Helper()
	installActive(t, store, name, "Follow the approved instructions.")
}

func lifecycleOf(entries []skills.Entry, name string) (skills.Lifecycle, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e.Lifecycle, true
		}
	}
	return "", false
}

func TestArchiveRestoreAndList(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	approve(t, store, "alpha-skill")
	approve(t, store, "beta-skill")

	// Both active.
	list, err := store.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if lc, ok := lifecycleOf(list, "alpha-skill"); !ok || lc != skills.Active {
		t.Fatalf("alpha should be active, got %q (%v)", lc, ok)
	}

	// Archive alpha → it leaves the active set, still listed as archived, and is
	// no longer discovered by the read-only source (not loadable).
	archived, err := store.Archive(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(archived) != 2 {
		t.Fatalf("Archive identities = %v, want active and archive files", archived)
	}
	replayedArchive, err := store.Archive(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("replayed Archive: %v", err)
	}
	if len(replayedArchive) != 0 {
		t.Fatalf("replayed Archive identities = %v, want none", replayedArchive)
	}
	list, _ = store.List(t.Context())
	if lc, _ := lifecycleOf(list, "alpha-skill"); lc != skills.Archived {
		t.Fatalf("alpha should be archived, got %q", lc)
	}
	if _, statErr := os.Stat(filepath.Join(root, "alpha-skill", "SKILL.md")); !os.IsNotExist(statErr) {
		t.Fatal("archived skill must leave the active directory")
	}
	if _, statErr := os.Stat(filepath.Join(root, "_archive", "alpha-skill", "SKILL.md")); statErr != nil {
		t.Fatalf("archived skill must be preserved under _archive: %v", statErr)
	}

	// Restore → active again.
	restored, err := store.Restore(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(restored) != 2 {
		t.Fatalf("Restore identities = %v, want archive and active files", restored)
	}
	replayedRestore, err := store.Restore(t.Context(), "alpha-skill")
	if err != nil {
		t.Fatalf("replayed Restore: %v", err)
	}
	if len(replayedRestore) != 0 {
		t.Fatalf("replayed Restore identities = %v, want none", replayedRestore)
	}
	list, _ = store.List(t.Context())
	if lc, _ := lifecycleOf(list, "alpha-skill"); lc != skills.Active {
		t.Fatalf("restored alpha should be active, got %q", lc)
	}
}

func TestManagedLibraryListRejectsOverCapacitySnapshot(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	for index := range skills.MaxSkillsPerSource + 1 {
		writeActiveSkillFixture(t, root, fmt.Sprintf("skill-%03d", index))
	}

	if _, err := store.List(t.Context()); !errors.Is(err, skills.ErrLibraryCapacity) {
		t.Fatalf("List error = %v, want ErrLibraryCapacity beyond %d managed Skills", err, skills.MaxSkillsPerSource)
	}
}

func TestArchiveMissingErrors(t *testing.T) {
	store := newStore(t, t.TempDir(), skills.ScopeUser)
	if _, err := store.Archive(t.Context(), "nope"); err == nil {
		t.Fatal("archiving a nonexistent skill must error")
	}
	if _, err := store.Restore(t.Context(), "nope"); err == nil {
		t.Fatal("restoring a nonexistent archived skill must error")
	}
}

func TestApproveProposalCannotExceedManagedLibraryCapacity(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	for index := range skills.MaxSkillsPerSource {
		writeActiveSkillFixture(t, root, fmt.Sprintf("skill-%03d", index))
	}
	proposal := skills.Proposal{
		Origin: skills.ProposalOriginRequested,
		Scope:  skills.ScopeUser, Name: "overflow",
		Description:  "A new Skill must not make the managed library unlistable.",
		Instructions: "instructions",
	}
	if _, err := publish(t, store, proposal); !errors.Is(err, skills.ErrLibraryCapacity) {
		t.Fatalf("ApproveProposal error = %v, want ErrLibraryCapacity", err)
	}
	if _, err := os.Stat(filepath.Join(root, proposal.Name, "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capacity rejection published an active Skill: %v", err)
	}
}

func TestApproveRevisionNeedsCapacityForFirstArchiveSlot(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	for index := range skills.MaxSkillsPerSource {
		writeActiveSkillFixture(t, root, fmt.Sprintf("skill-%03d", index))
	}
	proposal := skills.Proposal{
		Scope: skills.ScopeUser, Name: "skill-000",
		Description:  "A revision cannot hide an extra archived slot beyond capacity.",
		Instructions: "revised instructions",
		Origin:       skills.ProposalOriginMined, Revises: true,
	}
	if _, err := publish(t, store, proposal); !errors.Is(err, skills.ErrLibraryCapacity) {
		t.Fatalf("ApproveProposal(revision) error = %v, want ErrLibraryCapacity", err)
	}
	if _, err := os.Stat(filepath.Join(root, archiveSubdir, proposal.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capacity rejection created an archive slot: %v", err)
	}
}

func TestLifecycleConflictsPreserveBothStates(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root, skills.ScopeUser)
	approve(t, store, "conflict-safe")
	if _, err := store.Archive(t.Context(), "conflict-safe"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	proposal := skills.Proposal{Origin: skills.ProposalOriginRequested, Scope: skills.ScopeUser, Name: "conflict-safe", Description: "A different version must not replace the archive.", Instructions: "replacement"}
	if _, err := publish(t, store, proposal); !errors.Is(err, skills.ErrConflict) {
		t.Fatalf("ApproveProposal() error = %v, want ErrConflict", err)
	}
	if _, err := os.Stat(filepath.Join(root, archiveSubdir, proposal.Name, "SKILL.md")); err != nil {
		t.Fatalf("archived version was lost: %v", err)
	}
}
