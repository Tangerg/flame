package agentexec

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestBoundDiagnosticKeepsTextValidBoundedAndMarked(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		maximum int
		want    string
	}{
		{name: "short text survives verbatim", value: "boom", maximum: 16, want: "boom"},
		{name: "surrounding whitespace is dropped", value: "  boom\n", maximum: 16, want: "boom"},
		{name: "whitespace only yields nothing", value: " \n\t ", maximum: 16, want: ""},
		{name: "invalid bytes are removed", value: "bo" + string([]byte{0xff}) + "om", maximum: 16, want: "boom"},
		{name: "invalid bytes only yield nothing", value: string([]byte{0xff, 0xfe}), maximum: 16, want: ""},
		{name: "exactly at the bound survives", value: "abcdefgh", maximum: 8, want: "abcdefgh"},
		{name: "past the bound is marked", value: "abcdefghi", maximum: 8, want: "abcde" + diagnosticEllipsis},
		{name: "a cut never splits a rune", value: strings.Repeat("界", 4), maximum: 8, want: "界" + diagnosticEllipsis},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := boundDiagnostic(test.value, test.maximum)
			if got != test.want {
				t.Fatalf("boundDiagnostic(%q, %d) = %q, want %q", test.value, test.maximum, got, test.want)
			}
			if len(got) > test.maximum {
				t.Errorf("result is %d bytes, maximum is %d", len(got), test.maximum)
			}
			if !utf8.ValidString(got) {
				t.Errorf("result is not valid UTF-8")
			}
		})
	}
}

func TestDelegateFailureTellsTheModelItWasTruncated(t *testing.T) {
	call := corechat.ToolCall{ID: "call_1", Name: "delegate"}
	text, _ := delegateFailureModelResult(call, strings.Repeat("界", maximumDelegateDiagnosticBytes)).Output.Text()
	if !strings.HasSuffix(text, diagnosticEllipsis) {
		t.Errorf("truncated Delegate diagnostic is unmarked: %q", text[max(len(text)-16, 0):])
	}
	if !utf8.ValidString(text) {
		t.Error("truncated Delegate diagnostic is not valid UTF-8")
	}

	for _, empty := range []string{"", "   ", string([]byte{0xff})} {
		text, _ := delegateFailureModelResult(call, empty).Output.Text()
		if !strings.Contains(text, "Interaction operation failed") {
			t.Errorf("diagnostic %q lost its substitute wording: %q", empty, text)
		}
	}
}

func TestExecutorDiagnosticBoundsPersistedText(t *testing.T) {
	if got := executorDiagnostic(nil); got != "" {
		t.Errorf("executorDiagnostic(nil) = %q", got)
	}
	got := executorDiagnostic(errors.New(strings.Repeat("界", maximumExecutorDiagnosticBytes)))
	if len(got) > maximumExecutorDiagnosticBytes || !utf8.ValidString(got) {
		t.Errorf("persisted diagnostic is %d bytes, valid=%v", len(got), utf8.ValidString(got))
	}
}
