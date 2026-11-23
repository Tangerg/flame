package dispatch

import (
	"slices"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func registerItemUnions(s *Shapes) {
	itemIdentityFields := []string{"id", "runId", "status"}
	createdItemFields := slices.Concat(itemIdentityFields, []string{"createdAt"})
	toolItemFields := slices.Concat(itemIdentityFields, []string{"startedAt"})
	s.union(UnionSpec{
		GoType: typeOf[protocol.Item](),
		Variants: []VariantSpec{
			{
				Tag: string(protocol.ItemTypeUserMessage), Required: slices.Concat(createdItemFields, []string{"content"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeAgentMessage), Required: createdItemFields, Optional: []string{"phase", "content"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusRunning, protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
			}, {
				Tag: string(protocol.ItemTypeReasoning), Required: createdItemFields, Optional: []string{"text", "redacted"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusRunning, protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
			}, {
				Tag: string(protocol.ItemTypeQuestion), Required: slices.Concat(createdItemFields, []string{"question"}),
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			}, {
				Tag: string(protocol.ItemTypeToolCall), Required: slices.Concat(toolItemFields, []string{"tool"}), Optional: []string{"finishedAt", "durationMillis", "safetyClass", "approvalDecision", "error"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusRunning, protocol.ItemStatusCompleted, protocol.ItemStatusIncomplete),
			}, {
				Tag: string(protocol.ItemTypeCompaction), Required: slices.Concat(createdItemFields, []string{"summary"}), Optional: []string{"droppedMessages"},
				AllowedValues: allowedItemStatuses(protocol.ItemStatusCompleted),
			},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.ItemDelta](),
		Variants: []VariantSpec{
			{Tag: string(protocol.DeltaContent), Required: []string{"text"}},
			{Tag: string(protocol.DeltaReasoning), Required: []string{"text"}},
			{Tag: string(protocol.DeltaToolArguments), Required: []string{"argumentsTextDelta"}},
			{Tag: string(protocol.DeltaToolOutput), Required: []string{"text"}},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.CapabilityRequirement](),
		Variants: []VariantSpec{
			{Tag: string(protocol.RequirementFeature), Required: []string{"name"}},
			{Tag: string(protocol.RequirementInterruptType), Required: []string{"name"}},
			{Tag: string(protocol.RequirementRuntimeTopic), Required: []string{"name"}},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.ItemListScope](),
		Variants: []VariantSpec{
			{Tag: string(protocol.ItemScopeSession), Required: []string{"sessionId"}},
			{Tag: string(protocol.ItemScopeRun), Required: []string{"runId"}, Optional: []string{"includeDescendants"}},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.ContentBlock](),
		Variants: []VariantSpec{
			{Tag: string(protocol.ContentBlockText), Required: []string{"text"}},
			{Tag: string(protocol.ContentBlockImage), Required: []string{"mime", "data"}},
		},
	})

	s.union(UnionSpec{
		GoType: typeOf[protocol.QuestionField](),
		Variants: []VariantSpec{
			{Tag: string(protocol.QuestionFieldText), Required: []string{"prompt"}, Optional: []string{"header"}},
			{Tag: string(protocol.QuestionFieldChoice), Required: []string{"prompt", "options"}, Optional: []string{"header", "multiple", "allowCustom"}},
		},
	})
}

func registerItemConditions(s *Shapes) {
	terminalToolRules := []ConditionalRule{{
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusCompleted)},
		},
		Required:  []string{"finishedAt", "durationMillis"},
		Forbidden: []string{"error"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusIncomplete)},
		},
		Required: []string{"finishedAt"},
	}}
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.ArtifactItem](),
		Rules:  terminalToolRules,
	})

	runtimeItemRules := slices.Concat(terminalToolRules, []ConditionalRule{{
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeToolCall)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusRunning)},
		},
		Forbidden: []string{"finishedAt", "durationMillis"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeAgentMessage)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusRunning)},
		},
		Forbidden: []string{"phase"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeAgentMessage)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusCompleted)},
		},
		Required: []string{"phase", "content"},
	}, {
		When: []delivery.FieldCondition{
			{Field: "type", Operator: delivery.OperatorEquals, Value: string(protocol.ItemTypeReasoning)},
			{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusCompleted)},
		},
		Required: []string{"text"},
	}})
	for _, visible := range []struct {
		kind   protocol.ItemType
		fields []string
	}{
		{protocol.ItemTypeAgentMessage, []string{"phase", "content"}},
		{protocol.ItemTypeReasoning, []string{"text"}},
	} {
		runtimeItemRules = append(runtimeItemRules, ConditionalRule{
			When: []delivery.FieldCondition{
				{Field: "type", Operator: delivery.OperatorEquals, Value: string(visible.kind)},
				{Field: "status", Operator: delivery.OperatorEquals, Value: string(protocol.ItemStatusIncomplete)},
			}, Required: visible.fields,
		})
	}
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.Item](),
		Rules:  runtimeItemRules,
	})
}

func builtInNames() []string {
	names := make([]string, 0, len(builtincontract.Names()))
	for _, name := range builtincontract.Names() {
		names = append(names, string(name))
	}
	return names
}

func registerToolRefUnion(s *Shapes) {
	s.union(UnionSpec{GoType: typeOf[protocol.ToolRef](), Variants: []VariantSpec{
		{Tag: string(protocol.ToolRefBuiltIn), Required: []string{"name"}, AllowedValues: []AllowedValueSet{{Field: "name", Values: builtInNames()}}},
		{Tag: string(protocol.ToolRefMCP), Required: []string{"server", "name"}},
	}})
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.SetApprovalRuleRequest](), Rules: []ConditionalRule{
		{When: whenEquals("scope", "global"), Forbidden: []string{"sessionId"}},
		{When: whenEquals("scope", "session"), Required: []string{"sessionId"}},
		{When: whenEquals("scope", "project"), Required: []string{"sessionId"}},
	}})
}

func registerFeedbackValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.SessionAnnotation](),
		Constraints: append(append(resourceIdentity("sessionId"), resourceIdentity("runId")...), resourceIdentity("itemId")...),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.FeedbackRequest](),
		Constraints: append(
			append(append(resourceIdentity("sessionId"), resourceIdentity("runId")...), resourceIdentity("itemId")...),
			FieldConstraint{Field: "text", Kind: ConstraintPattern, Value: `\S`},
		),
	})
}

func registerToolValues(s *Shapes) {
	nonEmpty[protocol.InvokeToolRequest](s, "name")
}
