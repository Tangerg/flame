package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
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
		Name:            SkillsDiscoveredList,
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery) (*protocol.SkillDiscovery, error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.SkillDiscovery, error) {
		return service.ListDiscoveredSkills(ctx, request)
	})

	registry.query(MethodMeta{
		Name:            SkillsDiscoveredGet,
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrSkillNotFound.Error(), protocol.ErrSkillUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		GetDiscoveredSkill(context.Context, protocol.SkillDetailRequest) (*protocol.SkillDetail, error)
	}, ctx context.Context, request protocol.SkillDetailRequest) (*protocol.SkillDetail, error) {
		return service.GetDiscoveredSkill(ctx, request)
	})

	registry.query(MethodMeta{
		Name:            SkillsLibraryList,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListManagedSkills(context.Context) (*protocol.Page[protocol.ManagedSkill], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return service.ListManagedSkills(ctx)
	})

	registry.commandAck(MethodMeta{
		Name:            SkillsLibraryArchive,
		Errors:          []string{protocol.ErrSkillNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ArchiveSkill(context.Context, protocol.SkillNameRequest) error
	}, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.ArchiveSkill(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name:            SkillsLibraryRestore,
		Errors:          []string{protocol.ErrSkillNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		RestoreSkill(context.Context, protocol.SkillNameRequest) error
	}, ctx context.Context, request protocol.SkillNameRequest) error {
		return service.RestoreSkill(ctx, request)
	})

	registry.query(MethodMeta{
		Name:            SkillsProposalsList,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ListSkillProposals(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
		return service.ListSkillProposals(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name:            SkillsProposalsApprove,
		Errors:          []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		ApproveSkillProposal(context.Context, protocol.SkillProposalRef) error
	}, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.ApproveSkillProposal(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name:            SkillsProposalsReject,
		Errors:          []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service interface {
		RejectSkillProposal(context.Context, protocol.SkillProposalRef) error
	}, ctx context.Context, request protocol.SkillProposalRef) error {
		return service.RejectSkillProposal(ctx, request)
	})

}
