package builtin

import (
	json "encoding/json/v2"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/goals"
	plandomain "github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/scope/core/jsonschema"
)

func TestArgumentEnumsMatchTheirVocabulary(t *testing.T) {
	t.Parallel()
	lsp, lspErr := jsonschema.For[lspInput]()
	report, reportErr := jsonschema.For[reportArgs]()
	plan, planErr := jsonschema.For[setArgs]()
	for _, err := range []error{lspErr, reportErr, planErr} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name     string
		schema   jsonschema.Schema
		declared []string
		input    func(string) any
	}{
		{
			name: "lsp operation", schema: lsp,
			declared: []string{string(LSPDefinition), string(LSPReferences), string(LSPImplementation),
				string(LSPHover), string(LSPIncomingCalls), string(LSPOutgoingCalls),
				string(LSPDocumentSymbols), string(LSPWorkspaceSymbols), string(LSPDiagnostics)},
			input: func(value string) any { return lspInput{Operation: LSPOperation(value)} },
		},
		{
			name: "goal report outcome", schema: report,
			declared: []string{string(goals.OutcomeCompleted), string(goals.OutcomeBlocked)},
			input:    func(value string) any { return reportArgs{Outcome: goals.Outcome(value)} },
		},
		{
			name: "plan step status", schema: plan,
			declared: []string{string(plandomain.StatusPending), string(plandomain.StatusInProgress), string(plandomain.StatusCompleted)},
			input: func(value string) any {
				return setArgs{Steps: []stepArg{{Description: "Work", Status: plandomain.Status(value)}}}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, got := range advertisedEnums(t, test.schema) {
				if !slices.Equal(got, test.declared) {
					t.Fatalf("schema offers %v, owner declares %v", got, test.declared)
				}
			}
			for _, value := range append(test.declared, "unsupported") {
				raw, err := json.Marshal(test.input(value))
				if err != nil {
					t.Fatal(err)
				}
				err = test.schema.Validate(raw)
				if value == "unsupported" {
					if !errors.Is(err, jsonschema.ErrValidation) {
						t.Fatalf("unknown value admitted: %v", err)
					}
				} else if err != nil {
					t.Fatalf("owner value %q refused: %v", value, err)
				}
			}
		})
	}
}

func advertisedEnums(t *testing.T, schema jsonschema.Schema) [][]string {
	t.Helper()
	var document any
	if err := json.Unmarshal(schema.JSON(), &document); err != nil {
		t.Fatal(err)
	}
	var declarations [][]string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "enum" {
					raw, err := json.Marshal(child)
					if err != nil {
						t.Fatal(err)
					}
					var words []string
					if err := json.Unmarshal(raw, &words); err != nil {
						t.Fatal(err)
					}
					declarations = append(declarations, words)
				} else {
					visit(child)
				}
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(document)
	if len(declarations) == 0 {
		t.Fatal("schema does not advertise its closed argument vocabulary")
	}
	return declarations
}
