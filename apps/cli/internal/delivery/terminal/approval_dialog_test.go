package terminal

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestApprovalPaneRoutesInputOnlyToItsMountedForm(t *testing.T) {
	transcript := testTranscriptView(t)
	pane := approvalPane{theme: transcript.theme, glyphs: transcript.glyphs, detail: kit.NewParagraph("", transcript.theme.Text)}
	pane.initialize(input.Wheel{})
	form := func(done func()) *kit.Form {
		field := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Initial: "only", Rows: 1, Options: []base.Option[string]{{Label: "Only", Value: "only"}}})
		return kit.NewForm(kit.FormConfig{Theme: transcript.theme, Glyphs: transcript.glyphs, Fields: []base.Field{field}, Submit: func(base.SubmissionID) (scene.ActionState, error) { done(); return scene.Applied, nil }})
	}
	oldCalls, currentCalls := 0, 0
	pane.setForm(form(func() { oldCalls++ }), func() { t.Fatal("unexpected old form navigation") })
	root := newSceneTestRoot(t, pane.Element())
	if err := pane.form.Element().RequestFocusFirst(); err != nil {
		t.Fatal(err)
	}
	surface := grid.NewSurface(80, 20)
	root.Draw(surface.View())
	retired := pane.form.Element().Target()
	pane.setForm(form(func() { currentCalls++ }), func() { t.Fatal("unexpected current form navigation") })
	if retired.Valid() {
		t.Fatal("replaced approval form kept an input attachment")
	}
	if err := pane.form.Element().RequestFocusFirst(); err != nil {
		t.Fatal(err)
	}
	root.Handle(input.Key{Code: input.Enter})
	if oldCalls != 0 || currentCalls != 1 {
		t.Fatalf("input routed to retired=%d current=%d forms", oldCalls, currentCalls)
	}
	root.Draw(surface.View())
	root.Handle(input.Key{Code: input.Enter})
	if oldCalls != 0 || currentCalls != 2 {
		t.Fatalf("current frame routed to retired=%d current=%d forms", oldCalls, currentCalls)
	}
}

func TestReplacedApprovalFormCannotMutateTheCurrentDraft(t *testing.T) {
	prompts := testInterruptPrompts(t)
	prompts.review = &interruptReview{}
	prompts.approval = &conversation.Approval{}
	prompts.setApprovalForm(approvalAllowOnce)
	retired := prompts.approvalDraft
	prompts.approvalDialog.Show()
	root := newSceneTestRoot(t, prompts.stack.Element())
	if err := prompts.approvalPane.form.Element().RequestFocusFirst(); err != nil {
		t.Fatal(err)
	}
	surface := grid.NewSurface(80, 20)
	root.Draw(surface.View())

	prompts.review = &interruptReview{}
	prompts.approval = &conversation.Approval{}
	prompts.approvalDraft = &approvalDecisionDraft{}
	prompts.setApprovalForm(approvalAllowOnce)
	root.Handle(input.Key{Code: input.Down})
	if retired.choice != approvalAllowOnce {
		t.Fatalf("retired draft choice = %q, want unchanged allow once", retired.choice)
	}
	if current := prompts.approvalDraft.choice; current != approvalAllowOnce {
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
	values := func(options []base.Option[approvalAction]) []approvalAction {
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
	prompts := testInterruptPrompts(t)
	prompts.approval = &conversation.Approval{Rememberable: true}
	prompts.setApprovalForm(approvalDenyGlobal)
	prompts.approval.Rememberable = false
	prompts.setApprovalForm(prompts.approvalDraft.choice)
	if action := prompts.approvalDraft.choice; action != approvalDenyOnce {
		t.Fatalf("rebuilt denial became %q", action)
	}
	previous := prompts.approvalForm
	prompts.setApprovalForm(approvalAction("invalid"))
	if _, err := previous.Submit(); err != nil {
		t.Fatal(err)
	}
	if prompts.approvalForm != nil || prompts.approvalPane.form != nil {
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

// promptRunProbe records what prompts ask of the Run.
type promptRunProbe struct {
	resumed  []*interruptReview
	aborted  []string
	failures []error
	messages []string
}

func (p *promptRunProbe) resumeInterrupts(review *interruptReview) {
	p.resumed = append(p.resumed, review)
}
func (p *promptRunProbe) abortRun(reason string) { p.aborted = append(p.aborted, reason) }
func (p *promptRunProbe) fail(err error)         { p.failures = append(p.failures, err) }
func (p *promptRunProbe) message(label string)   { p.messages = append(p.messages, label) }

func testInterruptPrompts(t *testing.T) *interruptPrompts {
	t.Helper()
	transcript := testTranscriptView(t)
	stack := &workspaceScene{}
	stack.setWorkbench(transcript.Element())
	return newInterruptPrompts(interruptPromptsConfig{
		run: &promptRunProbe{}, remember: func() settings.RememberPreference { return settings.RememberNone },
		stack: stack, theme: transcript.theme, glyphs: transcript.glyphs, syntax: transcript.syntax,
		clipboard: (&program.Runtime{}).Clipboard(),
	})
}

func TestInterruptPromptsHandTheirOutcomeToTheRun(t *testing.T) {
	approval := conversation.Approval{RunID: "run_1", ItemID: "approval", Title: "Write config", Tool: &conversation.ToolCall{
		Kind: builtin.CategoryEdit, Name: "edit", Path: "config.json", Status: conversation.ToolRunning,
	}}
	declined := testInterruptPrompts(t)
	run := declined.run.(*promptRunProbe)
	if !declined.open([]conversation.Interrupt{approval}) || !declined.answering() {
		t.Fatalf("approval did not open: %v", run.failures)
	}
	if !declined.decline() || len(run.resumed) != 1 || run.resumed[0] != declined.review || declined.answering() {
		t.Fatalf("declined approval = resumed %v, answering %t", run.resumed, declined.answering())
	}
	answer, ok := declined.review.Answers()[0].(conversation.ApprovalAnswer)
	if !ok || answer.Decision != protocol.ApprovalDeny {
		t.Fatalf("declined approval answered %+v", declined.review.Answers())
	}

	aborted := testInterruptPrompts(t)
	run = aborted.run.(*promptRunProbe)
	aborted.open([]conversation.Interrupt{approval})
	aborted.abort("canceled in test")
	if aborted.review != nil || aborted.answering() || !slices.Equal(run.aborted, []string{"canceled in test"}) {
		t.Fatalf("abort left review %v, aborted %v", aborted.review, run.aborted)
	}
}
