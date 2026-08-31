package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListDiscoveredSkills returns Skills applicable to a workspace.
func (r *Runtime) ListDiscoveredSkills(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.Skill], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.Skill]](ctx, delivery.SkillsDiscoveredList, request, callOptions(options))
}

// ListManagedSkills returns user-scope Skills managed by the Runtime.
func (r *Runtime) ListManagedSkills(ctx context.Context, options CallOptions) (*protocol.Page[protocol.ManagedSkill], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.ManagedSkill]](ctx, delivery.SkillsLibraryList, struct{}{}, callOptions(options))
}

// ArchiveSkill removes a managed Skill from active discovery.
func (r *Runtime) ArchiveSkill(ctx context.Context, request protocol.SkillNameRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.SkillsLibraryArchive, request, commandOptions(options))
}

// RestoreSkill restores an archived managed Skill.
func (r *Runtime) RestoreSkill(ctx context.Context, request protocol.SkillNameRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.SkillsLibraryRestore, request, commandOptions(options))
}

// ListSkillProposals returns pending Skill proposals for a workspace.
func (r *Runtime) ListSkillProposals(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.SkillProposal], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.SkillProposal]](ctx, delivery.SkillsProposalsList, request, callOptions(options))
}

// ApproveSkillProposal accepts one proposed Skill.
func (r *Runtime) ApproveSkillProposal(ctx context.Context, request protocol.SkillProposalRef, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.SkillsProposalsApprove, request, commandOptions(options))
}

// RejectSkillProposal rejects one proposed Skill.
func (r *Runtime) RejectSkillProposal(ctx context.Context, request protocol.SkillProposalRef, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.SkillsProposalsReject, request, commandOptions(options))
}
