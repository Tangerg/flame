package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

const (
	SkillsDiscoveredGet    Name = "skills.discovered.get"
	SkillsDiscoveredList   Name = "skills.discovered.list"
	SkillsLibraryList      Name = "skills.library.list"
	SkillsLibraryArchive   Name = "skills.library.archive"
	SkillsLibraryRestore   Name = "skills.library.restore"
	SkillsProposalsList    Name = "skills.proposals.list"
	SkillsProposalsApprove Name = "skills.proposals.approve"
	SkillsProposalsReject  Name = "skills.proposals.reject"
)

func registerSkills(registry *Registry) {
	registry.query(MethodMeta{
		Name: SkillsDiscoveredList, GoName: "ListDiscoveredSkills",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery) (*protocol.SkillDiscovery, error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.SkillDiscovery, error) {
		return service.ListDiscoveredSkills(ctx, request)
	})

	registry.query(MethodMeta{
		Name: SkillsDiscoveredGet, GoName: "GetDiscoveredSkill",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrSkillNotFound.Error(), protocol.ErrSkillUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		GetDiscoveredSkill(context.Context, protocol.SkillDetailRequest) (*protocol.SkillDetail, error)
	}, ctx context.Context, request protocol.SkillDetailRequest) (*protocol.SkillDetail, error) {
		return service.GetDiscoveredSkill(ctx, request)
	})

	registry.query(MethodMeta{
		Name: SkillsLibraryList, GoName: "ListManagedSkills",
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListManagedSkills(context.Context) (*protocol.Page[protocol.ManagedSkill], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return service.ListManagedSkills(ctx)
	})

	registerSkillMove(registry, SkillsLibraryArchive, "ArchiveSkill", skills.Archived)
	registerSkillMove(registry, SkillsLibraryRestore, "RestoreSkill", skills.Active)

	registry.query(MethodMeta{
		Name: SkillsProposalsList, GoName: "ListSkillProposals",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListSkillProposals(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
		return service.ListSkillProposals(ctx, request)
	})

	registerSkillReview(registry, SkillsProposalsApprove, "ApproveSkillProposal", (*workspace.Skills).ApproveProposal)
	registerSkillReview(registry, SkillsProposalsReject, "RejectSkillProposal", (*workspace.Skills).RejectProposal)
}

func registerSkillMove(registry *Registry, method Name, goName string, destination skills.Lifecycle) {
	registry.commandAck(MethodMeta{
		Name: method, GoName: goName,
		Errors:          []string{protocol.ErrSkillNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(s *Handler, ctx context.Context, request protocol.SkillNameRequest) error {
		return mapSkillError(s.workspaceSkills.Move(ctx, request.Name, destination, operation.ReplayClaim{}))
	}).withOwnedReplay(skillMoveReplay(destination))
}
