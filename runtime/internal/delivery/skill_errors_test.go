package delivery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSkillDetailDistinguishesInvalidMissingAndUnavailableResources(t *testing.T) {
	root, user := t.TempDir(), t.TempDir()
	directory := filepath.Join(root, ".flame", "skills", "broken")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: secret-token\ndescription: [\n---\nbroken"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newWorkspaceHandlerWithConfig(root, workspaceTestConfig{Skills: promptsource.NewSkills(user)})
	for _, tt := range []struct {
		name        string
		want, cause error
	}{
		{"../outside", protocol.ErrInvalidParams, skills.ErrInvalidName},
		{"absent", protocol.ErrSkillNotFound, skills.ErrNotFound},
		{"broken", protocol.ErrSkillUnavailable, workspaceapp.ErrSkillUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.GetDiscoveredSkill(t.Context(), protocol.SkillDetailRequest{Workspace: protocol.WorkspaceRef{Path: root}, Name: tt.name})
			if !errors.Is(err, tt.want) || !errors.Is(err, tt.cause) {
				t.Fatalf("Get = %v; want %v and preserved %v", err, tt.want, tt.cause)
			}
			problem := ProjectError(err).Problem()
			if problem.Type != tt.want.Error() || strings.Contains(problem.Detail, "secret-token") || strings.Contains(problem.Detail, root) {
				t.Fatalf("unsafe or misclassified problem: %+v", problem)
			}
			if err := protocol.ValidateWireTree(problem); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSkillCurationReportsMissingResources(t *testing.T) {
	h := newWorkspaceHandlerWithConfig(t.TempDir(), workspaceTestConfig{Curator: emptySkillCurator{}})
	for name, operation := range map[string]func(context.Context, protocol.SkillNameRequest) error{"archive": h.ArchiveSkill, "restore": h.RestoreSkill} {
		t.Run(name, func(t *testing.T) {
			err := operation(t.Context(), protocol.SkillNameRequest{Name: "absent"})
			if !errors.Is(err, protocol.ErrSkillNotFound) || !errors.Is(err, skills.ErrNotFound) {
				t.Fatalf("operation = %v", err)
			}
		})
	}
}

func TestSkillWorkspaceFailuresMatchPublishedContract(t *testing.T) {
	workspace := protocol.WorkspaceRef{Path: filepath.Join(t.TempDir(), "missing")}
	proposal := protocol.SkillProposalRef{Workspace: workspace, Scope: protocol.SkillScopeProject, Name: "example", Revision: strings.Repeat("a", 64)}
	endpoint := mustNewEndpoint(t, newWorkspaceHandler(t.TempDir()), EndpointConfig{})
	for _, tc := range []struct {
		method Name
		params any
	}{
		{SkillsDiscoveredList, protocol.WorkspaceQuery{Workspace: workspace}},
		{SkillsDiscoveredGet, protocol.SkillDetailRequest{Workspace: workspace, Name: "example"}},
		{SkillsProposalsList, protocol.WorkspaceQuery{Workspace: workspace}},
		{SkillsProposalsApprove, proposal},
		{SkillsProposalsReject, proposal},
	} {
		t.Run(string(tc.method), func(t *testing.T) {
			result := endpoint.Invoke(t.Context(), tc.method, tc.params, Options{})
			if !errors.Is(result.Failure, protocol.ErrWorkspaceUnavailable) {
				t.Fatalf("failure = %v, want workspace unavailable", result.Failure)
			}
			method, _ := contract.lookup(tc.method)
			if !slices.Contains(method.Meta.Errors, result.Failure.Problem().Type) {
				t.Fatalf("returned %q is absent from the published method contract", result.Failure.Problem().Type)
			}
		})
	}
}
