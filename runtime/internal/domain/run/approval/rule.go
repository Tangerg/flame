package approval

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
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
		if err := identity.ValidateSession(sessionID); err != nil {
			return fmt.Errorf("%w: session: %v", ErrInvalidRule, err)
		}
	}
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		if !rule.scope.kind.Valid() {
			return fmt.Errorf("%w: visible rule %d was never constructed", ErrInvalidRule, index)
		}
		if !rule.scope.visibleFrom(sessionID, projectDir) {
			return fmt.Errorf(
				"%w: visible rule %d with scope %q and key %q is outside the requested scope",
				ErrInvalidRule,
				index,
				rule.scope.kind,
				rule.scope.key,
			)
		}
		if _, duplicate := seen[rule.ID()]; duplicate {
			return fmt.Errorf("%w: duplicate visible rule identity %q", ErrInvalidRule, rule.ID())
		}
		seen[rule.ID()] = struct{}{}
	}
	return nil
}

// RuleScope is where a remembered rule applies: a session rule names its
// Session, a project rule its directory, and a global rule nothing.
type RuleScope struct {
	kind Scope
	key  string
}

func GlobalScope() RuleScope { return RuleScope{kind: ScopeGlobal} }

func SessionScope(sessionID string) (RuleScope, error) {
	if err := identity.ValidateSession(sessionID); err != nil {
		return RuleScope{}, fmt.Errorf("%w: session scope: %v", ErrInvalidRule, err)
	}
	return RuleScope{kind: ScopeSession, key: sessionID}, nil
}

func ProjectScope(dir string) (RuleScope, error) {
	if strings.TrimSpace(dir) == "" {
		return RuleScope{}, fmt.Errorf("%w: project scope requires a directory", ErrInvalidRule)
	}
	return RuleScope{kind: ScopeProject, key: dir}, nil
}

// RestoreRuleScope reads a stored kind and key back through its constructor.
func RestoreRuleScope(kind Scope, key string) (RuleScope, error) {
	switch kind {
	case ScopeSession:
		return SessionScope(key)
	case ScopeProject:
		return ProjectScope(key)
	case ScopeGlobal:
		if key != "" {
			return RuleScope{}, fmt.Errorf("%w: global scope cannot carry a key", ErrInvalidRule)
		}
		return GlobalScope(), nil
	default:
		return RuleScope{}, fmt.Errorf("%w: unknown scope %q", ErrInvalidRule, kind)
	}
}

func (s RuleScope) Kind() Scope { return s.kind }

// Key is the Session identity or project directory the scope names; it is
// empty only for the global scope.
func (s RuleScope) Key() string { return s.key }

// visibleFrom refuses empty session and project keys so rules cannot leak
// across scopes.
func (s RuleScope) visibleFrom(sessionID, projectDir string) bool {
	switch s.kind {
	case ScopeSession:
		return sessionID != "" && s.key == sessionID
	case ScopeProject:
		return projectDir != "" && s.key == projectDir
	default:
		return s.kind == ScopeGlobal
	}
}

// NewRule constructs one durable rule; its identity is derived, never chosen.
func NewRule(scope RuleScope, ref tool.Ref, source framevalue.Digest, subject Subject, decision Decision) (Rule, error) {
	if !scope.kind.Valid() {
		return Rule{}, fmt.Errorf("%w: scope is required", ErrInvalidRule)
	}
	if err := ref.ValidateFingerprint(source); err != nil {
		return Rule{}, fmt.Errorf("%w: %w", ErrInvalidRule, err)
	}
	if !decision.Valid() {
		return Rule{}, fmt.Errorf("%w: unknown decision %q", ErrInvalidRule, decision)
	}
	if err := subject.Validate(); err != nil {
		return Rule{}, err
	}
	return Rule{scope: scope, tool: ref, source: source, subject: subject, decision: decision}, nil
}

// specificity encodes the conflict policy: narrower scope beats wider scope,
// then exact subject beats glob, and glob beats whole-tool.
// specificity ranks a rule by scope first and subject second: any narrower
// scope outranks any broader one, and within a scope an exact subject outranks
// a glob, which outranks matching every invocation.
func (r Rule) specificity() [2]int {
	rank := func(order []string, value string) int { return slices.Index(order, value) + 1 }
	return [2]int{
		rank([]string{string(ScopeGlobal), string(ScopeProject), string(ScopeSession)}, string(r.scope.kind)),
		rank([]string{string(SubjectAll), string(SubjectGlob), string(SubjectExact)}, string(r.subject.Type)),
	}
}

// Validate verifies the complete identity of one tool-call policy query.
func (q Query) Validate() error {
	if q.SessionID != "" {
		if err := identity.ValidateSession(q.SessionID); err != nil {
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
	var best [2]int
	var verdict Decision
	conflict := false
	for _, rule := range r {
		if rule.tool != q.Tool || rule.source != q.SourceFingerprint || !rule.subject.matches(q.Subject) {
			continue
		}
		switch score := rule.specificity(); {
		case cmp.Or(cmp.Compare(score[0], best[0]), cmp.Compare(score[1], best[1])) > 0:
			best, verdict, conflict = score, rule.decision, false
		case score == best && rule.decision != verdict:
			conflict = true
		}
	}
	if best == ([2]int{}) {
		return "", false, nil
	}
	if conflict {
		return Deny, true, nil
	}
	return verdict, true, nil
}

// Rule derives the durable rule represented by r; the scope requested names
// which of the Session or project keys the rule is stored under.
func (r RememberRequest) Rule() (Rule, error) {
	var scope RuleScope
	var err error
	switch r.Scope {
	case ScopeSession:
		scope, err = SessionScope(r.SessionID)
	case ScopeProject:
		scope, err = ProjectScope(r.ProjectDir)
	case ScopeGlobal:
		scope = GlobalScope()
	default:
		err = fmt.Errorf("%w: unknown scope %q", ErrInvalidRule, r.Scope)
	}
	if err != nil {
		return Rule{}, err
	}
	return NewRule(scope, r.Tool, r.SourceFingerprint, r.Subject, r.Decision)
}

// ID is derived from the rule's scope, tool and subject, so re-remembering the
// same rule is an upsert and the identity is a durable handle for forgetting it.
func (r Rule) ID() string {
	return "rule_" + identity.DigestStrings(string(r.scope.kind), r.scope.key, r.tool.String(), string(r.subject.Type), r.subject.Value).String()
}
