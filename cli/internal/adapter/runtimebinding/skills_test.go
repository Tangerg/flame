package runtimebinding

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

const skillRevision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const otherSkillRevision = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

type skillBindingStub struct {
	t       *testing.T
	actions []string
}

type invalidSkillBindingStub struct {
	*skillBindingStub
	discovered *protocol.Page[protocol.Skill]
	managed    *protocol.Page[protocol.ManagedSkill]
	proposals  *protocol.Page[protocol.SkillProposal]
}

func (s *invalidSkillBindingStub) ListDiscoveredSkills(_ context.Context, request protocol.WorkspaceQuery, options flameruntime.CallOptions) (*protocol.Page[protocol.Skill], error) {
	s.assertCall(request.Workspace.Path, options.RequestMeta)
	return s.discovered, nil
}

func (s *invalidSkillBindingStub) ListManagedSkills(_ context.Context, options flameruntime.CallOptions) (*protocol.Page[protocol.ManagedSkill], error) {
	s.assertMeta(options.RequestMeta)
	return s.managed, nil
}

func (s *invalidSkillBindingStub) ListSkillProposals(_ context.Context, request protocol.WorkspaceQuery, options flameruntime.CallOptions) (*protocol.Page[protocol.SkillProposal], error) {
	s.assertCall(request.Workspace.Path, options.RequestMeta)
	return s.proposals, nil
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
	if err != nil || len(discovered) != 1 || workspace.DiscoveredSkillKey(discovered[0]) != "project/release-checks" {
		t.Fatalf("Discover = (%+v, %v)", discovered, err)
	}
	managed, err := runtime.Managed(t.Context())
	if err != nil || len(managed) != 1 || managed[0].Lifecycle != protocol.SkillLifecycleArchived {
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

func TestSkillAdapterClonesDirectProtocolCatalogs(t *testing.T) {
	discoveredPage := protocol.NewPage([]protocol.Skill{{
		Name: "release-checks", Description: "Release safely", Scope: protocol.SkillScopeProject,
	}})
	managedPage := protocol.NewPage([]protocol.ManagedSkill{{
		Name: "review", Description: "Review code", Lifecycle: protocol.SkillLifecycleActive,
	}})
	stub := &invalidSkillBindingStub{
		skillBindingStub: &skillBindingStub{t: t}, discovered: discoveredPage, managed: managedPage,
	}
	runtime := &Connection{skills: stub, meta: requestMeta("test")}
	discovered, err := runtime.Discover(t.Context(), "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	managed, err := runtime.Managed(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	discoveredPage.Data[0].Description = "mutated"
	managedPage.Data[0].Description = "mutated"
	if discovered[0].Description != "Release safely" || managed[0].Description != "Review code" {
		t.Fatal("skill projections alias runtime catalog storage")
	}
}

func TestSkillAdapterRejectsInvalidWireValues(t *testing.T) {
	for _, test := range []struct {
		name string
		stub *invalidSkillBindingStub
		read func(*Connection) error
	}{
		{
			name: "blank discovered name",
			stub: &invalidSkillBindingStub{discovered: protocol.NewPage([]protocol.Skill{{Scope: protocol.SkillScopeProject}})},
			read: func(runtime *Connection) error {
				_, err := runtime.Discover(t.Context(), "/workspace")
				return err
			},
		}, {
			name: "repeated discovered identity",
			stub: &invalidSkillBindingStub{discovered: protocol.NewPage([]protocol.Skill{
				{Name: "review", Scope: protocol.SkillScopeProject},
				{Name: "review", Scope: protocol.SkillScopeProject},
			})},
			read: func(runtime *Connection) error {
				_, err := runtime.Discover(t.Context(), "/workspace")
				return err
			},
		}, {
			name: "blank managed name",
			stub: &invalidSkillBindingStub{managed: protocol.NewPage([]protocol.ManagedSkill{{Name: " \t", Lifecycle: protocol.SkillLifecycleActive}})},
			read: func(runtime *Connection) error {
				_, err := runtime.Managed(t.Context())
				return err
			},
		}, {
			name: "repeated managed name",
			stub: &invalidSkillBindingStub{managed: protocol.NewPage([]protocol.ManagedSkill{
				{Name: "review", Lifecycle: protocol.SkillLifecycleActive},
				{Name: "review", Lifecycle: protocol.SkillLifecycleArchived},
			})},
			read: func(runtime *Connection) error {
				_, err := runtime.Managed(t.Context())
				return err
			},
		}, {
			name: "repeated proposal slot",
			stub: &invalidSkillBindingStub{proposals: protocol.NewPage([]protocol.SkillProposal{
				{Name: "review", Revision: skillRevision, Scope: protocol.SkillScopeProject, Description: "Review code", Instructions: "Inspect code."},
				{Name: "review", Revision: otherSkillRevision, Scope: protocol.SkillScopeProject, Description: "Review again", Instructions: "Inspect code again."},
			})},
			read: func(runtime *Connection) error {
				_, err := runtime.Proposals(t.Context(), "/workspace")
				return err
			},
		}, {
			name: "out-of-order proposal catalog",
			stub: &invalidSkillBindingStub{proposals: protocol.NewPage([]protocol.SkillProposal{
				{Name: "alpha", Revision: skillRevision, Scope: protocol.SkillScopeUser, Description: "User proposal", Instructions: "Review the user proposal."},
				{Name: "zeta", Revision: otherSkillRevision, Scope: protocol.SkillScopeProject, Description: "Project proposal", Instructions: "Review the project proposal."},
			})},
			read: func(runtime *Connection) error {
				_, err := runtime.Proposals(t.Context(), "/workspace")
				return err
			},
		}, {
			name: "non-canonical proposal revision",
			stub: &invalidSkillBindingStub{proposals: protocol.NewPage([]protocol.SkillProposal{{
				Name: "review", Revision: "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789",
				Scope: protocol.SkillScopeUser, Description: "Review code", Instructions: "Inspect code.",
			}})},
			read: func(runtime *Connection) error {
				_, err := runtime.Proposals(t.Context(), "/workspace")
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.stub.skillBindingStub = &skillBindingStub{t: t}
			runtime := &Connection{skills: test.stub, meta: requestMeta("test")}
			err := test.read(runtime)
			if err == nil {
				t.Fatal("invalid skill value was accepted")
			}
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestConnectionProfileControlsOptionalAdapterAvailability(t *testing.T) {
	runtime := &Connection{profile: Profile{Features: map[string]Feature{
		protocol.FeatureSkills: {Enabled: true}, protocol.FeatureMCP: {Enabled: true},
		protocol.FeatureSchedules: {Enabled: true}, protocol.FeatureAgentMemory: {Enabled: true},
		protocol.FeatureKnowledge: {Enabled: true}, protocol.FeatureSessionExport: {Enabled: true},
	}}}
	profile := runtime.Profile()
	if !profile.Supports(protocol.FeatureSkills) || !profile.Supports(protocol.FeatureMCP) ||
		!profile.Supports(protocol.FeatureSchedules) || profile.Supports(protocol.FeatureGoals) {
		t.Fatalf("profile features = %+v", profile.Features)
	}
	if runtime.AgentMemory() == nil || runtime.Knowledge() == nil {
		t.Fatal("advertised context adapters were not exposed")
	}
	runtime.profile.Features[protocol.FeatureAgentMemory] = Feature{}
	if runtime.AgentMemory() != nil {
		t.Fatal("unadvertised agent memory exposed an adapter")
	}
}
