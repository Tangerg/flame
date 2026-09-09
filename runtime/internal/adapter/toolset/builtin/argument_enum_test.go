package builtin

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestArgumentEnumsMatchTheirVocabulary compares the two spellings of a closed
// tool argument. The model reads the jsonschema tag; the tool reads the
// constants, and a struct tag cannot be derived from them. An added value is
// otherwise invisible to the model, and a renamed one is offered and then
// rejected on arrival.
func TestArgumentEnumsMatchTheirVocabulary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     any
		field    string
		declared []string
	}{
		{
			name:  "lsp operation",
			args:  lspInput{},
			field: "Operation",
			declared: []string{
				string(LSPDefinition), string(LSPReferences), string(LSPImplementation),
				string(LSPHover), string(LSPIncomingCalls), string(LSPOutgoingCalls),
				string(LSPDocumentSymbols), string(LSPWorkspaceSymbols), string(LSPDiagnostics),
			},
		},
		{
			name:     "goal report outcome",
			args:     reportArgs{},
			field:    "Outcome",
			declared: []string{string(reportOutcomeCompleted), string(reportOutcomeBlocked)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			field, ok := reflect.TypeOf(test.args).FieldByName(test.field)
			if !ok {
				t.Fatalf("%s has no field %s", reflect.TypeOf(test.args), test.field)
			}
			offered := schemaEnumValues(field.Tag.Get("jsonschema"))
			if !slices.Equal(offered, test.declared) {
				t.Fatalf("schema offers %v, the tool accepts %v", offered, test.declared)
			}
		})
	}
}

func schemaEnumValues(tag string) []string {
	var values []string
	for _, part := range strings.Split(tag, ",") {
		if value, found := strings.CutPrefix(part, "enum="); found {
			values = append(values, value)
		}
	}
	return values
}
