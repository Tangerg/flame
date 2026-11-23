package dispatch

import (
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func registerApprovalConditions(s *Shapes) {
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ApprovalRule](),
		Rules: []ConditionalRule{{
			When:     whenEquals("scope", string(protocol.ApprovalRuleScopeProject)),
			Required: []string{"dir"},
		}, {
			When:      whenEquals("scope", string(protocol.ApprovalRuleScopeSession)),
			Forbidden: []string{"dir"},
		}, {
			When:      whenEquals("scope", string(protocol.ApprovalRuleScopeGlobal)),
			Forbidden: []string{"dir"},
		}},
	})
}

func registerApprovalSubjectUnion(s *Shapes) {
	s.union(UnionSpec{GoType: typeOf[protocol.ApprovalSubject](), Variants: []VariantSpec{
		{Tag: string(protocol.ApprovalSubjectAll)},
		{Tag: string(protocol.ApprovalSubjectExact), Required: []string{"value"}},
		{Tag: string(protocol.ApprovalSubjectGlob), Required: []string{"value"}},
	}})
}

func registerApprovalValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ToolRef](), Constraints: mcpRemoteToolIdentity()})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SetApprovalRuleRequest](), Constraints: resourceIdentity("sessionId")})

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ApprovalRule](),
		Constraints: append(requiredResourceIdentity("id"),
			FieldConstraint{Field: "modelName", Kind: ConstraintPattern, Value: tool.ModelNamePattern()}),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ListApprovalRulesRequest](), Constraints: resourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ListApprovalRulesResult](),
		Constraints: []FieldConstraint{{
			Field: "rules", Kind: ConstraintMaxItems, Limit: approval.MaximumVisibleRules,
		}},
	})
	nonEmpty[protocol.ForgetApprovalRuleRequest](s, "id")
}
