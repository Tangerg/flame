package skills

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/skill"
	"github.com/Tangerg/flame/runtime/internal/domain/destructive"
	skillspec "github.com/Tangerg/scope/skills"
)

var (
	ErrPublicationUnconfirmed = errors.New("skills: original file publication cannot be confirmed")
	ErrUnsafeProposal         = errors.New("skills: dangerous instruction")
	ErrInvalidName            = errors.New("skills: invalid name")
	ErrConflict               = errors.New("skills: destination already exists")
	ErrProposalChanged        = errors.New("skills: proposal content changed")
	ErrProposalQueueFull      = errors.New("skills: proposal review queue is full")
	ErrDocumentTooLarge       = errors.New("skills: document is too large")
	ErrLibraryCapacity        = errors.New("skills: library capacity exceeded")
	ErrResourceTooLarge       = errors.New("skills: resource is too large")
	ErrUsageTooLarge          = errors.New("skills: usage metadata is too large")
	ErrNotFound               = errors.New("skills: entry not found")
)

func ValidateName(name string) error {
	if err := skillspec.ValidateName(name); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidName, err)
	}
	return nil
}

const (
	MaxAuthoredSkillDocumentBytes = 1 << 20

	MaxSkillsPerSource = 256

	MaxSkillDirectoryEntries = MaxSkillsPerSource + 16

	MaxSkillResourceBytes = 1 << 20

	ProposalRevisionPattern = `^[0-9a-f]{64}$`
)

var proposalRevisionPattern = regexp.MustCompile(ProposalRevisionPattern)

type Scope string

const (
	ScopeRepository Scope = Scope(skill.Repository)
	ScopeUser       Scope = Scope(skill.User)
)

func (s Scope) Validate() error {
	switch s {
	case ScopeRepository, ScopeUser:
		return nil
	default:
		return fmt.Errorf("skills: invalid scope %q", s)
	}
}

type ProposalOrigin string

const (
	ProposalOriginRequested ProposalOrigin = ProposalOrigin(skill.Requested)
	ProposalOriginMined     ProposalOrigin = ProposalOrigin(skill.Mined)
)

func (p ProposalOrigin) Validate() error {
	switch p {
	case ProposalOriginRequested, ProposalOriginMined:
		return nil
	default:
		return fmt.Errorf("skills: invalid proposal origin %q", p)
	}
}

type Lifecycle string

const (
	Active   Lifecycle = "active"
	Archived Lifecycle = "archived"
)

func (l Lifecycle) Validate() error {
	if l != Active && l != Archived {
		return fmt.Errorf("skills: invalid lifecycle %q", l)
	}
	return nil
}

type Entry struct {
	Name        string
	Description string
	Lifecycle   Lifecycle
}

func (e Entry) Validate() error {
	if err := (skillspec.Frontmatter{Name: e.Name, Description: e.Description}).Validate(); err != nil {
		return err
	}
	return e.Lifecycle.Validate()
}

// Origin and SourceSession are runtime provenance and must never be accepted from
// model-authored tool arguments.
type Proposal struct {
	Scope         Scope
	Name          string
	Description   string
	Instructions  string
	Origin        ProposalOrigin
	SourceSession string
	Revises       bool
}

type ProposalRef struct {
	Scope    Scope
	Name     string
	Revision string
}

func CommandRef(scope Scope, name, command string) ProposalRef {
	return ProposalRef{Scope: scope, Name: name, Revision: command}
}

func (p ProposalRef) Validate() error {
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if err := skillspec.ValidateName(p.Name); err != nil {
		return fmt.Errorf("proposal reference name: %w", err)
	}
	if !proposalRevisionPattern.MatchString(p.Revision) {
		return errors.New("proposal reference revision must be a lowercase SHA-256 digest")
	}
	return nil
}

type ProposalReview struct {
	Ref           ProposalRef
	Description   string
	Instructions  string
	Origin        ProposalOrigin
	SourceSession string
	Revises       bool
}

func (p ProposalReview) Validate() error {
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	return (Proposal{
		Scope: p.Ref.Scope, Name: p.Ref.Name, Description: p.Description,
		Instructions: p.Instructions, Origin: p.Origin,
		SourceSession: p.SourceSession, Revises: p.Revises,
	}).Validate()
}

func (p Proposal) Validate() error {
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if err := p.Origin.Validate(); err != nil {
		return err
	}
	if err := (skillspec.Frontmatter{Name: p.Name, Description: p.Description}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Instructions) == "" {
		return errors.New("skill instructions are required")
	}
	if len(p.Instructions) > MaxAuthoredSkillDocumentBytes {
		return fmt.Errorf(
			"%w: instructions use %d bytes before frontmatter (maximum document %d)",
			ErrDocumentTooLarge,
			len(p.Instructions),
			MaxAuthoredSkillDocumentBytes,
		)
	}
	return p.CheckSafety()
}

var dangerousSkillPattern = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|]*\|\s*(sudo\s+)?(sh|bash|zsh)\b`),
}

type ProposalSafetyIssue string

const (
	ProposalSafe                 ProposalSafetyIssue = "safe"
	ProposalDangerousInstruction ProposalSafetyIssue = "dangerousInstruction"
)

func (p Proposal) SafetyIssue() ProposalSafetyIssue {
	content := p.Name + "\n" + p.Description + "\n" + p.Instructions
	if destructive.Command(content) {
		return ProposalDangerousInstruction
	}
	for _, re := range dangerousSkillPattern {
		if re.MatchString(content) {
			return ProposalDangerousInstruction
		}
	}
	return ProposalSafe
}

func (p Proposal) CheckSafety() error {
	if p.SafetyIssue() != ProposalSafe {
		return fmt.Errorf("%w: %q", ErrUnsafeProposal, p.Name)
	}
	return nil
}
