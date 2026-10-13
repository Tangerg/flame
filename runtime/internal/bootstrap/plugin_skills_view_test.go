package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSkillsViewUsesCanonicalOwnersAndWithdrawalPreservesCuration(t *testing.T) {
	cfg := poMCPConfig(t)
	active := filepath.Join(cfg.SkillsUserDir, "verify", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(active), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(active, []byte("---\nname: verify\ndescription: Verify changes\n---\nRun the checks."), 0600); err != nil {
		t.Fatal(err)
	}
	library, err := skillauthoring.NewStore(cfg.SkillsUserDir, skills.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	proposal := skills.Proposal{Scope: skills.ScopeUser, Name: "review", Description: "Review changes", Instructions: "Inspect the important invariants.", Origin: skills.ProposalOriginRequested}
	ref, _, err := library.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	r := poMCPOpen(t, cfg)
	source, err := filepath.Abs("../../../plugins/skills")
	if err != nil {
		t.Fatal(err)
	}
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Views) != 1 || installed.Selected.Views[0].Type != protocol.PluginViewSkills {
		t.Fatalf("skills admission: %+v", installed.Selected)
	}
	view := protocol.ReadPluginViewRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ViewID: "skills"}
	query := protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}
	read := protocol.ReadPluginSkillsRequest{ReadPluginViewRequest: view, WorkspaceQuery: query}
	detail := protocol.ReadPluginSkillRequest{ReadPluginViewRequest: view, SkillDetailRequest: protocol.SkillDetailRequest{Workspace: query.Workspace, Name: "verify"}}
	checks := []struct {
		plugin, canonical delivery.Name
		scoped, original  any
	}{
		{delivery.PluginsReadSkills, delivery.SkillsDiscoveredList, read, query},
		{delivery.PluginsReadSkill, delivery.SkillsDiscoveredGet, detail, detail.SkillDetailRequest},
		{delivery.PluginsReadSkillLibrary, delivery.SkillsLibraryList, view, struct{}{}},
		{delivery.PluginsReadSkillProposals, delivery.SkillsProposalsList, read, query},
	}
	compare := func() {
		t.Helper()
		for _, check := range checks {
			if got, want := r.must(check.plugin, check.scoped, ""), r.must(check.canonical, check.original, ""); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s differs from its canonical query: %+v / %+v", check.plugin, got, want)
			}
		}
	}
	compare()
	authored, err := os.ReadFile(filepath.Join(source, "views", "skills.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.must(delivery.PluginsReadView, view, "").(*protocol.PluginViewResource); got.HTML != string(authored) {
		t.Fatal("skills resource differs from admitted bytes")
	}
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}}
	refuse := func(method delivery.Name, input any, want error) {
		t.Helper()
		if result := r.endpoint.Invoke(t.Context(), method, input, options); !errors.Is(result.Failure, want) {
			t.Fatalf("%s: %v, want %v", method, result.Failure, want)
		}
	}
	missing := detail
	missing.Name = "absent"
	refuse(delivery.PluginsReadSkill, missing, protocol.ErrSkillNotFound)
	refuse(delivery.PluginsReadUsage, protocol.ReadPluginUsageRequest{ReadPluginViewRequest: view}, protocol.ErrPluginNotFound)
	memorySource := filepath.Join(firstPartyPackages, "memory")
	memory := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: memorySource}, "install-memory").(*protocol.PluginInstallation)
	memoryView := protocol.ReadPluginViewRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: memory.ID, Digest: memory.Selected.Digest}, ViewID: "memory"}
	r.must(delivery.PluginsApprove, memoryView.PluginReleaseRequest, "approve-memory")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: memory.ID, Enabled: true}, "enable-memory")
	wrong := read
	wrong.ReadPluginViewRequest = memoryView
	refuse(delivery.PluginsReadSkills, wrong, protocol.ErrPluginNotFound)
	r.must(delivery.SkillsLibraryArchive, protocol.SkillNameRequest{Name: "verify"}, "archive")
	refuse(delivery.PluginsReadSkill, detail, protocol.ErrSkillNotFound)
	if got, want := r.must(delivery.PluginsReadSkillLibrary, view, ""), r.must(delivery.SkillsLibraryList, struct{}{}, ""); !reflect.DeepEqual(got, want) {
		t.Fatal("archived library differs from canonical query")
	}
	r.must(delivery.SkillsLibraryRestore, protocol.SkillNameRequest{Name: "verify"}, "restore")
	proposal.Instructions = "Read the revised invariants before publication."
	current, _, err := library.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	compare()
	stale := protocol.SkillProposalRef{Workspace: query.Workspace, Scope: protocol.SkillProposalScopeUser, Name: ref.Name, Revision: ref.Revision}
	refuse(delivery.SkillsProposalsApprove, stale, protocol.ErrRevisionConflict)
	stale.Revision = current.Revision
	r.must(delivery.SkillsProposalsApprove, stale, "approve-current")
	compare()
	r.close()
	r = poMCPOpen(t, cfg)
	compare()
	r.must(delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: installed.ID}, "revoke")
	for _, check := range checks {
		refuse(check.plugin, check.scoped, protocol.ErrPluginUnapproved)
	}
	before := r.must(delivery.SkillsLibraryList, struct{}{}, "")
	proposal.Name = "pending"
	if _, _, err := library.SubmitProposal(t.Context(), proposal); err != nil {
		t.Fatal(err)
	}
	pending := r.must(delivery.SkillsProposalsList, query, "")
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, "uninstall")
	if after := r.must(delivery.SkillsLibraryList, struct{}{}, ""); !reflect.DeepEqual(before, after) {
		t.Fatal("uninstall advanced the Skill library")
	}
	if proposals := r.must(delivery.SkillsProposalsList, query, ""); !reflect.DeepEqual(proposals, pending) {
		t.Fatal("uninstall changed the pending proposal")
	}
	for _, check := range checks {
		refuse(check.plugin, check.scoped, protocol.ErrPluginNotFound)
	}
}
