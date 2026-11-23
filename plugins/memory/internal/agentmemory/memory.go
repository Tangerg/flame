package agentmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	MaxContentCharacters = 4096

	// Admission precedes deduplication so repeated output cannot evade the extraction envelope.
	MaxFactsPerBatch = 32

	MaxCurationProposals = 32

	MaxLedgerFoldFacts = 128

	MaxVisiblePerTarget    = 512
	MaxManagementPageItems = 100

	MaxRejectedPerTarget = 2048
)

func NormalizeContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("agentmemory: item content is required")
	}
	if !utf8.ValidString(content) {
		return "", errors.New("agentmemory: item content must be valid UTF-8")
	}
	if utf8.RuneCountInString(content) > MaxContentCharacters {
		return "", fmt.Errorf(
			"agentmemory: item content exceeds %d Unicode characters",
			MaxContentCharacters,
		)
	}
	return content, nil
}

func Digest(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

var ErrInvalidFacts = errors.New("agentmemory: invalid facts")

var ErrNotFound = errors.New("agentmemory: item not found")

var ErrNotPending = errors.New("agentmemory: item is not pending review")

var ErrNotVisible = errors.New("agentmemory: item is not visible")

var ErrTargetFull = errors.New("agentmemory: target is full")

var ErrInvalidRevision = errors.New("agentmemory: revision must be a positive exact integer")

var ErrRevisionConflict = errors.New("agentmemory: item revision changed")

var ErrDuplicateContent = errors.New("agentmemory: content already belongs to another item in this target")

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

func (s Scope) valid() bool {
	return slices.Contains(s.JSONSchemaEnum(), string(s))
}

func (s Scope) Validate() error {
	if !s.valid() {
		return fmt.Errorf("agentmemory: invalid scope %q", s)
	}
	return nil
}

func (s Scope) String() string { return string(s) }

func (Scope) JSONSchemaEnum() []string { return []string{string(ScopeProject), string(ScopeUser)} }

func ParseScope(value string) (Scope, error) {
	scope := Scope(value)
	return scope, scope.Validate()
}

func ValidateTarget(scope Scope, project string) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	switch scope {
	case ScopeProject:
		if strings.TrimSpace(project) == "" {
			return errors.New("agentmemory: project scope requires a project")
		}
	case ScopeUser:
		if project != "" {
			return errors.New("agentmemory: user scope forbids a project")
		}
	}
	return nil
}

type Status string

const (
	StatusActive   Status = "active"
	StatusPending  Status = "pending"
	StatusRejected Status = "rejected"
)

func (s Status) valid() bool {
	return slices.Contains(s.JSONSchemaEnum(), string(s))
}

func (s Status) validate() error {
	if !s.valid() {
		return fmt.Errorf("agentmemory: invalid status %q", s)
	}
	return nil
}

func (s Status) String() string { return string(s) }

func (Status) JSONSchemaEnum() []string {
	return []string{string(StatusActive), string(StatusPending), string(StatusRejected)}
}

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	return status, status.validate()
}

type Origin string

const (
	OriginAuto Origin = "auto"
	OriginUser Origin = "user"
)

func (o Origin) valid() bool {
	return slices.Contains(o.JSONSchemaEnum(), string(o))
}

func (o Origin) validate() error {
	if !o.valid() {
		return fmt.Errorf("agentmemory: invalid origin %q", o)
	}
	return nil
}

func (o Origin) String() string { return string(o) }

func (Origin) JSONSchemaEnum() []string { return []string{string(OriginAuto), string(OriginUser)} }

func ParseOrigin(value string) (Origin, error) {
	origin := Origin(value)
	return origin, origin.validate()
}

type ReviewDecision string

const (
	ReviewApprove ReviewDecision = "approve"
	ReviewReject  ReviewDecision = "reject"
)

func (ReviewDecision) JSONSchemaEnum() []string {
	return []string{string(ReviewApprove), string(ReviewReject)}
}

func (r ReviewDecision) Result() (Status, error) {
	switch r {
	case ReviewApprove:
		return StatusActive, nil
	case ReviewReject:
		return StatusRejected, nil
	default:
		return "", fmt.Errorf("agentmemory: invalid review decision %q", r)
	}
}
