package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/skills"
)

type skillBinding interface {
	ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.Skill], error)
	ListManagedSkills(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.ManagedSkill], error)
	ArchiveSkill(context.Context, protocol.SkillNameRequest, flameruntime.CommandOptions) error
	RestoreSkill(context.Context, protocol.SkillNameRequest, flameruntime.CommandOptions) error
	ListSkillProposals(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.SkillProposal], error)
	ApproveSkillProposal(context.Context, protocol.SkillProposalRef, flameruntime.CommandOptions) error
	RejectSkillProposal(context.Context, protocol.SkillProposalRef, flameruntime.CommandOptions) error
}

var _ skills.Service = (*Connection)(nil)

func (r *Connection) Discover(ctx context.Context, workspace string) ([]skills.Discovered, error) {
	query, err := skillWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.skills.ListDiscoveredSkills(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list discovered skills", page)
	if err != nil {
		return nil, err
	}
	return projectUniqueValues("list discovered skills", values, func(value protocol.Skill) skills.Discovered {
		return skills.Discovered{
			Name: value.Name, Description: value.Description, Scope: skills.Scope(value.Scope),
		}
	}, func(skill skills.Discovered) string {
		return skill.Key()
	})
}

func (r *Connection) Managed(ctx context.Context) ([]skills.Managed, error) {
	page, err := r.skills.ListManagedSkills(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list managed skills", page)
	if err != nil {
		return nil, err
	}
	return projectUniqueValues("list managed skills", values, func(value protocol.ManagedSkill) skills.Managed {
		return skills.Managed{
			Name: value.Name, Description: value.Description, Lifecycle: skills.Lifecycle(value.Lifecycle),
		}
	}, func(skill skills.Managed) string {
		return skill.Name
	})
}

func (r *Connection) Proposals(ctx context.Context, workspace string) ([]skills.Proposal, error) {
	query, err := skillWorkspaceQuery(workspace)
	if err != nil {
		return nil, err
	}
	page, err := r.skills.ListSkillProposals(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list skill proposals", page)
	if err != nil {
		return nil, err
	}
	projected := make([]skills.Proposal, 0, len(values))
	seen := make(map[[3]string]struct{}, len(values))
	for index, value := range values {
		proposal := skills.Proposal{
			Name: value.Name, Revision: value.Revision, Scope: skills.Scope(value.Scope),
			Description: value.Description, Instructions: value.Instructions,
			Origin: skills.Origin(value.Origin), SourceSession: value.SourceSession, Revises: value.Revises,
		}
		if err := proposal.Validate(); err != nil {
			return nil, runtimeContractViolation("list skill proposals item %d is invalid: %v", index+1, err)
		}
		identity := [3]string{string(proposal.Scope), proposal.Name, proposal.Revision}
		if _, duplicate := seen[identity]; duplicate {
			return nil, runtimeContractViolation("list skill proposals repeats %q", proposal.Key())
		}
		seen[identity] = struct{}{}
		projected = append(projected, proposal)
	}
	return projected, nil
}

func (r *Connection) Archive(ctx context.Context, name string) error {
	return r.changeSkillLifecycle(ctx, "archive skill", name, r.skills.ArchiveSkill)
}

func (r *Connection) Restore(ctx context.Context, name string) error {
	return r.changeSkillLifecycle(ctx, "restore skill", name, r.skills.RestoreSkill)
}

func (r *Connection) changeSkillLifecycle(
	ctx context.Context,
	operation, name string,
	change func(context.Context, protocol.SkillNameRequest, flameruntime.CommandOptions) error,
) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s: skill name is empty", operation)
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(change(ctx, protocol.SkillNameRequest{Name: name}, options))
}

func (r *Connection) Approve(ctx context.Context, reference skills.ProposalReference) error {
	return r.decideSkillProposal(ctx, "approve skill proposal", reference, r.skills.ApproveSkillProposal)
}

func (r *Connection) Reject(ctx context.Context, reference skills.ProposalReference) error {
	return r.decideSkillProposal(ctx, "reject skill proposal", reference, r.skills.RejectSkillProposal)
}

func (r *Connection) decideSkillProposal(
	ctx context.Context,
	operation string,
	reference skills.ProposalReference,
	decide func(context.Context, protocol.SkillProposalRef, flameruntime.CommandOptions) error,
) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	request := protocol.SkillProposalRef{
		Workspace: protocol.WorkspaceRef{Path: reference.Workspace},
		Name:      reference.Name, Revision: reference.Revision, Scope: protocol.SkillScope(reference.Scope),
	}
	if err := decide(ctx, request, options); err != nil {
		return classifyError(err)
	}
	return nil
}

func skillWorkspaceQuery(workspace string) (protocol.WorkspaceQuery, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return protocol.WorkspaceQuery{}, errors.New("skills: workspace is empty")
	}
	return protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: workspace}}, nil
}
