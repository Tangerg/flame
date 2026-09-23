package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListDiscoveredSkills maps the application-owned, name-ordered visible Skill
// catalog to the protocol shape.
func (s *Handler) ListDiscoveredSkills(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.SkillDiscovery, error) {
	found, err := s.workspaceSkills.List(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Skill, 0, len(found.Skills))
	for _, skill := range found.Skills {
		scope, ok := presentSkillScope(skill.Scope)
		if !ok {
			return nil, fmt.Errorf("skills.discovered.list: unsupported skill scope %q", skill.Scope)
		}
		out = append(out, protocol.Skill{Name: skill.Name, Description: skill.Description, Scope: scope})
	}
	diagnostics := make([]protocol.SkillDiagnostic, 0, len(found.Diagnostics))
	for _, diagnostic := range found.Diagnostics {
		diagnostics = append(diagnostics, protocol.SkillDiagnostic{Name: diagnostic.Name, Detail: diagnostic.Detail})
	}
	return &protocol.SkillDiscovery{Skills: out, Diagnostics: diagnostics}, nil
}

func (s *Handler) GetDiscoveredSkill(ctx context.Context, in protocol.SkillDetailRequest) (*protocol.SkillDetail, error) {
	detail, err := s.workspaceSkills.Get(ctx, in.Workspace.Path, in.Name)
	if err != nil {
		return nil, mapSkillError(err)
	}
	scope, ok := presentSkillScope(detail.Scope)
	if !ok {
		return nil, fmt.Errorf("skills.discovered.get: unsupported skill scope %q", detail.Scope)
	}
	return &protocol.SkillDetail{Skill: protocol.Skill{Name: detail.Name, Description: detail.Description, Scope: scope}, Path: detail.Path, Revision: detail.Revision, Instructions: detail.Instructions}, nil
}

// ListManagedSkills returns the user self-authored Skill library — active then
// archived, ordered by name within each lifecycle and tagged with its lifecycle
// (skills.library.list). The library is small, so it comes back in one page
// and has no continuation cursor.
func (s *Handler) ListManagedSkills(ctx context.Context) (*protocol.Page[protocol.ManagedSkill], error) {
	entries, err := s.workspaceSkills.Managed(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ManagedSkill, 0, len(entries))
	for _, e := range entries {
		lifecycle, ok := presentSkillLifecycle(e.Lifecycle)
		if !ok {
			return nil, fmt.Errorf("skills.library.list: unsupported lifecycle %q", e.Lifecycle)
		}
		out = append(out, protocol.ManagedSkill{
			Name:        e.Name,
			Description: e.Description,
			Lifecycle:   lifecycle,
		})
	}
	return protocol.NewPage(out), nil
}

func presentSkillLifecycle(lifecycle skills.Lifecycle) (protocol.SkillLifecycle, bool) {
	switch lifecycle {
	case skills.Active:
		return protocol.SkillLifecycleActive, true
	case skills.Archived:
		return protocol.SkillLifecycleArchived, true
	default:
		return "", false
	}
}

// ArchiveSkill removes a skill from active use without deleting it
// (skills.library.archive). The application use case publishes the refresh
// nudge after its durable mutation commits.
func (s *Handler) ArchiveSkill(ctx context.Context, in protocol.SkillNameRequest) error {
	return mapSkillError(s.workspaceSkills.Archive(ctx, in.Name))
}

// RestoreSkill returns an archived skill to active use
// (skills.library.restore). The application use case publishes the refresh
// nudge after its durable mutation commits.
func (s *Handler) RestoreSkill(ctx context.Context, in protocol.SkillNameRequest) error {
	return mapSkillError(s.workspaceSkills.Restore(ctx, in.Name))
}

func mapSkillError(err error) error {
	var kind error
	var detail string
	switch {
	case errors.Is(err, skills.ErrInvalidName):
		kind, detail = protocol.ErrInvalidParams, "skill name does not satisfy the Agent Skills naming rules"
	case errors.Is(err, skills.ErrNotFound):
		kind, detail = protocol.ErrSkillNotFound, "the skill is no longer available in the selected scope"
	case errors.Is(err, workspace.ErrSkillUnavailable):
		kind, detail = protocol.ErrSkillUnavailable, "the selected skill document is invalid or exceeds its size limit"
	default:
		return wireWorkspaceError(err)
	}
	return NewFailure(errors.Join(kind, err), detail)
}

// ListSkillProposals returns the one current proposal per scoped Skill name,
// ordered project first and then by name (skills.proposals.list).
func (s *Handler) ListSkillProposals(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
	proposals, err := s.workspaceSkills.Proposals(ctx, in.Workspace.Path)
	if err != nil {
		return nil, mapSkillProposalErr(err)
	}
	out := make([]protocol.SkillProposal, 0, len(proposals))
	for _, proposal := range proposals {
		scope, ok := presentSkillScope(proposal.Ref.Scope)
		if !ok {
			return nil, fmt.Errorf("skills.proposals.list: unsupported scope %q", proposal.Ref.Scope)
		}
		origin, ok := presentSkillProposalOrigin(proposal.Origin)
		if !ok {
			return nil, fmt.Errorf("skills.proposals.list: unsupported origin %q", proposal.Origin)
		}
		out = append(out, protocol.SkillProposal{
			Name:          proposal.Ref.Name,
			Revision:      proposal.Ref.Revision,
			Scope:         scope,
			Description:   proposal.Description,
			Instructions:  proposal.Instructions,
			Origin:        origin,
			SourceSession: proposal.SourceSession,
			Revises:       proposal.Revises,
		})
	}
	return protocol.NewPage(out), nil
}

// ApproveSkillProposal activates exactly the reviewed immutable proposal.
func (s *Handler) ApproveSkillProposal(ctx context.Context, in protocol.SkillProposalRef) error {
	ref, err := skillProposalRef(in)
	if err != nil {
		return err
	}
	return mapSkillProposalErr(s.workspaceSkills.ApproveProposal(ctx, in.Workspace.Path, ref))
}

// RejectSkillProposal removes exactly the reviewed immutable proposal.
func (s *Handler) RejectSkillProposal(ctx context.Context, in protocol.SkillProposalRef) error {
	ref, err := skillProposalRef(in)
	if err != nil {
		return err
	}
	return mapSkillProposalErr(s.workspaceSkills.RejectProposal(ctx, in.Workspace.Path, ref))
}

func skillProposalRef(in protocol.SkillProposalRef) (skills.ProposalRef, error) {
	scope, ok := proposalScopeDomain(in.Scope)
	if !ok {
		return skills.ProposalRef{}, NewFailure(protocol.ErrInvalidParams, "scope must be project or user")
	}
	ref := skills.ProposalRef{Scope: scope, Name: in.Name, Revision: in.Revision}
	if err := ref.Validate(); err != nil {
		return skills.ProposalRef{}, NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	}
	return ref, nil
}

func presentSkillScope(scope skills.Scope) (protocol.SkillScope, bool) {
	switch scope {
	case skills.ScopeProject:
		return protocol.SkillScopeProject, true
	case skills.ScopeUser:
		return protocol.SkillScopeUser, true
	default:
		return "", false
	}
}

func proposalScopeDomain(scope protocol.SkillScope) (skills.Scope, bool) {
	switch scope {
	case protocol.SkillScopeProject:
		return skills.ScopeProject, true
	case protocol.SkillScopeUser:
		return skills.ScopeUser, true
	default:
		return "", false
	}
}

func presentSkillProposalOrigin(origin skills.ProposalOrigin) (protocol.SkillProposalOrigin, bool) {
	switch origin {
	case "":
		return "", true
	case skills.ProposalOriginRequested:
		return protocol.SkillProposalOriginRequested, true
	case skills.ProposalOriginMined:
		return protocol.SkillProposalOriginMined, true
	default:
		return "", false
	}
}

func mapSkillProposalErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, skills.ErrConflict), errors.Is(err, skills.ErrProposalChanged), errors.Is(err, skills.ErrNotFound):
		return NewFailure(errors.Join(protocol.ErrRevisionConflict, err), fmt.Sprintf("proposal review is stale: %v", err))
	default:
		return wireWorkspaceError(err)
	}
}
