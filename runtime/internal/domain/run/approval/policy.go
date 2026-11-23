package approval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
)

var (
	ErrInvalidMode            = errors.New("approval: invalid mode")
	ErrInvalidSessionMode     = errors.New("approval: invalid session mode")
	ErrInvalidQuery           = errors.New("approval: invalid query")
	ErrInvalidRule            = errors.New("approval: invalid rule")
	ErrSourceAuthorityChanged = errors.New("approval: source authority changed")
	ErrRuleCapacity           = errors.New("approval: visible rule capacity exceeded")
)

const MaximumVisibleRules = 2_048

// Values describe policy identity, not strictness order: compare named constants
// and use the behavior methods rather than ordering modes.
type Mode string

const (
	ModeSafe     Mode = "safe"
	ModeBalanced Mode = "balanced"
	ModeYolo     Mode = "yolo"
	ModePlan     Mode = "plan"
)

func (m Mode) ValidDefault() bool {
	return m == ModeSafe || m == ModeBalanced || m == ModeYolo
}

type Scope string

const (
	ScopeSession Scope = "session"
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

func (s Scope) Valid() bool {
	switch s {
	case ScopeSession, ScopeProject, ScopeGlobal:
		return true
	default:
		return false
	}
}

type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
)

func (d Decision) Valid() bool { return d == Allow || d == Deny }

type Resolution struct {
	Decision        Decision
	EditedArguments string
	Reason          string
	RememberScope   Scope
}

func (r Resolution) Validate() error {
	if !r.Decision.Valid() {
		return fmt.Errorf("approval: unknown decision %q", r.Decision)
	}
	if r.RememberScope != "" && !r.RememberScope.Valid() {
		return fmt.Errorf("approval: unknown remember scope %q", r.RememberScope)
	}
	if r.EditedArguments != "" {
		if r.Decision != Allow {
			return errors.New("approval: denial cannot edit arguments")
		}
		if _, err := tool.ParseArguments(r.EditedArguments); err != nil {
			return fmt.Errorf("approval: edited arguments: %w", err)
		}
	}
	if r.Reason != strings.TrimSpace(r.Reason) {
		return errors.New("approval: denial reason has surrounding whitespace")
	}
	if r.Decision == Allow && r.Reason != "" {
		return errors.New("approval: approval cannot carry a denial reason")
	}
	return nil
}

type Rule struct {
	scope    RuleScope
	tool     tool.Ref
	source   framevalue.Digest
	subject  Subject
	decision Decision
}

func (r Rule) Scope() RuleScope                     { return r.scope }
func (r Rule) Tool() tool.Ref                       { return r.tool }
func (r Rule) SourceFingerprint() framevalue.Digest { return r.source }
func (r Rule) Subject() Subject                     { return r.subject }
func (r Rule) Decision() Decision                   { return r.decision }

type Query struct {
	SessionID         identity.SessionID
	ProjectDir        string
	Tool              tool.Ref
	SourceFingerprint framevalue.Digest
	Subject           string
}

type RememberRequest struct {
	Scope             Scope
	SessionID         identity.SessionID
	ProjectDir        string
	Tool              tool.Ref
	SourceFingerprint framevalue.Digest
	Subject           Subject
	Decision          Decision
}
