package dispatch

import (
	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func registerHookConditions(s *Shapes) {
	hookRules := []ConditionalRule{{
		RequiredAny: []string{"command", "inject"},
	}, {
		When:      whenPresent("command"),
		Required:  []string{"failurePolicy"},
		Forbidden: []string{"inject"},
	}, {
		When:      whenPresent("inject"),
		Forbidden: []string{"command", "matcher", "timeoutMillis", "failurePolicy"},
	}}
	requiredHookEvents := make([]string, 0)
	for _, event := range hookdata.RequiredFailureEvents() {
		requiredHookEvents = append(requiredHookEvents, string(event))
	}
	hookRules = append(hookRules, ConditionalRule{
		When:          whenEquals("failurePolicy", string(hookdata.FailureRequired)),
		AllowedValues: []AllowedValueSet{{Field: "event", Values: requiredHookEvents}},
	})
	for _, event := range []protocol.HookEvent{protocol.HookEventUserPromptSubmit, protocol.HookEventSessionStart, protocol.HookEventPreCompact, protocol.HookEventStop, protocol.HookEventNotification} {
		hookRules = append(hookRules, ConditionalRule{
			When:      whenEquals("event", string(event)),
			Forbidden: []string{"matcher"},
		})
	}
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.HookInfo](), Rules: hookRules})
}

func registerHookValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.HookInfo](),
		Constraints: []FieldConstraint{
			{Field: "source", Kind: ConstraintNonEmpty},
			{Field: "timeoutMillis", Kind: ConstraintNonNegative},
			{Field: "timeoutMillis", Kind: ConstraintMaximum, Limit: hookdata.MaxTimeoutMillis},
		},
	})
	nonEmpty[protocol.SetHookTrustRequest](s, "projectRoot")
}
