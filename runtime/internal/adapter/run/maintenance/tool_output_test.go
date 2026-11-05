package maintenance

import (
	toolcontract "github.com/Tangerg/scope/core/tool"

	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestToolOutputMaintenanceUsesVisibleContent(t *testing.T) {
	image := mustBudgetImage(t, []byte("small image"))
	mediaOutput := budgetToolImage(image).Parts[0].ToolResult.Output
	details := jsontext.Value(`{"hidden":"` + strings.Repeat("x", 5000) + `"}`)
	for _, tc := range []struct {
		name   string
		output toolcontract.Output
	}{
		{"text", toolcontract.NewTextOutput("visible")},
		{"media", mediaOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.output.Details = details
			visible := tc.output.Clone()
			visible.Details = nil
			before, err := json.Marshal(tc.output)
			if err != nil {
				t.Fatal(err)
			}
			if encodedVisibleToolOutputBytes(tc.output) != encodedVisibleToolOutputBytes(visible) {
				t.Error("hidden details counted as model-visible content")
			}
			after, err := json.Marshal(tc.output)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("maintenance mutated source output")
			}
		})
	}
}

func TestToolOutputMaintenancePreservesDetailsOnlyFallback(t *testing.T) {
	output := toolcontract.Output{Details: jsontext.Value(`{"value":"visible"}`)}
	small := encodedVisibleToolOutputBytes(output)
	output.Details = jsontext.Value(`{"value":"` + strings.Repeat("x", 5000) + `"}`)
	if encodedVisibleToolOutputBytes(output) <= small+4000 {
		t.Fatal("details-only model fallback was not counted")
	}
}
