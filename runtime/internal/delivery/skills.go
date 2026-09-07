package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	SkillsDiscoveredList   Name = "skills.discovered.list"
	SkillsLibraryList      Name = "skills.library.list"
	SkillsLibraryArchive   Name = "skills.library.archive"
	SkillsLibraryRestore   Name = "skills.library.restore"
	SkillsProposalsList    Name = "skills.proposals.list"
	SkillsProposalsApprove Name = "skills.proposals.approve"
	SkillsProposalsReject  Name = "skills.proposals.reject"
)

func registerSkills(registry *Registry) {
	registry.Query(MethodMeta{
		Name:            SkillsDiscoveredList,
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, (*Handler).ListDiscoveredSkills)

	registry.Query(MethodMeta{
		Name:            SkillsLibraryList,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(service *Handler, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return service.ListManagedSkills(ctx)
	})

	registry.CommandAck(MethodMeta{
		Name:            SkillsLibraryArchive,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, (*Handler).ArchiveSkill)

	registry.CommandAck(MethodMeta{
		Name:            SkillsLibraryRestore,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, (*Handler).RestoreSkill)

	registry.Query(MethodMeta{
		Name:            SkillsProposalsList,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, (*Handler).ListSkillProposals)

	registry.CommandAck(MethodMeta{
		Name:            SkillsProposalsApprove,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, (*Handler).ApproveSkillProposal)

	registry.CommandAck(MethodMeta{
		Name:            SkillsProposalsReject,
		CapabilityRules: requires(protocol.FeatureSkills),
	}, (*Handler).RejectSkillProposal)

}
