package maintenance

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestToolOutputMaintenanceUsesVisibleContent(t *testing.T) {
	image := mustBudgetImage(t, []byte("small image"))
	mediaOutput := budgetToolImage(image).Parts[0].ToolResult.Output
	details := json.RawMessage(`{"hidden":"` + strings.Repeat("x", 5000) + `"}`)
	for _, tc := range []struct {
		name   string
		output chat.ToolOutput
		want   string
	}{
		{"text", chat.NewTextToolOutput("visible"), "visible"},
		{"media", mediaOutput, "image result " + transcriptMediaMarker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.output.Details = details
			before, err := json.Marshal(tc.output)
			if err != nil {
				t.Fatal(err)
			}
			if got := renderToolOutput(tc.output); got != tc.want {
				t.Errorf("transcript includes hidden details: %q", got)
			}
			trimmed, changed := trimToolOutput(tc.output)
			if changed || !reflect.DeepEqual(trimmed, tc.output) {
				t.Error("hidden details triggered loss of visible content")
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
	output := chat.ToolOutput{Details: json.RawMessage(`{"value":"visible"}`)}
	if got := renderToolOutput(output); got != string(output.Details) {
		t.Fatalf("details-only transcript = %q", got)
	}
	if trimmed, changed := trimToolOutput(output); changed || !reflect.DeepEqual(trimmed, output) {
		t.Fatal("small details-only output changed")
	}
	output.Details = json.RawMessage(`{"value":"` + strings.Repeat("x", 5000) + `"}`)
	trimmed, changed := trimToolOutput(output)
	text, textual := trimmed.Text()
	if !changed || !textual || len(text) > ladderResultCap || !strings.Contains(text, "trimmed on compaction") {
		t.Fatal("large details-only output was not bounded")
	}
}
