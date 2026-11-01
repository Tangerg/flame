package protocol

import "github.com/Tangerg/flame/packages/go-sdk/skill"

// SkillProposalOrigin identifies why the runtime submitted a proposal.
//
//flame:enum
type SkillProposalOrigin string

const (
	SkillProposalOriginRequested SkillProposalOrigin = SkillProposalOrigin(skill.Requested)
	SkillProposalOriginMined     SkillProposalOrigin = SkillProposalOrigin(skill.Mined)
)

// SkillProposalScope names the library a proposal would join. Proposals are
// made only for the project or user library; installed plugins bring their own
// Skills and are never proposed into.
//
//flame:enum
type SkillProposalScope string

const (
	SkillProposalScopeProject SkillProposalScope = SkillProposalScope(skill.Project)
	SkillProposalScopeUser    SkillProposalScope = SkillProposalScope(skill.User)
)

// Revision identifies the original proposal command. The retained input fixes
// the content, target library, and source release seen by a reviewer.
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

// SkillProposalRef identifies the exact proposal and workspace review context
// that an approve or reject operation acts on. A changed, removed, or no longer
// applicable proposal returns revision_conflict; clients must read and review
// current content before issuing a new decision.
type SkillProposalRef struct {
	Workspace WorkspaceRef       `json:"workspace"`
	Name      string             `json:"name"`
	Revision  string             `json:"revision"`
	Scope     SkillProposalScope `json:"scope"`
}
