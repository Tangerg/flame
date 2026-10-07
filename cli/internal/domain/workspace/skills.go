package workspace

import "github.com/Tangerg/flame/runtime/protocol"

func DiscoveredSkillKey(skill protocol.Skill) string { return string(skill.Scope) + "/" + skill.Name }

type SkillProposal struct {
	Name          string
	Revision      string
	Scope         protocol.SkillProposalScope
	Description   string
	Instructions  string
	Origin        protocol.SkillProposalOrigin
	SourceSession string
	Revises       bool
}

func (p SkillProposal) Validate() error {
	return (protocol.SkillProposal{
		Name: p.Name, Revision: p.Revision, Scope: p.Scope,
		Description: p.Description, Instructions: p.Instructions,
		Origin: p.Origin, SourceSession: p.SourceSession, Revises: p.Revises,
	}).ValidateWire()
}

func (p SkillProposal) QualifiedName() string { return string(p.Scope) + "/" + p.Name }

func (p SkillProposal) Key() string {
	revision := p.Revision
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return p.QualifiedName() + "@" + revision
}

func (p SkillProposal) Reference(workspace string) (SkillProposalReference, error) {
	reference := SkillProposalReference{
		Workspace: workspace,
		Name:      p.Name,
		Revision:  p.Revision,
		Scope:     p.Scope,
	}
	return reference, reference.Validate()
}

type SkillProposalReference struct {
	Workspace string
	Name      string
	Revision  string
	Scope     protocol.SkillProposalScope
}

func (p SkillProposalReference) Validate() error {
	return protocol.ValidateWireTree(protocol.SkillProposalRef{
		Workspace: protocol.WorkspaceRef{Path: p.Workspace},
		Name:      p.Name, Revision: p.Revision, Scope: p.Scope,
	})
}
