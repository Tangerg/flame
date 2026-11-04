package model

import (
	toolcontract "github.com/Tangerg/scope/core/tool"

	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestToolOutputRenderingUsesVisibleContent(t *testing.T) {
	image := mustBudgetImage(t, []byte("small image"))
	mediaOutput := budgetToolImage(image).Parts[0].ToolResult.Output
	hiddenDetails := jsontext.Value(`{"hidden":"` + strings.Repeat("x", 5000) + `"}`)
	textOutput := toolcontract.NewTextOutput("visible")
	textOutput.Details = hiddenDetails
	mediaOutput.Details = hiddenDetails
	for _, test := range []struct {
		name   string
		output toolcontract.Output
		want   string
	}{
		{"text", textOutput, "visible"},
		{"media", mediaOutput, "image result " + mediaMarker},
		{"details only", toolcontract.Output{Details: jsontext.Value(`{"value":"visible"}`)}, `{"value":"visible"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := json.Marshal(test.output)
			if err != nil {
				t.Fatal(err)
			}
			if got := renderToolOutput(test.output); got != test.want {
				t.Fatalf("rendered output = %q, want %q", got, test.want)
			}
			after, err := json.Marshal(test.output)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("rendering mutated source output")
			}
		})
	}
}
