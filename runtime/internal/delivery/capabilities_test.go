package delivery

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

type conditionalParameters struct {
	Enabled      bool    `json:"enabled,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	OptionalMode *string `json:"optionalMode,omitempty"`
	Nested       *struct {
		Name string `json:"name,omitempty"`
	} `json:"nested,omitempty"`
}

func TestFieldConditionMatchesTypedParameters(t *testing.T) {
	t.Parallel()

	parameters := reflect.ValueOf(conditionalParameters{
		Enabled: true,
		Mode:    "files",
		Nested: &struct {
			Name string `json:"name,omitempty"`
		}{Name: "worker"},
	})
	tests := []struct {
		name      string
		condition FieldCondition
		want      bool
	}{
		{name: "present", condition: FieldCondition{Field: "enabled", Operator: OperatorPresent}, want: true},
		{name: "equals", condition: FieldCondition{Field: "mode", Operator: OperatorEquals, Value: "files"}, want: true},
		{name: "nested", condition: FieldCondition{Field: "nested.name", Operator: OperatorEquals, Value: "worker"}, want: true},
		{name: "empty", condition: FieldCondition{Field: "missing", Operator: OperatorPresent}},
		{name: "different", condition: FieldCondition{Field: "mode", Operator: OperatorEquals, Value: "history"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.condition.matches(parameters); got != test.want {
				t.Fatalf("matches = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFieldConditionEqualsRequiresAStringTarget(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"enabled", "optionalMode"} {
		err := ValidateFieldCondition("fixture", reflect.TypeFor[conditionalParameters](), FieldCondition{
			Field: field, Operator: OperatorEquals, Value: "value",
		})
		if err == nil || !strings.Contains(err.Error(), "requires a string field") {
			t.Fatalf("ValidateFieldCondition(%q) error = %v, want string-target requirement", field, err)
		}
	}
}

func TestCapabilityGateUsesHostFactsAndClientOptIn(t *testing.T) {
	for _, test := range []struct {
		name       string
		method     Name
		parameters any
		git        bool
		subagents  bool
		refused    bool
	}{
		{name: "git unavailable", method: WorkspaceChangesList, parameters: protocol.WorkspaceQuery{}, refused: true},
		{name: "git available", method: WorkspaceChangesList, parameters: protocol.WorkspaceQuery{}, git: true},
		{name: "history rollback without git", method: SessionsRollback, parameters: protocol.RollbackSessionRequest{SessionID: "ses_1"}},
		{name: "file restore without git", method: SessionsRollback, parameters: protocol.RollbackSessionRequest{SessionID: "ses_1", RestoreType: protocol.RestoreFiles}, refused: true},
		{name: "complete restore without git", method: SessionsRollback, parameters: protocol.RollbackSessionRequest{SessionID: "ses_1", RestoreType: protocol.RestoreBoth}, refused: true},
		{name: "file restore with git", method: SessionsRollback, parameters: protocol.RollbackSessionRequest{SessionID: "ses_1", RestoreType: protocol.RestoreFiles}, git: true},
		{name: "root Runs need no opt-in", method: RunsList, parameters: protocol.ListRunsRequest{}},
		{name: "descendants require opt-in", method: RunsList, parameters: protocol.ListRunsRequest{IncludeDescendants: true}, refused: true},
		{name: "descendants opted in", method: RunsList, parameters: protocol.ListRunsRequest{IncludeDescendants: true}, subagents: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			endpoint := mustNewEndpoint(t, &Handler{features: featureAvailability{git: test.git}}, EndpointConfig{})
			meta, found := Contract().Lookup(test.method)
			if !found {
				t.Fatalf("unknown method %q", test.method)
			}
			ctx := t.Context()
			if test.subagents {
				ctx = WithRequestMeta(ctx, protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}}}})
			}
			failure := endpoint.enforceCapabilities(ctx, meta, test.parameters)
			if test.refused {
				if !errors.Is(failure, protocol.ErrCapabilityNotNeg) {
					t.Fatalf("capability failure = %v", failure)
				}
			} else if failure != nil {
				t.Fatalf("allowed request failed: %v", failure)
			}
		})
	}
}
