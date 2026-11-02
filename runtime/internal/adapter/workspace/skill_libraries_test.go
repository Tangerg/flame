package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestPublishUsesOnlyAdmittedScopedLibrary(t *testing.T) {
	user := t.TempDir()
	project := t.TempDir()
	store, err := skillauthoring.NewStore(user, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := workspaceadapter.NewSkillLibraries(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []skills.Scope{skills.ScopeProject, skills.ScopeUser} {
		p := proposal(scope, "verified-name")
		base, err := libraries.Preimage(t.Context(), project, p)
		if err != nil {
			t.Fatal(err)
		}
		permit := testsupport.Digest(string(scope)).String()
		if _, err := libraries.Publish(t.Context(), project, p, base, permit); err != nil {
			t.Fatal(err)
		}
	}
	assertFile(t, filepath.Join(user, "verified-name", "SKILL.md"))
	assertFile(t, filepath.Join(promptsource.ProjectSkillDir(project), "verified-name", "SKILL.md"))
}

func TestPublishRefusesPermitForAnotherLibrary(t *testing.T) {
	user := t.TempDir()
	project := t.TempDir()
	store, err := skillauthoring.NewStore(user, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := workspaceadapter.NewSkillLibraries(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	approved := proposal(skills.ScopeProject, "verified-name")
	previous, err := libraries.Preimage(t.Context(), project, approved)
	if err != nil {
		t.Fatal(err)
	}
	permit := testsupport.Digest(t.Name()).String()
	if _, err := libraries.Publish(t.Context(), project, approved, previous, permit); err != nil {
		t.Fatal(err)
	}
	retargeted := proposal(skills.ScopeUser, approved.Name)
	previous, err = libraries.Preimage(t.Context(), project, retargeted)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := libraries.Publish(t.Context(), project, retargeted, previous, permit)
	if !errors.Is(err, skills.ErrProposalChanged) || len(changed) != 0 {
		t.Fatalf("retargeted publication = %v, %v, want the original permit refused", changed, err)
	}
	if _, err := os.Stat(filepath.Join(user, approved.Name, "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unapproved library content = %v, want no published file", err)
	}
	assertFile(t, filepath.Join(promptsource.ProjectSkillDir(project), approved.Name, "SKILL.md"))
}

func TestPublishRejectsLibraryOutsideProject(t *testing.T) {
	user := t.TempDir()
	project := t.TempDir()
	outside := t.TempDir()
	store, err := skillauthoring.NewStore(user, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := workspaceadapter.NewSkillLibraries(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := promptsource.ProjectSkillDir(project)
	if err := os.MkdirAll(filepath.Dir(lib), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, lib); err != nil {
		t.Skip(err)
	}
	if _, err := libraries.Preimage(t.Context(), project, proposal(skills.ScopeProject, "escaped")); err == nil {
		t.Fatal("project symlink escaped trusted root")
	}
}
func proposal(scope skills.Scope, name string) skills.Proposal {
	return skills.Proposal{
		Scope:        scope,
		Name:         name,
		Description:  "A reusable workflow with enough detail for Skill validation.",
		Instructions: "Follow the reusable workflow exactly.",
		Origin:       skills.ProposalOriginRequested,
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func TestSkillLibrariesRequireUserStore(t *testing.T) {
	if libraries, err := workspaceadapter.NewSkillLibraries(nil, t.TempDir()); err == nil || libraries != nil {
		t.Fatalf("NewSkillLibraries = (%v, %v), want incomplete construction rejected", libraries, err)
	}
}
