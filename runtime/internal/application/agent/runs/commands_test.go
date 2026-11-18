package runs

import (
	"errors"

	"strings"
	"testing"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
	corechat "github.com/Tangerg/scope/core/chat"
)

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
	execution.ModelSelection = modelchoice.Selection{}
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
			InterruptTypes: []run.InterruptType{run.InterruptApproval, run.InterruptApproval},
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
			test.execution.ModelSelection = testsupport.MustModelSelection("provider", "model")
			test.execution.WorkingContext = validRootExecutionStart().WorkingContext
			if err := test.execution.Validate(); err == nil {
				t.Fatal("Validate accepted non-canonical admission policy")
			}
		})
	}
}

func validRootExecutionStart() RootExecutionStart {
	return RootExecutionStart{
		ModelSelection: testsupport.MustModelSelection("provider", "model"),
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
