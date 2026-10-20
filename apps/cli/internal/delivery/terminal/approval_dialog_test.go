package terminal

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
)

func TestApprovalPaneRoutesInputToTheLastPresentedForm(t *testing.T) {
	transcript := testTranscriptView(t)
	pane := approvalPane{
		theme: transcript.theme, glyphs: transcript.glyphs,
		detail: kit.NewParagraph("", transcript.theme.Text),
	}
	pane.view = kit.Transcript{
		Content: &pane.preview, Scroll: &pane.scroll,
		Theme: transcript.theme, Glyphs: transcript.glyphs,
	}
	form := func(done func()) *kit.Form {
		choice := "only"
		selectField := headless.NewSelect(headless.SelectConfig[string]{Same: headless.Equal[string], Value: headless.Bind(&choice), Rows: 1})
		selectField.SetOptions([]headless.Option[string]{{Label: "Only", Value: "only"}})
		controller := headless.NewForm(selectField)
		controller.Done = done
		return kit.NewForm(kit.FormConfig{Theme: transcript.theme, Glyphs: transcript.glyphs, Controller: controller})
	}
	oldCalls, currentCalls := 0, 0
	pane.form = form(func() { oldCalls++ })
	pane.Focus(true)
	root := headless.NewRoot(&pane)
	surface := grid.NewSurface(80, 20)
	root.Draw(surface.View())

	pane.form = form(func() { currentCalls++ })
	root.Handle(input.Key{Code: input.Enter})
	if oldCalls != 1 || currentCalls != 0 {
		t.Fatalf("stale frame routed to old=%d current=%d forms", oldCalls, currentCalls)
	}

	root.Draw(surface.View())
	root.Handle(input.Key{Code: input.Enter})
	if oldCalls != 1 || currentCalls != 1 {
		t.Fatalf("current frame routed to old=%d current=%d forms", oldCalls, currentCalls)
	}
}

func TestReplacedApprovalFormCannotMutateTheCurrentDraft(t *testing.T) {
	transcript := testTranscriptView(t)
	application := &app{loop: &program.Runtime{}, transcript: transcript}
	application.dialogs.approvalPane = approvalPane{
		theme: transcript.theme, glyphs: transcript.glyphs,
		detail: kit.NewParagraph("", transcript.theme.Text),
	}
	application.dialogs.approvalPane.view = kit.Transcript{
		Content: &application.dialogs.approvalPane.preview, Scroll: &application.dialogs.approvalPane.scroll,
		Theme: transcript.theme, Glyphs: transcript.glyphs,
	}
	application.dialogs.interruptReview = &interruptReview{}
	application.dialogs.approval = &conversation.Approval{}
	application.setApprovalForm(approvalAllowOnce)
	retired := application.dialogs.approvalDraft
	application.dialogs.approvalPane.Focus(true)
	root := headless.NewRoot(&application.dialogs.approvalPane)
	surface := grid.NewSurface(80, 20)
	root.Draw(surface.View())

	application.dialogs.interruptReview = &interruptReview{}
	application.dialogs.approval = &conversation.Approval{}
	application.dialogs.approvalDraft = &approvalDecisionDraft{}
	application.setApprovalForm(approvalAllowOnce)
	root.Handle(input.Key{Code: input.Down})
	if retired.choice != approvalDenyOnce {
		t.Fatalf("retired draft choice = %q, want deny once", retired.choice)
	}
	if current := application.dialogs.approvalDraft.choice; current != approvalAllowOnce {
		t.Fatalf("current draft choice = %q after stale input, want allow once", current)
	}
}

func TestApprovalChoiceMapsEveryDecisionAndRememberScope(t *testing.T) {
	tests := []struct {
		name     string
		action   approvalAction
		decision protocol.ApprovalDecision
		remember protocol.RememberScopeKind
	}{
		{name: "allow once", action: approvalAllowOnce, decision: protocol.ApprovalApprove},
		{name: "allow session", action: approvalAllowSession, decision: protocol.ApprovalApprove, remember: protocol.RememberSession},
		{name: "allow project", action: approvalAllowProject, decision: protocol.ApprovalApprove, remember: protocol.RememberProject},
		{name: "allow global", action: approvalAllowGlobal, decision: protocol.ApprovalApprove, remember: protocol.RememberGlobal},
		{name: "deny once", action: approvalDenyOnce, decision: protocol.ApprovalDeny},
		{name: "deny session", action: approvalDenySession, decision: protocol.ApprovalDeny, remember: protocol.RememberSession},
		{name: "deny project", action: approvalDenyProject, decision: protocol.ApprovalDeny, remember: protocol.RememberProject},
		{name: "deny global", action: approvalDenyGlobal, decision: protocol.ApprovalDeny, remember: protocol.RememberGlobal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer, ok := test.action.Answer()
			if !ok {
				t.Fatalf("approval action %q was not recognized", test.action)
			}
			if got := approvalActionFromAnswer(answer); got != test.action {
				t.Fatalf("answer round trip = %q, want %q", got, test.action)
			}
			if answer.Decision != test.decision || answer.Remember != test.remember {
				t.Fatalf("approval action %q = %+v", test.action, answer)
			}
		})
	}
	if answer, ok := approvalEditArgs.Answer(); ok || answer != (conversation.ApprovalAnswer{}) {
		t.Fatalf("editor choice projected a runtime answer: %+v, %v", answer, ok)
	}
}

func TestApprovalOptionsKeepOneShotActionsStableAndExposeRememberedDenials(t *testing.T) {
	t.Parallel()
	values := func(options []headless.Option[approvalAction]) []approvalAction {
		return slices.Collect(func(yield func(approvalAction) bool) {
			for _, option := range options {
				if !yield(option.Value) {
					return
				}
			}
		})
	}
	if got, want := values(approvalOptions(false)), []approvalAction{
		approvalAllowOnce, approvalDenyOnce, approvalEditArgs,
	}; !slices.Equal(got, want) {
		t.Fatalf("one-shot approval options = %v, want %v", got, want)
	}
	if got, want := values(approvalOptions(true)), []approvalAction{
		approvalAllowOnce, approvalAllowSession, approvalAllowProject, approvalAllowGlobal,
		approvalDenyOnce, approvalDenySession, approvalDenyProject, approvalDenyGlobal, approvalEditArgs,
	}; !slices.Equal(got, want) {
		t.Fatalf("rememberable approval options = %v, want %v", got, want)
	}
}

func TestApprovalDefaultSelectsEveryConfiguredRememberScope(t *testing.T) {
	tests := []struct {
		preference settings.RememberPreference
		want       approvalAction
	}{
		{preference: settings.RememberNone, want: approvalAllowOnce},
		{preference: settings.RememberSession, want: approvalAllowSession},
		{preference: settings.RememberProject, want: approvalAllowProject},
		{preference: settings.RememberGlobal, want: approvalAllowGlobal},
	}
	for _, test := range tests {
		if got := defaultApprovalAction(test.preference); got != test.want {
			t.Errorf("defaultApprovalAction(%q) = %q, want %q", test.preference, got, test.want)
		}
	}
}

func TestApprovalChoiceNormalizationRespectsRememberability(t *testing.T) {
	t.Parallel()
	for _, option := range approvalOptions(true) {
		for _, rememberable := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/rememberable=%t", option.Value, rememberable), func(t *testing.T) {
				got, ok := option.Value.Normalize(rememberable)
				if !ok {
					t.Fatalf("known action %q was rejected", option.Value)
				}
				if rememberable && got != option.Value {
					t.Fatalf("changed available action to %q", got)
				}
				answer, answers := option.Value.Answer()
				normalized, normalizedAnswers := got.Answer()
				if answers != normalizedAnswers || answer.Decision != normalized.Decision {
					t.Fatalf("normalization changed intent: %+v to %+v", answer, normalized)
				}
				if !rememberable && normalized.Remember != "" {
					t.Fatalf("retained unsupported scope: %+v", normalized)
				}
			})
		}
	}
	for _, rememberable := range []bool{true, false} {
		if action, ok := approvalAction("unexpected").Normalize(rememberable); ok || action != "" {
			t.Fatalf("unknown action became valid: %q, %t", action, ok)
		}
	}
}

func TestApprovalFormPreservesDenialWhenRememberingBecomesUnavailable(t *testing.T) {
	view := testTranscriptView(t)
	application := &app{loop: &program.Runtime{}, transcript: view, status: &statusView{}, execution: executionState{conversation: conversation.New()}}
	application.dialogs.approvalPane = approvalPane{theme: view.theme, glyphs: view.glyphs}
	application.dialogs.approval = &conversation.Approval{Rememberable: true}
	application.setApprovalForm(approvalDenyGlobal)
	application.dialogs.approval.Rememberable = false
	application.setApprovalForm(application.dialogs.approvalDraft.choice)
	if action := application.dialogs.approvalDraft.choice; action != approvalDenyOnce {
		t.Fatalf("rebuilt denial became %q", action)
	}
	previous := application.dialogs.approvalForm
	application.setApprovalForm(approvalAction("invalid"))
	previous.Done()
	if application.dialogs.approvalForm != nil || application.dialogs.approvalPane.form != nil {
		t.Fatal("unknown action left an answerable approval form")
	}
}

func TestUnknownApprovalAnswerCannotBecomeAnAction(t *testing.T) {
	for _, answer := range []conversation.ApprovalAnswer{
		{},
		{Decision: protocol.ApprovalApprove, Remember: protocol.RememberScopeKind("unknown")},
	} {
		if action := approvalActionFromAnswer(answer); action != "" {
			t.Fatalf("invalid answer became %q", action)
		}
	}
}
