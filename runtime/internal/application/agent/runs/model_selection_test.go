package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

type modelSelectorFunc func(context.Context, modelref.Selection, *chat.Request) (modelref.Selection, error)

func (f modelSelectorFunc) SelectModel(ctx context.Context, proposed modelref.Selection, request *chat.Request) (modelref.Selection, error) {
	return f(ctx, proposed, request)
}

type selectedInputAdmitter struct {
	target modelref.Selection
	calls  int
}

func (*selectedInputAdmitter) AdmitSelection(modelref.Selection) error { return nil }
func (a *selectedInputAdmitter) AdmitInput(selection modelref.Selection, _ []chat.Message) error {
	a.calls++
	if selection != a.target {
		return errors.New("input admitted before the selected model")
	}
	return nil
}

func TestFreshStartBindsSelectedModelAndKeepsSessionPreferenceAndOriginalInput(t *testing.T) {
	proposed := testsupport.DefaultModelSelection()
	selected := testsupport.MustModelSelection("provider", "selected-model")
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_selected", ExecutorID: "exec_selected"}}
	effects := &fakeEffects{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{}, control, new(fakeRunSessions), effects)
	count := 0
	coordinator.modelSelector = modelSelectorFunc(func(_ context.Context, proposal modelref.Selection, request *chat.Request) (modelref.Selection, error) {
		count++
		if proposal != proposed || len(request.Messages) != 1 || request.Messages[0].Text() != "question" || request.Options.MaxOutputTokens == nil || *request.Options.MaxOutputTokens != 128 {
			t.Error("selection lost original proposal/context/output reservation")
		}
		request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("forged input"))
		return selected, nil
	})
	admitter := &selectedInputAdmitter{target: selected}
	coordinator.models = admitter
	result, err := coordinator.Start(t.Context(), StartCommand{Acceptance: &CommandIdentity{Key: "selected-opening", Fingerprint: testsupport.Digest("selected opening").String()}, NewSession: &SessionOpening{SessionID: "ses_selected", RunID: "run_selected", CWD: "/selected", Title: "Selected"}, ModelSelection: proposed, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "question"}}, Options: &chat.Options{MaxOutputTokens: new(int64(128))}})
	if err != nil {
		t.Fatal(err)
	}
	consumeEvents(result.Events)
	if count != 1 || admitter.calls != 2 || control.started.ModelSelection != selected || control.validated.ModelSelection != selected || control.started.WorkingContext[0].Text() != "question" {
		t.Fatalf("selected execution changed: selections=%d admissions=%d started=%+v validated=%+v", count, admitter.calls, control.started, control.validated)
	}
	opening := effects.opening()
	admitted, ok := opening.Admission()
	if !ok || admitted.ModelSelection != selected {
		t.Fatal("accepted Run did not own selected model")
	}
	initial, ok := opening.InitialSession()
	if !ok || initial.Selection() != proposed {
		t.Fatal("execution routing rewrote the Session preference")
	}
}

func TestSelectionFailureRefusesBeforeStagingAndDurableOpening(t *testing.T) {
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_selected", ExecutorID: "exec_selected"}}
	effects := &fakeEffects{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{}, control, new(fakeRunSessions), effects)
	refusal := errors.New("selection unavailable")
	coordinator.modelSelector = modelSelectorFunc(func(context.Context, modelref.Selection, *chat.Request) (modelref.Selection, error) {
		return modelref.Selection{}, refusal
	})
	_, err := coordinator.Start(t.Context(), StartCommand{Acceptance: &CommandIdentity{Key: "selected-opening", Fingerprint: testsupport.Digest("selected opening").String()}, NewSession: &SessionOpening{SessionID: "ses_selected", RunID: "run_selected", CWD: "/selected", Title: "Selected"}, ModelSelection: testsupport.DefaultModelSelection(), Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "question"}}})
	if !errors.Is(err, refusal) || control.started.SessionID != "" || control.validated.SessionID != "" {
		t.Fatalf("failed selection staged execution: %+v %+v %v", control.started, control.validated, err)
	}
	if _, admitted := effects.opening().Admission(); admitted {
		t.Fatal("failed selection opened a durable Run")
	}
}

func TestAcceptedStartRecoveryDoesNotSelectAgain(t *testing.T) {
	accepted := validCommandAcceptance()
	reader := &recordingAcceptanceReader{record: CommandAcceptanceRecord{Found: true, CurrentRun: nil, Acceptance: accepted}}
	coordinator := mustNewCoordinator(Dependencies{CommandAcceptances: reader, ModelSelector: modelSelectorFunc(func(context.Context, modelref.Selection, *chat.Request) (modelref.Selection, error) {
		t.Fatal("accepted command reran selection")
		return modelref.Selection{}, errors.New("unexpected selection")
	})})
	result, err := coordinator.Start(t.Context(), StartCommand{SessionID: accepted.SessionID, Acceptance: &CommandIdentity{Key: accepted.Key, Fingerprint: accepted.Fingerprint}})
	if err != nil || !result.Reopened || result.RunID != accepted.RunID {
		t.Fatalf("original acceptance lost: %+v %v", result, err)
	}
}
