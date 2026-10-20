package domain

import (
	"errors"
	"math"

	"github.com/Tangerg/flame/runtime/model"
)

var (
	ErrInvalidPolicy    = errors.New("model router: invalid policy")
	ErrRevisionConflict = errors.New("model router: policy revision changed")
	ErrRevisionLimit    = errors.New("model router: policy revision exhausted")
)

type Rule struct {
	Use       model.Use              `json:"use"`
	Selection model.Selection        `json:"selection"`
	Ceiling   *model.SelectionLimits `json:"ceiling,omitzero"`
}

type Snapshot struct {
	Revision int64  `json:"revision" jsonschema:"minimum=1"`
	Rules    []Rule `json:"rules" jsonschema:"maxItems=3"`
}

type Policy struct {
	revision int64
	rules    []Rule
}

func NewPolicy() Policy { return Policy{revision: 1, rules: []Rule{}} }

func Restore(snapshot Snapshot) (Policy, error) {
	if snapshot.Revision < 1 || snapshot.Rules == nil || len(snapshot.Rules) > 3 {
		return Policy{}, ErrInvalidPolicy
	}
	seen := make(map[model.Use]bool, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		if seen[rule.Use] {
			return Policy{}, ErrInvalidPolicy
		}
		if err := rule.Use.Validate(); err != nil {
			return Policy{}, errors.Join(ErrInvalidPolicy, err)
		}
		if err := rule.Selection.ValidateExact(); err != nil {
			return Policy{}, errors.Join(ErrInvalidPolicy, err)
		}
		if rule.Ceiling != nil {
			if err := (model.SelectionContext{Use: rule.Use, Limits: rule.Ceiling}).Validate(); err != nil {
				return Policy{}, errors.Join(ErrInvalidPolicy, err)
			}
		}
		seen[rule.Use] = true
	}
	return Policy{revision: snapshot.Revision, rules: cloneRules(snapshot.Rules)}, nil
}

func (p Policy) Snapshot() Snapshot {
	return Snapshot{Revision: p.revision, Rules: cloneRules(p.rules)}
}

func (p Policy) Replace(expected int64, rules []Rule) (Policy, error) {
	if p.revision < 1 {
		return Policy{}, ErrInvalidPolicy
	}
	if expected != p.revision {
		return Policy{}, ErrRevisionConflict
	}
	if p.revision == math.MaxInt64 {
		return Policy{}, ErrRevisionLimit
	}
	return Restore(Snapshot{Revision: p.revision + 1, Rules: rules})
}

func (p Policy) Select(context model.SelectionContext, proposed model.Selection) (model.Selection, error) {
	if p.revision < 1 {
		return model.Selection{}, ErrInvalidPolicy
	}
	if err := context.Validate(); err != nil {
		return model.Selection{}, err
	}
	if err := proposed.ValidateExact(); err != nil {
		return model.Selection{}, err
	}
	for _, rule := range p.rules {
		if rule.Use != context.Use {
			continue
		}
		if rule.Ceiling == nil || within(context.Limits, *rule.Ceiling) {
			return rule.Selection, nil
		}
	}
	return proposed, nil
}

func within(limits *model.SelectionLimits, ceiling model.SelectionLimits) bool {
	return limits != nil && limits.InputKind == ceiling.InputKind && limits.MaxInputBytes <= ceiling.MaxInputBytes && limits.MaxOutputTokens <= ceiling.MaxOutputTokens
}

func cloneRules(rules []Rule) []Rule {
	result := make([]Rule, len(rules))
	for i, rule := range rules {
		result[i] = rule
		if rule.Ceiling != nil {
			result[i].Ceiling = new(*rule.Ceiling)
		}
	}
	return result
}
