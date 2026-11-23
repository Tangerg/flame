package builtin

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"strings"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/core/jsonschema"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type askUserArgs struct {
	Questions []questionArg `json:"questions" jsonschema:"required,minItems=1" jsonschema_description:"The questions to ask the user."`
}

type questionArg struct {
	Question    string      `json:"question" jsonschema:"minLength=1" jsonschema_description:"The full question text."`
	Header      string      `json:"header,omitempty" jsonschema_description:"Short label that identifies the question, measured in Unicode characters."`
	Options     []optionArg `json:"options,omitempty" jsonschema_description:"Choices for a multiple-choice question. Omit for a free-text answer."`
	MultiSelect bool        `json:"multi_select,omitzero" jsonschema_description:"Allow the user to pick more than one option (only meaningful with options)."`
}

func (askUserArgs) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"questions": {MaxItems: new(uint64(questioncontract.MaximumFields))},
	}}
}

func (questionArg) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"header":  {MaxLength: new(uint64(transcript.MaximumQuestionHeaderCharacters))},
		"options": {MinItems: new(uint64(questioncontract.MinimumOptions)), MaxItems: new(uint64(questioncontract.MaximumOptions))},
	}}
}

type optionArg struct {
	Label       string `json:"label" jsonschema:"minLength=1" jsonschema_description:"The choice shown to the user."`
	Description string `json:"description,omitempty" jsonschema_description:"Optional one-line explanation of the choice."`
}

func (a askUserArgs) toFields() []transcript.QuestionField {
	fields := make([]transcript.QuestionField, len(a.Questions))
	for i, q := range a.Questions {
		fields[i] = transcript.QuestionField{Prompt: q.Question, Header: q.Header, Kind: questioncontract.Text}
		if len(q.Options) == 0 {
			continue
		}
		fields[i].Kind, fields[i].Multiple, fields[i].AllowCustom = questioncontract.Choice, q.MultiSelect, true
		for _, o := range q.Options {
			fields[i].Options = append(fields[i].Options, transcript.QuestionOption{Label: o.Label, Description: o.Description})
		}
	}
	return fields
}

func (a askUserArgs) arguments() (string, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("%s: encode arguments: %w", builtincontract.AskUser, err)
	}
	return string(b), nil
}

func NewAskUser(interrupt runs.InterruptFunc) (toolcontract.Tool, error) {
	return newInterruptTool[askUserArgs](
		toolcontract.FuncConfig{
			Name:        string(builtincontract.AskUser),
			Description: fmt.Sprintf("Ask the user 1 to %d questions and wait for their answers. Use this only when progress requires a decision, clarification, or information only the user can provide, not for routine progress updates. Give %d to %d `options` for a multiple-choice question and put the recommended option first; the user may still provide a custom answer. Omit `options` for free text, and set `multi_select` only when options are present and more than one may apply. Headers contain at most %d Unicode characters.", questioncontract.MaximumFields, questioncontract.MinimumOptions, questioncontract.MaximumOptions, transcript.MaximumQuestionHeaderCharacters),
		},
		interrupt,
		askUserPlan,
		askUserResolve,
	)
}

func askUserPlan(_ context.Context, args askUserArgs) (runs.Interrupt, error) {
	arguments, err := args.arguments()
	if err != nil {
		return nil, err
	}
	pending := runs.QuestionPrompt{
		ToolName:  string(builtincontract.AskUser),
		Arguments: arguments,
		Fields:    args.toFields(),
	}
	if err := pending.Validate(); err != nil {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: %w", builtincontract.AskUser, err))
	}
	return pending, nil
}

func askUserResolve(_ context.Context, args askUserArgs, resolution run.InterruptResolution) (string, error) {
	in := runs.QuestionPrompt{ToolName: string(builtincontract.AskUser), Fields: args.toFields()}
	return answerText(in, resolution.Question().Answers()), nil
}

func answerText(in runs.QuestionPrompt, answers [][]string) string {
	if len(in.Fields) == 1 {
		return strings.Join(answers[0], "\n")
	}
	var b strings.Builder
	for i, q := range in.Fields {
		label := q.Header
		if label == "" {
			label = q.Prompt
		}
		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(answers[i], ", "))
	}
	return strings.TrimSpace(b.String())
}
