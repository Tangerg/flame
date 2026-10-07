package protocol

// SkillProposalOrigin identifies why the runtime submitted a proposal.
type SkillProposalOrigin string

const (
	SkillProposalOriginRequested SkillProposalOrigin = "requested"
	SkillProposalOriginMined     SkillProposalOrigin = "mined"
)

// SkillProposalScope names the library a proposal would join. Proposals are
// made only for the project or user library; installed plugins bring their own
// Skills and are never proposed into.
type SkillProposalScope string

const (
	SkillProposalScopeProject SkillProposalScope = "project"
	SkillProposalScopeUser    SkillProposalScope = "user"
)

// SkillProposal is complete immutable Skill content awaiting review. Name,
// Revision, and Scope form the content-addressed reference used by approve and
// reject operations. List results contain one current revision per Scope/Name,
// ordered project scope first, then user scope, and by Name within each scope.
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
