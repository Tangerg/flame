package dispatch

import "github.com/Tangerg/flame/packages/go-sdk/protocol"

func registerGoalConditions(s *Shapes) {
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.StartGoalRequest](), Rules: modelSelectionRules()})

	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.Goal](),
		Rules: []ConditionalRule{{
			Required: []string{"createdAt", "updatedAt"},
		}, {
			When:      whenEquals("status", string(protocol.GoalActive)),
			Forbidden: []string{"reason"},
		}, {
			When:      whenEquals("status", string(protocol.GoalCompleting)),
			Forbidden: []string{"reason"},
		}, {
			When:          whenEquals("status", string(protocol.GoalPaused)),
			Required:      []string{"reason"},
			AllowedValues: allowedGoalReasonCodes(protocol.GoalReasonStoppedByUser, protocol.GoalReasonRuntimeRestarted, protocol.GoalReasonRunStartFailed, protocol.GoalReasonAwaitingInput, protocol.GoalReasonTerminalOutcomeMissing, protocol.GoalReasonRunNotCompleted),
		}, {
			When:          whenEquals("status", string(protocol.GoalBlocked)),
			Required:      []string{"reason"},
			AllowedValues: allowedGoalReasonCodes(protocol.GoalReasonBlockedByModel),
		}},
	})
	goalReasonRules := []ConditionalRule{
		{
			When:      whenEquals("code", string(protocol.GoalReasonRunNotCompleted)),
			Required:  []string{"runOutcome"},
			Forbidden: []string{"explanation"},
			AllowedValues: []AllowedValueSet{{Field: "runOutcome", Values: []string{
				string(protocol.OutcomeTimedOut), string(protocol.OutcomeFailed), string(protocol.OutcomeCanceled), string(protocol.OutcomeLost),
			}}},
		}, {
			When:      whenEquals("code", string(protocol.GoalReasonBlockedByModel)),
			Required:  []string{"explanation"},
			Forbidden: []string{"runOutcome"},
		},
	}
	for _, code := range []protocol.GoalReasonCode{protocol.GoalReasonStoppedByUser, protocol.GoalReasonRuntimeRestarted, protocol.GoalReasonRunStartFailed, protocol.GoalReasonAwaitingInput, protocol.GoalReasonTerminalOutcomeMissing} {
		goalReasonRules = append(goalReasonRules, ConditionalRule{
			When:      whenEquals("code", string(code)),
			Forbidden: []string{"runOutcome", "explanation"},
		})
	}
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.GoalReason](), Rules: goalReasonRules})
}

func registerGoalValues(s *Shapes) {
	const nonBlankObjective = `\S`
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.Goal](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective},
		}...), modelSelectionIdentities()...),
	})
	nonNegative[protocol.GoalUsage](s, "runs", "costUsd", "steps")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.StartGoalRequest](),
		Constraints: append(append(requiredResourceIdentity("sessionId"), []FieldConstraint{
			{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective},
		}...), modelSelectionIdentities()...),
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.GoalRequest](), Constraints: requiredResourceIdentity("sessionId")})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateGoalRequest](),
		Constraints: append(requiredResourceIdentity("sessionId"),
			FieldConstraint{Field: "objective", Kind: ConstraintPattern, Value: nonBlankObjective}),
	})
}
