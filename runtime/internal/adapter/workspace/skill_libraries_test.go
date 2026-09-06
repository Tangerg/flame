package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

func TestProjectSkillsRemainAvailableWithoutUserLibrary(t *testing.T) {
	projectRoot := t.TempDir()
	scope, err := workspaceapp.NewScope(projectRoot, "", workspaceadapter.Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	libraries := workspaceadapter.NewSkillLibraries(nil)
	useCases, err := workspaceapp.NewSkills(scope, promptsource.NewWorkspaceSkills(""), nil, libraries, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := useCases.Managed(t.Context()); !errors.Is(err, workspaceapp.ErrSkillLibraryUnavailable) {
		t.Fatalf("Managed = %v, want user-library curation disabled", err)
	}
	if _, err := useCases.SubmitProposal(t.Context(), projectRoot, proposal(skills.ScopeUser, "personal-check")); err == nil {
		t.Fatal("user proposal was accepted without a user library")
	}
	ref, err := useCases.SubmitProposal(t.Context(), projectRoot, proposal(skills.ScopeProject, "project-check"))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := useCases.Proposals(t.Context(), projectRoot)
	if err != nil || len(pending) != 1 || pending[0].Ref != ref {
		t.Fatalf("Proposals = (%+v, %v), want submitted project proposal", pending, err)
	}
	if err := useCases.ApproveProposal(t.Context(), projectRoot, ref); err != nil {
		t.Fatal(err)
	}
	visible, err := useCases.List(t.Context(), projectRoot)
	if err != nil || len(visible) != 1 || visible[0].Name != ref.Name || visible[0].Scope != workspaceapp.SkillScopeProject {
		t.Fatalf("List = (%+v, %v), want approved project skill", visible, err)
	}
}

func TestSkillLibrariesRouteProposalsByScope(t *testing.T) {
	userRoot := filepath.Join(t.TempDir(), "user-skills")
	projectRoot := filepath.Join(t.TempDir(), "project")
	store, err := skillauthoring.NewStore(userRoot, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	libraries := workspaceadapter.NewSkillLibraries(store)

	projectProposal := proposal(skills.ScopeProject, "project-check")
	projectRef, _, err := libraries.SubmitProposal(t.Context(), projectRoot, projectProposal)
	if err != nil {
		t.Fatalf("SubmitProposal(project): %v", err)
	}
	userProposal := proposal(skills.ScopeUser, "personal-check")
	userRef, _, err := libraries.SubmitProposal(t.Context(), projectRoot, userProposal)
	if err != nil {
		t.Fatalf("SubmitProposal(user): %v", err)
	}

	got, err := libraries.ListProposals(t.Context(), projectRoot)
	if err != nil {
		t.Fatalf("ListProposals: %v", err)
	}
	if len(got) != 2 || got[0].Ref != projectRef || got[1].Ref != userRef {
		t.Fatalf("ListProposals = %+v; want project then user", got)
	}

	if _, err := libraries.ApproveProposal(t.Context(), projectRoot, projectRef); err != nil {
		t.Fatalf("ApproveProposal(project): %v", err)
	}
	if _, err := libraries.ApproveProposal(t.Context(), projectRoot, userRef); err != nil {
		t.Fatalf("ApproveProposal(user): %v", err)
	}
	assertFile(t, filepath.Join(promptsource.ProjectSkillDir(projectRoot), projectRef.Name, "SKILL.md"))
	assertFile(t, filepath.Join(userRoot, userRef.Name, "SKILL.md"))
}

func TestSkillLibrariesRejectProposalFromItsScopedStore(t *testing.T) {
	userRoot := filepath.Join(t.TempDir(), "user-skills")
	projectRoot := filepath.Join(t.TempDir(), "project")
	store, err := skillauthoring.NewStore(userRoot, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	libraries := workspaceadapter.NewSkillLibraries(store)
	ref, _, err := libraries.SubmitProposal(t.Context(), projectRoot, proposal(skills.ScopeProject, "throwaway"))
	if err != nil {
		t.Fatal(err)
	}
	if _, rejectProposalErr := libraries.RejectProposal(t.Context(), projectRoot, ref); rejectProposalErr != nil {
		t.Fatalf("RejectProposal: %v", rejectProposalErr)
	}
	got, err := libraries.ListProposals(t.Context(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("rejected proposal still listed: %+v", got)
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
