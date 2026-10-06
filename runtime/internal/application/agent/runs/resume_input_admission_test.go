package runs

import (
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	corechat "github.com/Tangerg/scope/core/chat"
)

type resumeInputAdmitter struct {
	ModelAdmitter
	refusal   error
	calls     int
	selection modelref.Selection
}

func (a *resumeInputAdmitter) AdmitInput(selection modelref.Selection, messages []corechat.Message) error {
	a.calls++
	a.selection = selection
	if len(messages) != 1 {
		return errors.New("expected one materialized resume message")
	}
	return a.refusal
}

func TestResumeRejectsModelInputBeforeConsumingWait(t *testing.T) {
	pending := testApprovalPending("member_1", time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC))
	claims := 0
	effects := &fakeEffects{mutateClaim: func(*ExecutorCheckpoint) { claims++ }}
	sessions := &fakeRunSessions{
		sess: testsupport.MustRestoreSession(session.Snapshot{
			ID: "ses_1", Workspace: testsupport.MustWorkspace("/work"),
		}),
		pending: map[string]Pending{"run_1": pending},
	}
	control := &fakeExecutionPorts{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{block: true}, control, sessions, effects)
	refusal := errors.New("retained model does not accept this input")
	admitter := &resumeInputAdmitter{ModelAdmitter: coordinator.models, refusal: refusal}
	coordinator.models = admitter
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aA1sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	command := ResumeCommand{
		RunID:              "run_1",
		CallerCapabilities: run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: interrupt.Approval,
			Approval: &ApprovalResponse{Approved: true},
		}},
		Input: []transcript.ContentBlock{{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: image}},
	}
	for attempt := range 2 {
		result, err := coordinator.Resume(t.Context(), command)
		if !errors.Is(err, ErrUnsupportedMedia) || !errors.Is(err, refusal) {
			t.Fatalf("resume refusal = %v, want categorized model refusal", err)
		}
		if result.RunID != "" || claims != 0 || control.resumed || len(effects.openingSnapshot()) != 0 {
			t.Fatal("rejected input advanced the waiting execution")
		}
		if admitter.calls != attempt+1 || admitter.selection != testsupport.DefaultModelSelection() {
			t.Fatalf("resume did not admit against the retained root selection: %+v", admitter)
		}
		if !reflect.DeepEqual(sessions.pending["run_1"], pending) {
			t.Fatal("rejected input consumed the original waiting state")
		}
	}
}
