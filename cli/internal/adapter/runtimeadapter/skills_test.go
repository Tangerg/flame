package runtimeadapter

import (
	"context"
	"testing"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimeprofile"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

const skillRevision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type skillBindingStub struct {
	t       *testing.T
	actions []string
}

func (s *skillBindingStub) ListDiscoveredSkills(_ context.Context, request protocol.WorkspaceQuery, options flameruntime.CallOptions) (*protocol.Page[protocol.Skill], error) {
	s.assertCall(request.Workspace.Path, options.RequestMeta)
	return protocol.NewPage([]protocol.Skill{{Name: "release-checks", Description: "Release safely", Scope: protocol.SkillScopeProject}}), nil
}

func (s *skillBindingStub) ListManagedSkills(_ context.Context, options flameruntime.CallOptions) (*protocol.Page[protocol.ManagedSkill], error) {
	s.assertMeta(options.RequestMeta)
	return protocol.NewPage([]protocol.ManagedSkill{{Name: "review", Description: "Review code", Lifecycle: protocol.SkillLifecycleArchived}}), nil
}

func (s *skillBindingStub) ListSkillProposals(_ context.Context, request protocol.WorkspaceQuery, options flameruntime.CallOptions) (*protocol.Page[protocol.SkillProposal], error) {
	s.assertCall(request.Workspace.Path, options.RequestMeta)
	return protocol.NewPage([]protocol.SkillProposal{{
		Name: "release-checks", Revision: skillRevision, Scope: protocol.SkillScopeUser,
		Description: "Release safely", Instructions: "Run every release gate.",
		Origin: protocol.SkillProposalOriginRequested, SourceSession: "ses_1",
	}}), nil
}

func (s *skillBindingStub) ArchiveSkill(_ context.Context, request protocol.SkillNameRequest, options flameruntime.CommandOptions) error {
	s.assertCommand("archive:"+request.Name, options)
	return nil
}

func (s *skillBindingStub) RestoreSkill(_ context.Context, request protocol.SkillNameRequest, options flameruntime.CommandOptions) error {
	s.assertCommand("restore:"+request.Name, options)
	return nil
}

func (s *skillBindingStub) ApproveSkillProposal(_ context.Context, request protocol.SkillProposalRef, options flameruntime.CommandOptions) error {
	s.assertReference("approve", request, options)
	return nil
}

func (s *skillBindingStub) RejectSkillProposal(_ context.Context, request protocol.SkillProposalRef, options flameruntime.CommandOptions) error {
	s.assertReference("reject", request, options)
	return nil
}

func (s *skillBindingStub) assertCall(workspace string, meta protocol.RequestMeta) {
	s.t.Helper()
	if workspace != "/workspace" {
		s.t.Fatalf("skill call = workspace %q, meta %+v", workspace, meta)
	}
	s.assertMeta(meta)
}

func (s *skillBindingStub) assertMeta(meta protocol.RequestMeta) {
	s.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		s.t.Fatalf("skill call meta = %+v", meta)
	}
}

func (s *skillBindingStub) assertCommand(action string, options flameruntime.CommandOptions) {
	s.t.Helper()
	if options.IdempotencyKey == "" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
		s.t.Fatalf("skill command options = %+v", options)
	}
	s.actions = append(s.actions, action)
}

func (s *skillBindingStub) assertReference(action string, request protocol.SkillProposalRef, options flameruntime.CommandOptions) {
	s.t.Helper()
	s.assertCommand(action+":"+string(request.Scope)+"/"+request.Name, options)
	if request.Workspace.Path != "/workspace" || request.Revision != skillRevision {
		s.t.Fatalf("skill proposal reference = %+v", request)
	}
}

func TestSkillAdapterProjectsCatalogsAndExactMutationReferences(t *testing.T) {
	stub := &skillBindingStub{t: t}
	runtime := &Connection{skills: stub, meta: requestMeta("test")}
	discovered, err := runtime.Discover(t.Context(), "/workspace")
	if err != nil || len(discovered) != 1 || discovered[0].Key() != "project/release-checks" {
		t.Fatalf("Discover = (%+v, %v)", discovered, err)
	}
	managed, err := runtime.Managed(t.Context())
	if err != nil || len(managed) != 1 || managed[0].Lifecycle != workspace.SkillArchived {
		t.Fatalf("Managed = (%+v, %v)", managed, err)
	}
	proposals, err := runtime.Proposals(t.Context(), "/workspace")
	if err != nil || len(proposals) != 1 || proposals[0].Key() != "user/release-checks@0123456789ab" {
		t.Fatalf("Proposals = (%+v, %v)", proposals, err)
	}
	if archiveErr := runtime.Archive(t.Context(), "review"); archiveErr != nil {
		t.Fatal(archiveErr)
	}
	if restoreErr := runtime.Restore(t.Context(), "review"); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	reference, err := proposals[0].Reference("/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Approve(t.Context(), reference); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reject(t.Context(), reference); err != nil {
		t.Fatal(err)
	}
	want := []string{"archive:review", "restore:review", "approve:user/release-checks", "reject:user/release-checks"}
	if len(stub.actions) != len(want) {
		t.Fatalf("actions = %v, want %v", stub.actions, want)
	}
	for index := range want {
		if stub.actions[index] != want[index] {
			t.Fatalf("actions = %v, want %v", stub.actions, want)
		}
	}
}

func TestConnectionProfileControlsOptionalAdapterAvailability(t *testing.T) {
	runtime := &Connection{profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
		runtimeprofile.FeatureSkills: {Enabled: true}, runtimeprofile.FeatureMCP: {Enabled: true},
		runtimeprofile.FeatureSchedules: {Enabled: true}, runtimeprofile.FeatureAgentMemory: {Enabled: true},
		runtimeprofile.FeatureKnowledge: {Enabled: true}, runtimeprofile.FeatureSessionExport: {Enabled: true},
	}}}
	profile := runtime.Profile()
	if !profile.Supports(runtimeprofile.FeatureSkills) || !profile.Supports(runtimeprofile.FeatureMCP) ||
		!profile.Supports(runtimeprofile.FeatureSchedules) || profile.Supports(runtimeprofile.FeatureGoals) {
		t.Fatalf("profile features = %+v", profile.Features)
	}
	if runtime.AgentMemoryService() == nil || runtime.KnowledgeService() == nil {
		t.Fatal("advertised context adapters were not exposed")
	}
	runtime.profile.Features[runtimeprofile.FeatureAgentMemory] = runtimeprofile.Feature{}
	if runtime.AgentMemoryService() != nil {
		t.Fatal("unadvertised agent memory exposed an adapter")
	}
}
