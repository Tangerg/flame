package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
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

func (r *Connection) Discover(ctx context.Context, workspacePath string) ([]protocol.Skill, error) {
	query, err := skillWorkspaceQuery(workspacePath)
	if err != nil {
		return nil, err
	}
	page, err := r.skills.ListDiscoveredSkills(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	found, err := requireCompletePage("list discovered skills", page)
	if err != nil {
		return nil, err
	}
	if err := validateUniqueWireValues("list discovered skills", found, func(skill protocol.Skill) string {
		return skill.Name
	}); err != nil {
		return nil, err
	}
	for index := 1; index < len(found); index++ {
		if found[index].Name < found[index-1].Name {
			return nil, runtimeContractViolation(
				"list discovered skills returned name %q out of catalog order after %q",
				found[index].Name,
				found[index-1].Name,
			)
		}
	}
	return found, nil
}

func (r *Connection) Managed(ctx context.Context) ([]protocol.ManagedSkill, error) {
	page, err := r.skills.ListManagedSkills(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	managed, err := requireCompletePage("list managed skills", page)
	if err != nil {
		return nil, err
	}
	if err := validateUniqueWireValues("list managed skills", managed, func(skill protocol.ManagedSkill) string {
		return skill.Name
	}); err != nil {
		return nil, err
	}
	for index := 1; index < len(managed); index++ {
		previous, current := managed[index-1], managed[index]
		if current.Lifecycle < previous.Lifecycle || current.Lifecycle == previous.Lifecycle && current.Name < previous.Name {
			return nil, runtimeContractViolation(
				"list managed skills returned name %q out of catalog order after %q",
				current.Name,
				previous.Name,
			)
		}
	}
	return managed, nil
}

func (r *Connection) Proposals(ctx context.Context, workspacePath string) ([]workspace.SkillProposal, error) {
	query, err := skillWorkspaceQuery(workspacePath)
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
	projected := make([]workspace.SkillProposal, 0, len(values))
	seen := make(map[[2]string]struct{}, len(values))
	for index, value := range values {
		if err := protocol.ValidateWireTree(value); err != nil {
			return nil, runtimeContractViolation("list skill proposals item %d is invalid: %v", index+1, err)
		}
		proposal := workspace.SkillProposal{
			Name: value.Name, Revision: value.Revision, Scope: value.Scope,
			Description: value.Description, Instructions: value.Instructions,
			Origin: value.Origin, SourceSession: value.SourceSession, Revises: value.Revises,
		}
		identity := [2]string{string(proposal.Scope), proposal.Name}
		if _, duplicate := seen[identity]; duplicate {
			return nil, runtimeContractViolation("list skill proposals repeats current slot %q", proposal.QualifiedName())
		}
		seen[identity] = struct{}{}
		projected = append(projected, proposal)
	}
	for index := 1; index < len(projected); index++ {
		previous, current := projected[index-1], projected[index]
		if current.Scope < previous.Scope || current.Scope == previous.Scope && current.Name < previous.Name {
			return nil, runtimeContractViolation(
				"list skill proposals returned %q out of catalog order after %q",
				current.QualifiedName(),
				previous.QualifiedName(),
			)
		}
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

func (r *Connection) Approve(ctx context.Context, reference workspace.SkillProposalReference) error {
	return r.decideSkillProposal(ctx, "approve skill proposal", reference, r.skills.ApproveSkillProposal)
}

func (r *Connection) Reject(ctx context.Context, reference workspace.SkillProposalReference) error {
	return r.decideSkillProposal(ctx, "reject skill proposal", reference, r.skills.RejectSkillProposal)
}

func (r *Connection) decideSkillProposal(
	ctx context.Context,
	operation string,
	reference workspace.SkillProposalReference,
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
		Name:      reference.Name, Revision: reference.Revision, Scope: reference.Scope,
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
