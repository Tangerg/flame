package approval

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type ruleSet []Rule

// Decide evaluates visible rules against q using the domain's specificity and
// deny-on-conflict policy.
func Decide(rules []Rule, q Query) (Decision, bool, error) {
	return ruleSet(rules).decide(q)
}

// ValidateVisibleRules verifies that rules is one complete relation visible
// from the requested session and project. Stores filter this relation for
// efficiency; Domain rechecks scope membership and identity uniqueness before
// the relation can affect authorization or escape through a read model.
func ValidateVisibleRules(rules []Rule, sessionID, projectDir string) error {
	if len(rules) > MaximumVisibleRules {
		return fmt.Errorf(
			"%w: got %d rules, maximum %d",
			ErrRuleCapacity,
			len(rules),
			MaximumVisibleRules,
		)
	}
	if sessionID != "" {
		if err := resourceid.ValidateSession(sessionID); err != nil {
			return fmt.Errorf("%w: session: %v", ErrInvalidRule, err)
		}
	}
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("approval: visible rule %d: %w", index, err)
		}
		scopeKey, visible := rule.Scope.key(sessionID, projectDir)
		if !visible || rule.ScopeKey != scopeKey {
			return fmt.Errorf(
				"%w: visible rule %d with scope %q and key %q is outside the requested scope",
				ErrInvalidRule,
				index,
				rule.Scope,
				rule.ScopeKey,
			)
		}
		if _, duplicate := seen[rule.ID]; duplicate {
			return fmt.Errorf("%w: duplicate visible rule identity %q", ErrInvalidRule, rule.ID)
		}
		seen[rule.ID] = struct{}{}
	}
	return nil
}

// NewRule constructs one durable rule and derives its deterministic identity.
func NewRule(scope Scope, scopeKey string, ref tool.Ref, fingerprint string, subject Subject, decision Decision) (Rule, error) {
	rule := Rule{
		Scope: scope, ScopeKey: scopeKey, Tool: ref, SourceFingerprint: fingerprint,
		Subject: subject, Decision: decision,
	}
	rule.ID = rule.stableID()
	if err := rule.Validate(); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// Validate protects the durable approval vocabulary and deterministic rule
// identity at every store boundary.
func (r Rule) Validate() error {
	if !r.Scope.Valid() {
		return fmt.Errorf("%w: unknown scope %q", ErrInvalidRule, r.Scope)
	}
	switch r.Scope {
	case ScopeSession:
		if err := resourceid.ValidateSession(r.ScopeKey); err != nil {
			return fmt.Errorf("%w: session scope: %v", ErrInvalidRule, err)
		}
	case ScopeProject:
		if strings.TrimSpace(r.ScopeKey) == "" {
			return fmt.Errorf("%w: scope %q requires a key", ErrInvalidRule, r.Scope)
		}
	case ScopeGlobal:
		if r.ScopeKey != "" {
			return fmt.Errorf("%w: global scope cannot carry a key", ErrInvalidRule)
		}
	}
	if err := r.Tool.ValidateFingerprint(r.SourceFingerprint); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRule, err)
	}
	if !r.Decision.Valid() {
		return fmt.Errorf("%w: unknown decision %q", ErrInvalidRule, r.Decision)
	}
	if err := r.Subject.Validate(); err != nil {
		return err
	}
	if r.ID == "" || r.ID != r.stableID() {
		return fmt.Errorf("%w: identity %q does not match rule contents", ErrInvalidRule, r.ID)
	}
	return nil
}

// specificity encodes the conflict policy: narrower scope beats wider scope,
// then exact subject beats glob, and glob beats whole-tool.
func (r Rule) specificity() int {
	score := 0
	switch r.Scope {
	case ScopeSession:
		score = 300
	case ScopeProject:
		score = 200
	case ScopeGlobal:
		score = 100
	}
	switch r.Subject.Type {
	case SubjectGlob:
		score++
	case SubjectExact:
		score += 2
	}
	return score
}

// Validate verifies the complete identity of one tool-call policy query.
func (q Query) Validate() error {
	if q.SessionID != "" {
		if err := resourceid.ValidateSession(q.SessionID); err != nil {
			return fmt.Errorf("%w: session: %v", ErrInvalidQuery, err)
		}
	}
	if err := q.Tool.ValidateFingerprint(q.SourceFingerprint); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidQuery, err)
	}
	return nil
}

// decide picks the strongest visible rule; equally specific disagreements
// resolve to Deny so a remembered deny cannot be canceled by a peer allow.
func (r ruleSet) decide(q Query) (Decision, bool, error) {
	if err := q.Validate(); err != nil {
		return "", false, err
	}
	if err := ValidateVisibleRules(r, q.SessionID, q.ProjectDir); err != nil {
		return "", false, err
	}
	best := -1
	var verdict Decision
	conflict := false
	for _, rule := range r {
		if rule.Tool != q.Tool || rule.SourceFingerprint != q.SourceFingerprint || !rule.Subject.matches(q.Subject) {
			continue
		}
		switch score := rule.specificity(); {
		case score > best:
			best, verdict, conflict = score, rule.Decision, false
		case score == best && rule.Decision != verdict:
			conflict = true
		}
	}
	if best < 0 {
		return "", false, nil
	}
	if conflict {
		return Deny, true, nil
	}
	return verdict, true, nil
}

// key refuses empty session/project keys so rules cannot leak across scopes.
func (s Scope) key(sessionID, projectDir string) (string, bool) {
	switch s {
	case ScopeSession:
		return sessionID, sessionID != ""
	case ScopeProject:
		return projectDir, projectDir != ""
	case ScopeGlobal:
		return "", true
	default:
		return "", false
	}
}

// Rule derives and validates the durable rule represented by r.
func (r RememberRequest) Rule() (Rule, error) {
	if r.SessionID != "" {
		if err := resourceid.ValidateSession(r.SessionID); err != nil {
			return Rule{}, fmt.Errorf("%w: session: %v", ErrInvalidRule, err)
		}
	}
	key, ok := r.Scope.key(r.SessionID, r.ProjectDir)
	if !ok {
		return Rule{}, fmt.Errorf("%w: scope %q has no usable key", ErrInvalidRule, r.Scope)
	}
	if err := r.Tool.ValidateFingerprint(r.SourceFingerprint); err != nil {
		return Rule{}, fmt.Errorf("%w: %w", ErrInvalidRule, err)
	}
	if !r.Decision.Valid() {
		return Rule{}, fmt.Errorf("%w: unknown decision %q", ErrInvalidRule, r.Decision)
	}
	return NewRule(r.Scope, key, r.Tool, r.SourceFingerprint, r.Subject, r.Decision)
}

// stableID makes re-remembering the same rule an upsert and supplies a durable
// handle for forgetting it later.
func (r Rule) stableID() string {
	return "rule_" + fingerprint.Strings(string(r.Scope), r.ScopeKey, r.Tool.String(), string(r.Subject.Type), r.Subject.Value)
}
