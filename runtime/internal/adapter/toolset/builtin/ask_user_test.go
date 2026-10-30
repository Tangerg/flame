package builtin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"testing"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/jsonschema"
	toolcontract "github.com/Tangerg/scope/core/tool"
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
	tool, err := NewAskUser(func(context.Context, toolcontract.Invocation, string, runs.Interrupt) (run.InterruptResolution, *toolcontract.InputRequest, error) {
		t.Fatal("a refused question must not park")
		return run.InterruptResolution{}, nil, nil
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
	tool, err := NewAskUser(func(_ context.Context, _ toolcontract.Invocation, _ string, request runs.Interrupt) (run.InterruptResolution, *toolcontract.InputRequest, error) {
		captured = request
		return testsupport.QuestionAnswer(t, [][]string{{"another database"}}), nil, nil
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

func TestAskUserSchemaProjectsTheTranscriptLimits(t *testing.T) {
	executable, err := NewAskUser(nil)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.Parse(executable.Definition().InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name                       string
		questions, options, header int
		valid                      bool
	}{
		{"maximum questions", questioncontract.MaximumFields, questioncontract.MinimumOptions, 0, true},
		{"excess questions", questioncontract.MaximumFields + 1, questioncontract.MinimumOptions, 0, false},
		{"minimum options", 1, questioncontract.MinimumOptions, 0, true},
		{"too few options", 1, questioncontract.MinimumOptions - 1, 0, false},
		{"maximum options", 1, questioncontract.MaximumOptions, 0, true},
		{"excess options", 1, questioncontract.MaximumOptions + 1, 0, false},
		{"maximum Unicode header", 1, questioncontract.MinimumOptions, transcript.MaximumQuestionHeaderCharacters, true},
		{"excess Unicode header", 1, questioncontract.MinimumOptions, transcript.MaximumQuestionHeaderCharacters + 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := askUserArgs{}
			for i := range test.questions {
				question := questionArg{Question: fmt.Sprintf("Question %d", i), Header: strings.Repeat("界", test.header)}
				for j := range test.options {
					question.Options = append(question.Options, optionArg{Label: fmt.Sprintf("Option %d", j)})
				}
				args.Questions = append(args.Questions, question)
			}
			raw, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			schemaErr := schema.Validate(raw)
			_, _, ownerErr := askUserPlan(t.Context(), args)
			if test.valid {
				if schemaErr != nil || ownerErr != nil {
					t.Fatalf("valid question refused: schema=%v owner=%v", schemaErr, ownerErr)
				}
			} else if !errors.Is(schemaErr, jsonschema.ErrValidation) || ownerErr == nil {
				t.Fatalf("invalid question admitted: schema=%v owner=%v", schemaErr, ownerErr)
			}
		})
	}
}
