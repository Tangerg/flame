package runs

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

func TestStartCommandCloneOwnsMutableInput(t *testing.T) {
	maxOutputTokens := int64(256)
	imageBytes := []byte("image")
	command := StartCommand{
		Options: &corechat.Options{MaxOutputTokens: &maxOutputTokens},
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Input: []transcript.ContentBlock{{
			Kind: transcript.ImageContent, MediaType: "image/png", Bytes: imageBytes,
		}},
	}
	owned := command.clone()

	imageBytes[0] = 'x'
	command.Input[0].MediaType = "image/jpeg"
	*command.Options.MaxOutputTokens = 512
	command.Capabilities.InterruptKinds[0] = interrupt.Approval

	if got := string(owned.Input[0].Bytes); got != "image" {
		t.Fatalf("owned image bytes = %q", got)
	}
	if got := owned.Input[0].MediaType; got != "image/png" {
		t.Fatalf("owned media type = %q", got)
	}
	if got := *owned.Options.MaxOutputTokens; got != 256 {
		t.Fatalf("owned max output tokens = %d", got)
	}
	if got := owned.Capabilities.InterruptKinds; !slices.Equal(got, []interrupt.Kind{interrupt.Question}) {
		t.Fatalf("owned interrupt kinds = %v", got)
	}
}

func TestResumeCommandCloneOwnsMutableInput(t *testing.T) {
	command := ResumeCommand{
		Responses: []ResumeResponse{
			{Kind: ApprovalResponseKind, Approval: &ApprovalResponse{Arguments: "{}"}},
			{Kind: QuestionResponseKind, Question: &QuestionResponse{Answers: [][]string{{"yes"}}}},
		},
		Input: []transcript.ContentBlock{{
			Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte("image"),
		}},
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
	}
	owned := command.clone()

	command.Responses[0].Approval.Arguments = "changed"
	command.Responses[1].Question.Answers[0][0] = "changed"
	command.Input[0].Bytes[0] = 'x'
	command.CallerCapabilities.InterruptKinds[0] = interrupt.Approval

	if got := owned.Responses[0].Approval.Arguments; got != "{}" {
		t.Fatalf("owned approval arguments = %q", got)
	}
	if got := owned.Responses[1].Question.Answers[0][0]; got != "yes" {
		t.Fatalf("owned question answer = %q", got)
	}
	if got := string(owned.Input[0].Bytes); got != "image" {
		t.Fatalf("owned input bytes = %q", got)
	}
	if got := owned.CallerCapabilities.InterruptKinds; !slices.Equal(got, []interrupt.Kind{interrupt.Question}) {
		t.Fatalf("owned caller capabilities = %v", got)
	}
}

func TestSteerCommandCloneOwnsMutableInput(t *testing.T) {
	command := SteerCommand{Input: []transcript.ContentBlock{{
		Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte("image"),
	}}}
	owned := command.clone()

	command.Input[0].MediaType = "image/jpeg"
	command.Input[0].Bytes[0] = 'x'

	if got := owned.Input[0].MediaType; got != "image/png" {
		t.Fatalf("owned media type = %q", got)
	}
	if got := string(owned.Input[0].Bytes); got != "image" {
		t.Fatalf("owned input bytes = %q", got)
	}
}

func TestStartExecutionValidateDelegatesCoreOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options corechat.Options
	}{
		{name: "temperature above maximum", options: corechat.Options{Temperature: testPointer(2.1)}},
		{name: "frequency penalty", options: corechat.Options{FrequencyPenalty: testPointer(2.1)}},
		{name: "presence penalty", options: corechat.Options{PresencePenalty: testPointer(-2.1)}},
		{name: "top k", options: corechat.Options{TopK: testPointer(int64(0))}},
		{name: "nan temperature", options: corechat.Options{Temperature: testPointer(math.NaN())}},
		{name: "infinite top p", options: corechat.Options{TopP: testPointer(math.Inf(1))}},
		{name: "negative infinite presence penalty", options: corechat.Options{PresencePenalty: testPointer(math.Inf(-1))}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			execution := validRootExecutionStart()
			execution.Options = &test.options
			err := execution.Validate()
			if !errors.Is(err, ErrInvalidRunOptions) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRunOptions", err)
			}
			if !errors.Is(err, corechat.ErrInvalidOptions) {
				t.Fatalf("Validate() error = %v, want wrapped chat.ErrInvalidOptions", err)
			}
		})
	}
}

func TestStartExecutionValidateKeepsModelSelectionOutsideOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options corechat.Options
	}{
		{name: "model", options: corechat.Options{Model: "model-inside-options"}},
		{name: "reasoning effort", options: corechat.Options{ReasoningEffort: "high"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution := validRootExecutionStart()
			execution.Options = &test.options
			err := execution.Validate()
			if !errors.Is(err, ErrInvalidRunOptions) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRunOptions", err)
			}
		})
	}
}

func TestStartExecutionValidateRequiresWorkingContext(t *testing.T) {
	t.Parallel()

	if err := (RootExecutionStart{}).Validate(); !errors.Is(err, ErrInputRequired) {
		t.Fatalf("Validate() error = %v, want ErrInputRequired", err)
	}
}

func TestStartExecutionValidateRequiresExactModelSelection(t *testing.T) {
	t.Parallel()

	execution := validRootExecutionStart()
	execution.ModelSelection = modelref.Selection{}
	err := execution.Validate()
	if err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("Validate() error = %v, want required model selection", err)
	}
}

func TestStartExecutionValidateRejectsNonCanonicalAdmissionPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		execution RootExecutionStart
	}{
		{name: "duplicate interrupt kind", execution: RootExecutionStart{
			InterruptKinds: []interrupt.Kind{
				interrupt.Approval,
				interrupt.Approval,
			},
		}},
		{name: "goal incarnation surrounding whitespace", execution: RootExecutionStart{
			GoalIncarnationID: " lease",
		}},
		{name: "goal incarnation interior whitespace", execution: RootExecutionStart{
			GoalIncarnationID: "goal incarnation",
		}},
		{name: "goal incarnation non-printing", execution: RootExecutionStart{
			GoalIncarnationID: "goal\u200bincarnation",
		}},
		{name: "goal incarnation oversized", execution: RootExecutionStart{
			GoalIncarnationID: strings.Repeat("界", goalref.MaximumIncarnationCharacters+1),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.execution.ModelSelection = mustSelection("provider", "model")
			test.execution.WorkingContext = validRootExecutionStart().WorkingContext
			if err := test.execution.Validate(); err == nil {
				t.Fatal("Validate accepted non-canonical admission policy")
			}
		})
	}
}

func validRootExecutionStart() RootExecutionStart {
	return RootExecutionStart{
		ModelSelection: mustSelection("provider", "model"),
		WorkingContext: []corechat.Message{
			corechat.NewUserMessage(corechat.NewTextPart("hello")),
		},
	}
}

func TestCancelCommandNormalizeReasonOwnsProductBoundary(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		reason     string
		wantReason string
		wantErr    bool
	}{
		{name: "omitted", wantReason: defaultCancelReason},
		{name: "whitespace", reason: "  user stopped  ", wantReason: "user stopped"},
		{
			name:       "unicode boundary",
			reason:     strings.Repeat("界", MaxCancellationReasonCharacters),
			wantReason: strings.Repeat("界", MaxCancellationReasonCharacters),
		},
		{
			name:    "over unicode boundary",
			reason:  strings.Repeat("界", MaxCancellationReasonCharacters+1),
			wantErr: true,
		},
		{name: "invalid UTF-8", reason: string([]byte{0xff}), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			command, err := (CancelCommand{RunID: "run_1", Reason: test.reason}).normalizeReason()
			if test.wantErr {
				if !errors.Is(err, ErrInvalidCancellationReason) {
					t.Fatalf("normalizeReason() error = %v, want ErrInvalidCancellationReason", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeReason(): %v", err)
			}
			if command.Reason != test.wantReason {
				t.Fatalf("Reason = %q, want %q", command.Reason, test.wantReason)
			}
		})
	}
}

func testPointer[T any](value T) *T {
	return &value
}
