package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"strings"
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
		if _, err := libraries.Publish(t.Context(), project, p, base, strings.Repeat("c", 64)); err != nil {
			t.Fatal(err)
		}
	}
	assertFile(t, filepath.Join(user, "verified-name", "SKILL.md"))
	assertFile(t, filepath.Join(promptsource.ProjectSkillDir(project), "verified-name", "SKILL.md"))
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
