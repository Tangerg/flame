package workspace

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

const (
	MaxAuthoredPromptDocumentBytes = 1 << 20
	MaxAgentDocumentsPerCascade    = 64
	MaxAgentDocumentCascadeBytes   = 4 << 20
)

var (
	ErrPromptSourceTooLarge = errors.New("workspace: prompt source too large")
	ErrInvalidPromptSource  = errors.New("workspace: invalid prompt source")
)

func ValidateAuthoredPromptDocumentSize(size int64) error {
	if size < 0 {
		return fmt.Errorf("%w: document size cannot be negative", ErrInvalidPromptSource)
	}
	if size > MaxAuthoredPromptDocumentBytes {
		return fmt.Errorf(
			"%w: document uses %d bytes, maximum %d",
			ErrPromptSourceTooLarge,
			size,
			MaxAuthoredPromptDocumentBytes,
		)
	}
	return nil
}

// Checked after reading so a file that grows after stat cannot cross the same
// admission boundary.
func ValidateAuthoredPromptDocument(document []byte) error {
	if err := ValidateAuthoredPromptDocumentSize(int64(len(document))); err != nil {
		return err
	}
	if !utf8.Valid(document) {
		return fmt.Errorf("%w: document must be valid UTF-8", ErrInvalidPromptSource)
	}
	return nil
}

type AgentDocScope string

const (
	AgentDocScopeHome           AgentDocScope = "home"
	AgentDocScopeCWD            AgentDocScope = "cwd"
	AgentDocScopeRepositoryRoot AgentDocScope = "repositoryRoot"
)

type AgentDocFile struct {
	Path    string
	Content string
	Scope   AgentDocScope
}

func ValidateAgentDocumentCascade(files []AgentDocFile) error {
	if len(files) > MaxAgentDocumentsPerCascade {
		return fmt.Errorf(
			"%w: agent document cascade has %d documents, maximum %d",
			ErrPromptSourceTooLarge,
			len(files),
			MaxAgentDocumentsPerCascade,
		)
	}
	total := 0
	seen := make(map[string]struct{}, len(files))
	previousPhase := -1
	for index, file := range files {
		if strings.TrimSpace(file.Path) == "" || strings.TrimSpace(file.Content) == "" {
			return fmt.Errorf("%w: agent document %d is incomplete", ErrInvalidPromptSource, index)
		}
		if !utf8.ValidString(file.Path) {
			return fmt.Errorf("%w: agent document %d path must be valid UTF-8", ErrInvalidPromptSource, index)
		}
		phase, validScope := file.Scope.renderPhase()
		if !validScope {
			return fmt.Errorf("%w: agent document %q has unknown scope %q", ErrInvalidPromptSource, file.Path, file.Scope)
		}
		if phase < previousPhase {
			return fmt.Errorf("%w: agent document %q returns to scope %q after a later render phase", ErrInvalidPromptSource, file.Path, file.Scope)
		}
		previousPhase = phase
		if _, duplicate := seen[file.Path]; duplicate {
			return fmt.Errorf("%w: agent document cascade repeats source %q", ErrInvalidPromptSource, file.Path)
		}
		seen[file.Path] = struct{}{}
		if err := validateAuthoredPromptString(file.Content); err != nil {
			return fmt.Errorf("agent document %q: %w", file.Path, err)
		}
		if len(file.Content) > MaxAgentDocumentCascadeBytes-total {
			return fmt.Errorf(
				"%w: agent document cascade exceeds %d bytes",
				ErrPromptSourceTooLarge,
				MaxAgentDocumentCascadeBytes,
			)
		}
		total += len(file.Content)
	}
	return nil
}

func (s AgentDocScope) renderPhase() (int, bool) {
	switch s {
	case AgentDocScopeHome:
		return 0, true
	case AgentDocScopeRepositoryRoot:
		return 1, true
	case AgentDocScopeCWD:
		return 2, true
	default:
		return 0, false
	}
}

func validateAuthoredPromptString(document string) error {
	if err := ValidateAuthoredPromptDocumentSize(int64(len(document))); err != nil {
		return err
	}
	if !utf8.ValidString(document) {
		return fmt.Errorf("%w: document must be valid UTF-8", ErrInvalidPromptSource)
	}
	return nil
}

type SkillSource struct {
	scope      skills.Scope
	dependency plugin.Dependency
}

func RepositorySkillSource() SkillSource { return SkillSource{scope: skills.ScopeRepository} }
func UserSkillSource() SkillSource       { return SkillSource{scope: skills.ScopeUser} }

func InstallationSkillSource(release plugin.Dependency) (SkillSource, error) {
	if err := release.Validate(); err != nil {
		return SkillSource{}, fmt.Errorf("workspace: installation Skill source: %w", err)
	}
	return SkillSource{dependency: release}, nil
}

func (s SkillSource) Scope() skills.Scope { return s.scope }

func (s SkillSource) Installation() (plugin.Dependency, bool) {
	return s.dependency, s.dependency.InstallationID.Validate() == nil
}

func (s SkillSource) Validate() error {
	if _, found := s.Installation(); found {
		return nil
	}
	return s.scope.Validate()
}

type SkillSummary struct {
	Name        string
	Description string
	Source      SkillSource
}

type SkillDiscovery struct {
	Skills      []SkillSummary
	Diagnostics []SkillDiagnostic
}

type SkillDiagnostic struct {
	Name   string
	Detail string
}

type SkillDetail struct {
	SkillSummary
	Path         string
	Revision     string
	Instructions string
}
