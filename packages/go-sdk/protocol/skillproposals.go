package protocol

import "github.com/Tangerg/flame/packages/go-sdk/skill"

//flame:enum
type SkillProposalOrigin string

const (
	SkillProposalOriginRequested SkillProposalOrigin = SkillProposalOrigin(skill.Requested)
	SkillProposalOriginMined     SkillProposalOrigin = SkillProposalOrigin(skill.Mined)
)

//flame:enum
type SkillProposalScope string

const (
	SkillProposalScopeRepository SkillProposalScope = SkillProposalScope(skill.Repository)
	SkillProposalScopeUser       SkillProposalScope = SkillProposalScope(skill.User)
)

type SkillProposal struct {
	Name          string              `json:"name"`
	Revision      string              `json:"revision"`
	Scope         SkillProposalScope  `json:"scope"`
	Description   string              `json:"description"`
	Instructions  string              `json:"instructions"`
	Origin        SkillProposalOrigin `json:"origin"`
	SourceSession string              `json:"sourceSession,omitempty"`
	Revises       bool                `json:"revises,omitzero"`
}

type SkillProposalRef struct {
	Workspace WorkspaceRef       `json:"workspace"`
	Name      string             `json:"name"`
	Revision  string             `json:"revision"`
	Scope     SkillProposalScope `json:"scope"`
}
