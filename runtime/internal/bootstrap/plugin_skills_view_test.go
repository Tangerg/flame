package bootstrap

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSkillsComponentUsesCanonicalOwnersAndWithdrawalPreservesCuration(t *testing.T) {
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
	source, err := filepath.Abs(filepath.Join(firstPartyPackages, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	installed := poMCPActivate(r, source)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 1 || installed.Selected.Components[0].Scope != "client" {
		t.Fatalf("skills component admission: %+v", installed.Selected)
	}
	component := installed.Selected.Components[0]
	resource := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: component.ID}, Path: component.Entry}
	query := protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}
	detail := protocol.SkillDetailRequest{Workspace: query.Workspace, Name: "verify"}
	inspect := func() {
		t.Helper()
		r.must(delivery.SkillsDiscoveredList, query, "")
		r.must(delivery.SkillsDiscoveredGet, detail, "")
		r.must(delivery.SkillsLibraryList, struct{}{}, "")
		r.must(delivery.SkillsProposalsList, query, "")
	}
	inspect()
	authored, err := os.ReadFile(filepath.Join(source, component.Entry))
	if err != nil {
		t.Fatal(err)
	}
	projected := r.must(delivery.PluginsReadResource, resource, "").(*protocol.PluginResource)
	body, err := base64.StdEncoding.DecodeString(projected.Body)
	if err != nil || string(body) != string(authored) {
		t.Fatalf("skills component bytes: %v", err)
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
	refuse(delivery.SkillsDiscoveredGet, missing, protocol.ErrSkillNotFound)
	r.must(delivery.SkillsLibraryArchive, protocol.SkillNameRequest{Name: "verify"}, "archive")
	refuse(delivery.SkillsDiscoveredGet, detail, protocol.ErrSkillNotFound)
	archived := r.must(delivery.SkillsLibraryList, struct{}{}, "").(*protocol.Page[protocol.ManagedSkill])
	foundArchived := false
	for _, item := range archived.Data {
		if item.Name == "verify" && item.Lifecycle == "archived" {
			foundArchived = true
		}
	}
	if !foundArchived {
		t.Fatal("canonical library did not retain the archived Skill")
	}

	r.must(delivery.SkillsLibraryRestore, protocol.SkillNameRequest{Name: "verify"}, "restore")
	proposal.Instructions = "Read the revised invariants before publication."
	current, _, err := library.SubmitProposal(t.Context(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	inspect()
	stale := protocol.SkillProposalRef{Workspace: query.Workspace, Scope: protocol.SkillProposalScopeUser, Name: ref.Name, Revision: ref.Revision}
	refuse(delivery.SkillsProposalsApprove, stale, protocol.ErrRevisionConflict)
	stale.Revision = current.Revision
	r.must(delivery.SkillsProposalsApprove, stale, "approve-current")
	inspect()
	r.close()
	r = poMCPOpen(t, cfg)
	inspect()
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	refuse(delivery.PluginsReadResource, resource, protocol.ErrPluginDisabled)
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
	refuse(delivery.PluginsReadResource, resource, protocol.ErrPluginNotFound)

}
