package builtin

import (
	"context"
	json "encoding/json/v2"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// TestAskUser_Validation: malformed args and an empty questions list are
// model-facing errors raised before the call parks (no HITL context needed).
func TestAskUser_Validation(t *testing.T) {
	tool, err := NewAskUser(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callTextTool(context.Background(), tool, `not json`); err == nil {
		t.Error("invalid JSON must error")
	}
	for _, arguments := range []string{
		`{"questions":[]}`,
		`{"questions":[{"question":""}]}`,
		`{"questions":[{"question":"Choose","header":"1234567890123"}]}`,
		`{"questions":[{"question":"Choose","options":[{"label":"one"}]}]}`,
		`{"questions":[{"question":"Choose","options":[{"label":""},{"label":"two"}]}]}`,
		`{"questions":[{"question":"Choose","multi_select":true}]}`,
	} {
		if _, err := callTextTool(context.Background(), tool, arguments); err == nil {
			t.Errorf("arguments outside the ask_user contract must error: %s", arguments)
		}
	}
}

// The schema admits questions the interrupt contract refuses. Nothing has parked
// when that refusal happens, so it must fail the call rather than lose the Run tree.
func TestAskUserQuestionRejectionFailsTheCallNotTheRun(t *testing.T) {
	tool, err := NewAskUser(func(context.Context, string, runs.Interrupt) (interrupt.Resolution, error) {
		t.Fatal("a refused question must not park")
		return interrupt.Resolution{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{
		`{"questions":[{"question":"   "}]}`,
		`{"questions":[{"question":"Choose","options":[{"label":"Yes"},{"label":"Yes"}]}]}`,
		`{"questions":[{"question":"Choose","options":[{"label":"Yes "},{"label":"No"}]}]}`,
	} {
		_, err := callTextTool(t.Context(), tool, arguments)
		requireDefiniteFailure(t, err, "ask_user "+arguments)
	}
}

func TestAskUserKeepsOptionsOpenToARealUserAnswer(t *testing.T) {
	var captured runs.Interrupt
	tool, err := NewAskUser(func(_ context.Context, _ string, request runs.Interrupt) (interrupt.Resolution, error) {
		captured = request
		return interrupt.Resolution{Answers: [][]string{{"another database"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := callTextTool(context.Background(), tool, `{
		"questions":[{
			"question":"Pick a database",
			"options":[{"label":"Postgres"},{"label":"SQLite"}]
		}]
	}`)

	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result != "another database" {
		t.Fatalf("result = %q", result)
	}
	if captured.Question == nil || len(captured.Question.Fields) != 1 ||
		!captured.Question.Fields[0].AllowCustom {
		t.Fatalf("question interrupt = %#v, want custom answer enabled", captured.Question)
	}
}

// TestAnswerText covers the result rendering: a single question returns just its
// answer; multiple questions return labeled lines; multi-select joins values.
func TestAnswerText(t *testing.T) {
	single := runs.QuestionPrompt{Fields: []runs.QuestionFieldSpec{{Prompt: "Proceed?"}}}
	if got := answerText(single, [][]string{{"yes"}}); got != "yes" {
		t.Errorf("single = %q, want %q", got, "yes")
	}

	multi := runs.QuestionPrompt{Fields: []runs.QuestionFieldSpec{
		{Prompt: "Pick a DB", Header: "DB"},
		{Prompt: "Pick langs", Header: "Langs", Multiple: true},
	}}
	answers := [][]string{{"sqlite"}, {"go", "rust"}}
	got := answerText(multi, answers)
	if !strings.Contains(got, "DB: sqlite") || !strings.Contains(got, "Langs: go, rust") {
		t.Errorf("multi = %q, want labeled lines incl. \"DB: sqlite\" and \"Langs: go, rust\"", got)
	}
}

// Struct tags cannot name constants, so the advertised schema restates the
// question limits the transcript owns. This pins that projection to its owner:
// a changed limit must change what the model is told.
func TestAskUserSchemaProjectsTheTranscriptLimits(t *testing.T) {
	tool, err := NewAskUser(nil)
	if err != nil {
		t.Fatal(err)
	}
	type property struct {
		MaxItems  *int `json:"maxItems"`
		MaxLength *int `json:"maxLength"`
	}
	type definition struct {
		Properties map[string]property `json:"properties"`
	}
	var schema struct {
		Properties map[string]property   `json:"properties"`
		Defs       map[string]definition `json:"$defs"`
	}
	if err := json.Unmarshal(tool.Definition().InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	var question definition
	for name, candidate := range schema.Defs {
		if strings.HasSuffix(name, "_questionArg") {
			question = candidate
		}
	}
	for _, limit := range []struct {
		name string
		got  *int
		want int
	}{
		{"questions.maxItems", schema.Properties["questions"].MaxItems, transcript.MaximumQuestionFields},
		{"header.maxLength", question.Properties["header"].MaxLength, transcript.MaximumQuestionHeaderCharacters},
		{"options.maxItems", question.Properties["options"].MaxItems, transcript.MaximumQuestionOptions},
	} {
		if limit.got == nil || *limit.got != limit.want {
			t.Errorf("ask_user schema %s = %v, want %d", limit.name, limit.got, limit.want)
		}
	}
}
