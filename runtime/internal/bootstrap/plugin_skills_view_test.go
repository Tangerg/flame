package bootstrap

import (
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

func TestSkillsComponentUsesCanonicalOwnersAndWithdrawalPreservesCuration(t *testing.T) {
	cfg := poMCPConfig(t)
	active := filepath.Join(cfg.skillsUserDir, "verify", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(active), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(active, []byte("---\nname: verify\ndescription: Verify changes\n---\nRun the checks."), 0600); err != nil {
		t.Fatal(err)
	}
	r := poMCPOpen(t, cfg)
	source, err := filepath.Abs(filepath.Join(firstPartyPackages, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	installed := poMCPActivate(r, source)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Components) != 1 || installed.Selected.Components[0].Scope != "client" {
		t.Fatalf("skills component admission: %+v", installed.Selected)
	}
	submit := func(key string, p skills.Proposal) skills.ProposalRef {
		t.Helper()
		app := r.host.application.delivery.WorkspaceSkills.(*workspace.Skills)
		executable, err := builtin.NewProposal(app)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := toolcontract.Bind(executable)
		if err != nil {
			t.Fatal(err)
		}
		args, err := json.Marshal(struct {
			Name         string       `json:"name"`
			Description  string       `json:"description"`
			Instructions string       `json:"instructions"`
			Scope        skills.Scope `json:"scope"`
		}{p.Name, p.Description, p.Instructions, p.Scope})
		if err != nil {
			t.Fatal(err)
		}
		invocation, err := bound.Contract().Prepare(chat.ToolCall{ID: key, Name: bound.Contract().Definition().Name, Arguments: string(args)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		ctx := executionctx.WithScope(operation.Start(t.Context(), extension.Model, "test-agent", key), runs.ExecutionScope{SessionID: "ses_review", CWD: cfg.defaultWorkspacePath, WorkspaceCWD: cfg.defaultWorkspacePath})
		result, err := bound.Call(ctx, invocation)
		if err != nil {
			t.Fatal(err)
		}
		text, ok := result.Output.Text()
		if !ok {
			t.Fatal("agent received no pending reference")
		}
		var ref skills.ProposalRef
		if err := json.Unmarshal([]byte(text), &ref, json.MatchCaseInsensitiveNames(true)); err != nil {
			t.Fatal(err)
		}
		if err := ref.Validate(); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	proposal := skills.Proposal{Scope: skills.ScopeUser, Name: "review", Description: "Review changes", Instructions: "Inspect the important invariants.", Origin: skills.ProposalOriginRequested}
	ref := submit("initial", proposal)
	component := installed.Selected.Components[0]
	resource := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, ComponentID: component.ID}, Path: component.Entry}
	query := protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}}
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
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}}
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
	current := submit("replacement", proposal)
	inspect()
	stale := protocol.SkillProposalRef{Workspace: query.Workspace, Scope: protocol.SkillProposalScopeUser, Name: ref.Name, Revision: ref.Revision}
	refuse(delivery.SkillsProposalsApprove, stale, protocol.ErrRevisionConflict)
	stale.Revision = current.Revision
	r.must(delivery.SkillsProposalsApprove, stale, "approve-current")
	inspect()
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	inspect()
	proposal.Name = "pending"
	outstanding := submit("pending", proposal)
	outstandingRef := protocol.SkillProposalRef{Workspace: query.Workspace, Name: outstanding.Name, Revision: outstanding.Revision, Scope: protocol.SkillProposalScopeUser}
	other := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-alternative").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: other.ID, Enabled: true}, "enable-alternative")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: skilllearning.Reference, InstallationID: other.ID, Contribution: "skill-learning"}, "select-alternative")
	waitCallContributorState(t, r, other.ID, protocol.MCPServerConnected)
	proposal.Instructions = "Only the replacement provider owns these instructions."
	replacement := submit("other-decision", proposal)
	originals := r.must(delivery.SkillsProposalsList, query, "").(*protocol.Page[protocol.SkillProposal])
	if len(originals.Data) != 2 || replacement == outstanding {
		t.Fatalf("source switch merged independent proposals: %+v", originals)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	refuse(delivery.PluginsReadResource, resource, protocol.ErrPluginDisabled)
	if result := r.endpoint.Invoke(t.Context(), delivery.SkillsProposalsApprove, outstandingRef, options); result.Failure == nil {
		t.Fatal("disabled release published a proposal")
	}
	before := r.must(delivery.SkillsLibraryList, struct{}{}, "")
	pending := r.must(delivery.SkillsProposalsList, query, "")
	r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, "uninstall")
	if after := r.must(delivery.SkillsLibraryList, struct{}{}, ""); !reflect.DeepEqual(before, after) {
		t.Fatal("uninstall advanced the Skill library")
	}
	if proposals := r.must(delivery.SkillsProposalsList, query, ""); !reflect.DeepEqual(proposals, pending) {
		t.Fatal("uninstall changed the pending proposal")
	}
	if result := r.endpoint.Invoke(t.Context(), delivery.SkillsProposalsApprove, outstandingRef, options); result.Failure == nil {
		t.Fatal("uninstalled source was retargeted to an enabled replacement")
	}
	replacementRef := outstandingRef
	replacementRef.Revision = replacement.Revision
	r.must(delivery.SkillsProposalsApprove, replacementRef, "approve-other")
	activeBytes, err := os.ReadFile(filepath.Join(cfg.skillsUserDir, proposal.Name, "SKILL.md"))
	if err != nil || !strings.Contains(string(activeBytes), proposal.Instructions) {
		t.Fatalf("replacement did not publish its own input: %s, %v", activeBytes, err)
	}
	refuse(delivery.PluginsReadResource, resource, protocol.ErrPluginNotFound)

}
