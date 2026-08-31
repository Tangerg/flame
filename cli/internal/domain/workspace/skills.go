package workspace

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type SkillScope string

const (
	SkillProjectScope SkillScope = "project"
	SkillUserScope    SkillScope = "user"
)

func (s SkillScope) Validate() error {
	if s != SkillProjectScope && s != SkillUserScope {
		return fmt.Errorf("skill scope %q is invalid", s)
	}
	return nil
}

type SkillLifecycle string

const (
	SkillActive   SkillLifecycle = "active"
	SkillArchived SkillLifecycle = "archived"
)

func (l SkillLifecycle) Validate() error {
	if l != SkillActive && l != SkillArchived {
		return fmt.Errorf("skill lifecycle %q is invalid", l)
	}
	return nil
}

type SkillProposalOrigin string

const (
	SkillProposalRequested SkillProposalOrigin = "requested"
	SkillProposalMined     SkillProposalOrigin = "mined"
)

func (o SkillProposalOrigin) Validate() error {
	if o != "" && o != SkillProposalRequested && o != SkillProposalMined {
		return fmt.Errorf("skill proposal origin %q is invalid", o)
	}
	return nil
}

type DiscoveredSkill struct {
	Name        string
	Description string
	Scope       SkillScope
}

func (d DiscoveredSkill) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("discovered skill name is empty")
	}
	return d.Scope.Validate()
}

func (d DiscoveredSkill) Key() string { return string(d.Scope) + "/" + d.Name }

type ManagedSkill struct {
	Name        string
	Description string
	Lifecycle   SkillLifecycle
}

func (m ManagedSkill) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("managed skill name is empty")
	}
	return m.Lifecycle.Validate()
}

// ValidateSkillLifecycleAcknowledgement proves that an authoritative managed-skill
// catalog reflects the requested lifecycle for exactly one named skill.
func ValidateSkillLifecycleAcknowledgement(catalog []ManagedSkill, name string, lifecycle SkillLifecycle) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("managed skill acknowledgement name is empty")
	}
	if err := lifecycle.Validate(); err != nil {
		return err
	}
	found := false
	for index, skill := range catalog {
		if err := skill.Validate(); err != nil {
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
	Scope         SkillScope
	Description   string
	Instructions  string
	Origin        SkillProposalOrigin
	SourceSession string
	Revises       bool
}

func (p SkillProposal) Validate() error {
	if err := validateProposalIdentity(p.Name, p.Revision, p.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(p.Description) == "" {
		return errors.New("skill proposal description is empty")
	}
	if strings.TrimSpace(p.Instructions) == "" {
		return errors.New("skill proposal instructions are empty")
	}
	return p.Origin.Validate()
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
	Scope     SkillScope
}

func (p SkillProposalReference) Validate() error {
	if strings.TrimSpace(p.Workspace) == "" {
		return errors.New("skill proposal reference workspace is empty")
	}
	if err := validateProposalIdentity(p.Name, p.Revision, p.Scope); err != nil {
		return fmt.Errorf("skill proposal reference: %w", err)
	}
	return nil
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

func validateProposalIdentity(name, revision string, scope SkillScope) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("skill proposal name is empty")
	}
	if len(revision) != 64 {
		return errors.New("skill proposal revision is not a SHA-256 digest")
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return fmt.Errorf("skill proposal revision: %w", err)
	}
	return scope.Validate()
}

type SkillService interface {
	Discover(context.Context, string) ([]DiscoveredSkill, error)
	Managed(context.Context) ([]ManagedSkill, error)
	Proposals(context.Context, string) ([]SkillProposal, error)
	Archive(context.Context, string) error
	Restore(context.Context, string) error
	Approve(context.Context, SkillProposalReference) error
	Reject(context.Context, SkillProposalReference) error
}
