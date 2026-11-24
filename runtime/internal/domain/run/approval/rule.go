package approval

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
)

type ruleSet []Rule

func Decide(rules []Rule, q Query) (Decision, bool, error) {
	return ruleSet(rules).decide(q)
}

func ValidateVisibleRules(rules []Rule, sessionID identity.SessionID, workspaceDir string) error {
	if len(rules) > MaximumVisibleRules {
		return fmt.Errorf(
			"%w: got %d rules, maximum %d",
			ErrRuleCapacity,
			len(rules),
			MaximumVisibleRules,
		)
	}
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		if !rule.scope.kind.Valid() {
			return fmt.Errorf("%w: visible rule %d was never constructed", ErrInvalidRule, index)
		}
		if !rule.scope.visibleFrom(sessionID, workspaceDir) {
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

type RuleScope struct {
	kind Scope
	key  string
}

func GlobalScope() RuleScope { return RuleScope{kind: ScopeGlobal} }

func SessionScope(sessionID identity.SessionID) (RuleScope, error) {
	if err := sessionID.Validate(); err != nil {
		return RuleScope{}, fmt.Errorf("%w: session scope: %w", ErrInvalidRule, err)
	}
	return RuleScope{kind: ScopeSession, key: sessionID.String()}, nil
}

func WorkspaceScope(dir string) (RuleScope, error) {
	if strings.TrimSpace(dir) == "" {
		return RuleScope{}, fmt.Errorf("%w: workspace scope requires a directory", ErrInvalidRule)
	}
	return RuleScope{kind: ScopeWorkspace, key: dir}, nil
}

func RestoreRuleScope(kind Scope, key string) (RuleScope, error) {
	switch kind {
	case ScopeSession:
		sessionID, err := identity.ParseSession(key)
		if err != nil {
			return RuleScope{}, fmt.Errorf("%w: session scope: %w", ErrInvalidRule, err)
		}
		return SessionScope(sessionID)
	case ScopeWorkspace:
		return WorkspaceScope(key)
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

func (s RuleScope) Key() string { return s.key }

// Empty session and workspace keys are refused so rules cannot leak across scopes.
func (s RuleScope) visibleFrom(sessionID identity.SessionID, workspaceDir string) bool {
	switch s.kind {
	case ScopeSession:
		return !sessionID.IsZero() && s.key == sessionID.String()
	case ScopeWorkspace:
		return workspaceDir != "" && s.key == workspaceDir
	default:
		return s.kind == ScopeGlobal
	}
}

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

func (r Rule) specificity() [2]int {
	rank := func(order []string, value string) int { return slices.Index(order, value) + 1 }
	return [2]int{
		rank([]string{string(ScopeGlobal), string(ScopeWorkspace), string(ScopeSession)}, string(r.scope.kind)),
		rank([]string{string(SubjectAll), string(SubjectGlob), string(SubjectExact)}, string(r.subject.Type)),
	}
}

func (q Query) Validate() error {
	if err := q.Tool.ValidateFingerprint(q.SourceFingerprint); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidQuery, err)
	}
	return nil
}

// Equally specific disagreements resolve to Deny so a remembered deny cannot be
// canceled by a peer allow.
func (r ruleSet) decide(q Query) (Decision, bool, error) {
	if err := q.Validate(); err != nil {
		return "", false, err
	}
	if err := ValidateVisibleRules(r, q.SessionID, q.WorkspaceDir); err != nil {
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

func (r RememberRequest) Rule() (Rule, error) {
	var scope RuleScope
	var err error
	switch r.Scope {
	case ScopeSession:
		scope, err = SessionScope(r.SessionID)
	case ScopeWorkspace:
		scope, err = WorkspaceScope(r.WorkspaceDir)
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

func (r Rule) ID() string {
	return "rule_" + identity.DigestStrings(string(r.scope.kind), r.scope.key, r.tool.String(), string(r.subject.Type), r.subject.Value).String()
}
