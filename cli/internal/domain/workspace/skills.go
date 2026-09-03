package workspace

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

type DiscoveredSkill struct {
	Name        string
	Description string
	Scope       protocol.SkillScope
}

func (d DiscoveredSkill) Validate() error {
	return (protocol.Skill{Name: d.Name, Description: d.Description, Scope: d.Scope}).ValidateWire()
}

func (d DiscoveredSkill) Key() string { return string(d.Scope) + "/" + d.Name }

// ValidateSkillLifecycleAcknowledgement proves that an authoritative managed-skill
// catalog reflects the requested lifecycle for exactly one named skill.
func ValidateSkillLifecycleAcknowledgement(catalog []protocol.ManagedSkill, name string, lifecycle protocol.SkillLifecycle) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("managed skill acknowledgement name is empty")
	}
	if err := (protocol.ManagedSkill{Name: name, Lifecycle: lifecycle}).ValidateWire(); err != nil {
		return err
	}
	found := false
	for index, skill := range catalog {
		if err := skill.ValidateWire(); err != nil {
			return fmt.Errorf("managed skill acknowledgement item %d: %w", index+1, err)
		}
		if skill.Name != name {
			continue
		}
		if found {
			return fmt.Errorf("managed skill acknowledgement repeats %q", name)
		}
		found = true
		if skill.Lifecycle != lifecycle {
			return fmt.Errorf("managed skill %q lifecycle is %q, want %q", name, skill.Lifecycle, lifecycle)
		}
	}
	if !found {
		return fmt.Errorf("managed skill %q is missing after lifecycle change", name)
	}
	return nil
}

type SkillProposal struct {
	Name          string
	Revision      string
	Scope         protocol.SkillScope
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
	Scope     protocol.SkillScope
}

func (p SkillProposalReference) Validate() error {
	return protocol.ValidateWireTree(protocol.SkillProposalRef{
		Workspace: protocol.WorkspaceRef{Path: p.Workspace},
		Name:      p.Name, Revision: p.Revision, Scope: p.Scope,
	})
}

// ValidateDecisionAcknowledgement proves that the exact immutable proposal
// reviewed by Approve or Reject is no longer pending. Other revisions of the
// same skill remain independent proposals.
func (p SkillProposalReference) ValidateDecisionAcknowledgement(pending []SkillProposal) error {
	if err := p.Validate(); err != nil {
		return err
	}
	for index, proposal := range pending {
		if err := proposal.Validate(); err != nil {
			return fmt.Errorf("skill proposal acknowledgement item %d: %w", index+1, err)
		}
		if proposal.Name == p.Name && proposal.Scope == p.Scope && proposal.Revision == p.Revision {
			return fmt.Errorf("skill proposal %s remains pending after decision", proposal.Key())
		}
	}
	return nil
}
