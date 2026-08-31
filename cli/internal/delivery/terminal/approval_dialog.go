package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

type approvalPane struct {
	theme         kit.Theme
	glyphs        kit.Glyphs
	title         string
	detail        *kit.Paragraph
	preview       headless.Transcript
	scroll        headless.Scroll
	view          kit.Transcript
	form          *kit.Form
	presentedForm headless.Snapshot[*kit.Form]
}

type approvalDecisionDraft struct {
	choice approvalAction
	reason string
}

func (a approvalDecisionDraft) answer(action approvalAction, override *agent.ToolArgumentOverride) (agent.ApprovalAnswer, bool) {
	decision, ok := action.Answer()
	if !ok {
		return agent.ApprovalAnswer{}, false
	}
	if decision.Decision == agent.ApprovalDeny {
		decision.Reason = strings.TrimSpace(a.reason)
		if decision.Reason == "" {
			decision.Reason = "denied by the user in the terminal"
		}
	} else {
		decision.ArgumentOverride = override.Clone()
	}
	return decision, true
}

func (a *approvalPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	form := a.form
	a.presentedForm.Stage(frame, form)
	if width <= 0 || height <= 0 || form == nil {
		return
	}
	formRows := min(form.Measure(width), max(height-1, 0))
	detailRows := min(a.detail.Measure(width), min(4, max(height-formRows-1, 0)))
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(detailRows)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(formRows)},
	}))
	kit.Label{Text: a.title, Style: a.theme.Strong, Ellipsis: "…"}.Draw(rows[0].View)
	a.detail.Draw(rows[1].View)
	a.view.Draw(rows[2])
	form.Draw(rows[3])
}

func (a *approvalPane) Handle(event input.Event) bool {
	if form := a.presentedForm.Value(); form != nil && form.Handle(event) {
		return true
	}
	return a.view.Handle(event)
}

func (a *approvalPane) Focus(has bool) {
	if a.form != nil {
		a.form.Focus(has)
	}
}

func (a *app) buildApprovalDialog(theme kit.Theme, glyphs kit.Glyphs) {
	a.dialogs.approvalPane = approvalPane{
		theme: theme, glyphs: glyphs, detail: kit.NewParagraph("", theme.Text),
	}
	a.dialogs.approvalPane.scroll.Wheel(a.loop.Environment().Wheel())
	a.dialogs.approvalPane.view = kit.Transcript{
		Content: &a.dialogs.approvalPane.preview, Scroll: &a.dialogs.approvalPane.scroll,
		Theme: theme, Glyphs: glyphs,
	}
	a.setApprovalForm(approvalAllowOnce)
	a.dialogs.approvalDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Tool approval", Body: &a.dialogs.approvalPane,
		Where: layout.Placement{Width: 88, Height: 24},
	})
}

func (a *app) setApprovalForm(initial approvalAction) {
	review := a.dialogs.interactionReview
	approval := a.dialogs.approval
	draft := a.dialogs.approvalDraft
	if draft == nil {
		draft = &approvalDecisionDraft{}
		a.dialogs.approvalDraft = draft
	}
	rememberable := a.dialogs.approval == nil || a.dialogs.approval.Rememberable
	draft.choice = initial.Normalize(rememberable)
	choice := &headless.Select[approvalAction]{
		Label: "How should flame proceed?", Value: headless.Bind(&draft.choice), Rows: 3,
	}
	choice.SetOptions(approvalOptions(rememberable))
	reason := &headless.Text{
		Label: "Denial feedback (optional)", Placeholder: "Explain what should change before retrying",
		Value: headless.Bind(&draft.reason),
	}
	reason.Editor().Clipboard = a.loop.Clipboard()
	keys := headless.DefaultFormKeys()
	a.dialogs.approvalForm = headless.NewForm(choice, reason)
	a.dialogs.approvalForm.Keys = keys
	a.dialogs.approvalForm.Done = func() {
		if a.dialogs.interactionReview == review && a.dialogs.approval == approval && a.dialogs.approvalDraft == draft {
			a.answerApproval(draft.choice)
		}
	}
	a.dialogs.approvalForm.GaveUp = func() {
		if a.dialogs.interactionReview == review && a.dialogs.approval == approval && a.dialogs.approvalDraft == draft {
			a.backOrCancelApproval()
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.dialogs.approvalPane.theme, Glyphs: a.dialogs.approvalPane.glyphs, Controller: a.dialogs.approvalForm,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.dialogs.approvalPane.form = dressed
}

func (a *app) openApproval(approval agent.Approval) {
	cloned := approval.Clone()
	a.dialogs.approval = &cloned
	a.dialogs.approvalDraft = &approvalDecisionDraft{}
	a.dialogs.approvalArguments = editableApprovalArguments(approval.Tool)
	a.dialogs.approvalOverride = nil
	initial := defaultApprovalAction(a.settings.Approval.Remember.Scope())
	if answer, ok := a.dialogs.interactionReview.CurrentAnswer().(agent.ApprovalAnswer); ok {
		initial = approvalActionFromAnswer(answer)
		a.dialogs.approvalDraft.reason = answer.Reason
		if answer.ArgumentOverride != nil {
			a.dialogs.approvalOverride = answer.ArgumentOverride.Clone()
			a.dialogs.approvalArguments = formatToolArguments(a.dialogs.approvalOverride.JSON())
		}
	}
	a.setApprovalForm(initial)
	a.dialogs.approvalPane.title = approval.Title
	call := approval.Tool.Clone()
	if strings.TrimSpace(call.Diff) == "" {
		call.Diff = approval.Diff
	}
	presentation, err := selectToolPresentation(a.registry.Values(ToolPresenters), call)
	if err != nil {
		presentation = ToolPresentation{
			Label:    toolLabel(call),
			Sections: []ToolSection{{Title: "Presentation error", Style: toolSectionParagraph, Text: err.Error()}},
		}
	}
	a.dialogs.approvalSections = slices.Clone(presentation.Sections)
	details := []string{a.dialogs.interactionReview.SubmissionFailure(), approval.Detail, presentation.Label}
	if approval.Risk != "" {
		details = append(details, "risk: "+string(approval.Risk))
	}
	if approval.RuleHint != "" {
		details = append(details, "rule: "+approval.RuleHint)
	}
	a.dialogs.approvalPane.detail.SetText([]text.Line{text.Of(strings.Join(nonEmptyStrings(details), "\n"), a.dialogs.approvalPane.theme.Text)})
	a.setApprovalPreview(a.approvalPreviewSections())
	a.dialogs.approvalDialog.Controller().SetDescription(approval.Title)
	a.dialogs.approvalDialog.Show()
}

func (a *app) setApprovalPreview(sections []ToolSection) {
	a.dialogs.approvalPane.preview = headless.Transcript{}
	a.dialogs.approvalPane.view.Content = &a.dialogs.approvalPane.preview
	presentation := BlockPresentation{
		Theme: a.dialogs.approvalPane.theme, Glyphs: a.dialogs.approvalPane.glyphs, Syntax: a.syntax,
	}
	blockCount := 0
	for _, section := range sections {
		for _, block := range renderToolSections(presentation, []ToolSection{section}, false) {
			id := a.dialogs.approvalPane.preview.Append(newReaderSectionBlock(a.dialogs.approvalPane.theme, section.Title, block))
			a.dialogs.approvalPane.preview.Finish(id)
			blockCount++
		}
	}
	if blockCount == 0 {
		id := a.dialogs.approvalPane.preview.Append(&kit.Entry{Theme: a.dialogs.approvalPane.theme, Body: "This request has no additional preview."})
		a.dialogs.approvalPane.preview.Finish(id)
	}
	a.dialogs.approvalPane.scroll = headless.Scroll{}
	a.dialogs.approvalPane.scroll.Wheel(a.loop.Environment().Wheel())
	a.dialogs.approvalPane.scroll.ToTop()
	a.dialogs.approvalPane.view.Scroll = &a.dialogs.approvalPane.scroll
}

func (a *app) openInteractions(interactions []agent.Interaction) {
	if a.dialogs.interactionReview != nil {
		a.fail(errors.New("runtime opened interactions while another set is active"))
		return
	}
	review, err := newInteractionReview(interactions)
	if err != nil {
		a.fail(fmt.Errorf("runtime interactions: %w", err))
		return
	}
	a.dialogs.interactionReview = review
	a.openCurrentInteraction()
	a.raiseAttention(interactionAttention(interactions))
}

func (a *app) openCurrentInteraction() {
	if a.dialogs.interactionReview == nil {
		return
	}
	if a.dialogs.interactionReview.Reviewing() {
		a.openInteractionSummary()
		return
	}
	interaction, ok := a.dialogs.interactionReview.Current()
	if !ok {
		a.fail(errors.New("interaction review has no current item"))
		return
	}
	switch item := interaction.(type) {
	case agent.Approval:
		a.openApproval(item)
	case agent.Question:
		a.openQuestion(item)
	default:
		a.fail(errors.New("runtime returned an unknown interaction"))
	}
}

func (a *app) answerApproval(action approvalAction) {
	approval := a.dialogs.approval
	draft := a.dialogs.approvalDraft
	if approval == nil || draft == nil {
		return
	}
	if action == approvalEditArgs {
		a.openApprovalArgumentEditor()
		return
	}
	decision, ok := draft.answer(action, a.dialogs.approvalOverride)
	if !ok {
		a.fail(fmt.Errorf("approval action %q is unsupported", action))
		return
	}
	a.submitApproval(decision)
}

func (a *app) submitApproval(decision agent.ApprovalAnswer) {
	if a.dialogs.interactionReview == nil {
		return
	}
	if err := a.dialogs.interactionReview.Record(decision); err != nil {
		a.fail(fmt.Errorf("record approval: %w", err))
		return
	}
	a.clearApprovalProjection()
	a.dialogs.approvalDialog.Dismiss()
	a.advanceInteractionReview()
}

func (a *app) backOrCancelApproval() {
	if a.backInteraction() {
		return
	}
	a.answerApproval(approvalDenyOnce)
}

func (a *app) advanceInteractionReview() {
	if a.dialogs.interactionReview == nil {
		return
	}
	if a.dialogs.interactionReview.Advance() || a.dialogs.interactionReview.Reviewing() {
		a.openCurrentInteraction()
		return
	}
	a.resumeInteractions()
}

func (a *app) backInteraction() bool {
	if a.dialogs.interactionReview == nil || !a.dialogs.interactionReview.Back() {
		return false
	}
	if a.dialogs.approval != nil {
		a.clearApprovalProjection()
		a.dialogs.approvalDialog.Dismiss()
	}
	if a.dialogs.questionnaire != nil {
		a.dialogs.questionnaire = nil
		a.dialogs.questionDialog.Controller().Dismiss()
		a.dialogs.questionDialog = nil
	}
	if a.dialogs.reviewDialog != nil {
		a.dialogs.reviewDialog.Controller().Dismiss()
		a.dialogs.reviewDialog = nil
	}
	a.openCurrentInteraction()
	return true
}

func (a *app) resumeInteractions() {
	if a.dialogs.interactionReview == nil {
		return
	}
	answers, err := a.dialogs.interactionReview.Responses()
	if err != nil {
		a.fail(fmt.Errorf("commit interaction review: %w", err))
		return
	}
	runID := a.execution.conversation.RunID()
	review := a.dialogs.interactionReview
	commandID, err := agent.NewCommandID()
	if err != nil {
		a.fail(err)
		return
	}
	command := agent.ResumeRun{CommandID: commandID, RunID: runID, Answers: answers}
	replay := commandReplayGuard(a.runtimeProfile)
	if a.workbench != nil {
		pending := workbench.PendingResume{
			Command: command.Clone(), Interactions: review.Items(), Replay: replay,
		}
		if err := a.workbench.StagePendingResume(a.session.current.ID, pending); err != nil {
			failure := fmt.Errorf("resume blocked: save interaction decisions: %w", err)
			review.ReportSubmissionFailure(failure)
			a.message(failure.Error())
			a.status.note("resume blocked · review preserved")
			if reopenErr := a.reopenCompletedInteractionReview(review); reopenErr != nil {
				a.fail(errors.Join(failure, reopenErr))
			}
			return
		}
	}
	a.deliverInteractionResume(review, command, replay)
}

// reopenCompletedInteractionReview restores the UI owner of a completed HITL
// draft when the draft could not be durably staged or its delivery was
// definitively refused. A multi-item draft returns to its review summary; a
// single-item draft returns to the answered interaction so the user can retry
// or revise it.
func (a *app) reopenCompletedInteractionReview(review *interactionReview) error {
	if review == nil || a.dialogs.interactionReview != review {
		return errors.New("completed interaction review is no longer active")
	}
	if !review.completed() {
		return errors.New("interaction review is not complete")
	}
	if review.Reviewing() {
		a.openInteractionSummary()
		return nil
	}
	if !review.Back() {
		return errors.New("interaction review cannot return to its submitted answer")
	}
	a.openCurrentInteraction()
	return nil
}

func (a *app) deliverInteractionResume(
	review *interactionReview,
	command agent.ResumeRun,
	replay commandreplay.Guard,
) {
	a.status.active("resuming")
	a.syncAnimation()
	a.followOpening(func(ctx context.Context) (agent.SegmentStream, error) {
		if err := commandReplayAdmission(replay, a.runtimeProfile)(); err != nil {
			return agent.SegmentStream{}, &resumeRunCallError{err: err}
		}
		stream, err := a.runtime.ResumeRun(ctx, command)
		if err != nil {
			if _, accepted := agent.AcceptedMutationReceipt(err); accepted {
				return agent.SegmentStream{}, err
			}
			return agent.SegmentStream{}, &resumeRunCallError{err: err}
		}
		if err := stream.ValidateResume(command.RunID, nil); err != nil {
			return agent.SegmentStream{}, agent.NewAcceptedMutationError(stream, fmt.Errorf("resume run: %w", err))
		}
		return stream, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(agent.SegmentStream) streamOpeningDisposition {
			a.dialogs.interactionReview = nil
			a.settleAcknowledgedResume(command.CommandID)
			acceptedQuestions, err := a.execution.conversation.RecordAcceptedInteractionAnswers(command.Answers)
			if err == nil {
				err = a.transcript.acceptQuestions(acceptedQuestions)
			}
			if err != nil {
				failure := fmt.Errorf("project accepted interaction answers: %w", err)
				a.cancelRuntimePreservingFailure(agent.CancelRun{
					RunID: command.RunID, Reason: "terminal could not project accepted interaction answers",
				})
				a.fail(failure)
				return rejectOpenedStream
			}
			return followOpenedStream
		},
		rejected: func(failure error) error {
			if _, accepted := agent.AcceptedMutationReceipt(failure); accepted {
				a.dialogs.interactionReview = nil
				a.cancelRuntimePreservingFailure(agent.CancelRun{
					RunID: command.RunID, Reason: "runtime returned an invalid resume receipt",
				})
				return failure
			}
			return a.restoreRejectedInteractionReview(review, command, failure)
		},
	})
}

func (a *app) settleAcknowledgedResume(commandID agent.CommandID) {
	if err := a.retireAcknowledgedResume(commandID); err != nil {
		a.reportWorkbenchIssue(workbenchResumeOutbox, err)
		a.message("could not settle acknowledged interaction decisions: " + err.Error())
		a.retryAuthoringSettlement(
			resumeSettlementOperation,
			func() error { return a.retireAcknowledgedResume(commandID) },
			func() { a.reportWorkbenchIssue(workbenchResumeOutbox, nil) },
		)
		return
	}
	a.reportWorkbenchIssue(workbenchResumeOutbox, nil)
}

func (a *app) retireAcknowledgedResume(commandID agent.CommandID) error {
	if a.workbench == nil {
		return nil
	}
	pending, ok := a.workbench.PendingResume(a.session.current.ID)
	if !ok {
		return nil
	}
	if pending.Command.CommandID != commandID {
		return errors.New("pending resume command identity changed")
	}
	return a.workbench.AcknowledgePendingResume(a.session.current.ID, commandID)
}

func (a *app) restoreRejectedInteractionReview(review *interactionReview, command agent.ResumeRun, failure error) error {
	callFailure, refused := errors.AsType[*resumeRunCallError](failure)
	if refused && a.workbench != nil && errors.Is(callFailure.err, mutation.ErrReplayGuaranteeUnavailable) {
		a.reconcileExpiredResume(command)
		return nil
	}
	if !refused || mutation.OutcomeUnknown(callFailure.err) || a.dialogs.interactionReview != review ||
		a.execution.conversation.Phase() != agent.ConversationWaiting || a.execution.conversation.RunID() != command.RunID {
		return failure
	}
	if a.workbench != nil {
		if err := a.workbench.RejectPendingResume(a.session.current.ID, command.CommandID); err != nil {
			return errors.Join(failure, fmt.Errorf("release refused interaction decisions: %w", err))
		}
	}
	a.execution.following = false
	if a.session.invalidated {
		a.status.note("resume refused · refreshing session")
		a.refreshInvalidatedSession(false)
		return nil
	}
	a.status.note("resume refused · review preserved")
	review.ReportSubmissionFailure(fmt.Errorf("resume refused: %w", callFailure.err))
	if err := a.reopenCompletedInteractionReview(review); err != nil {
		return errors.Join(failure, err)
	}
	return nil
}

func (a *app) reconcileExpiredResume(command agent.ResumeRun) {
	a.execution.following = false
	a.status.note("resume replay expired · checking runtime state")
	a.syncAnimation()
	sessionID := a.session.current.ID
	started := a.runSessionSettlement(resumeRecoveryOperation, false,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			return a.readInvalidatedSession(ctx, sessionID)
		},
		func(snapshot agent.SessionSnapshot, err error) {
			if err != nil {
				a.message("could not reconcile expired interaction delivery: " + err.Error())
				a.status.note("resume outcome unknown · decisions preserved")
				return
			}
			pending, ok := a.workbench.PendingResume(sessionID)
			if !ok || pending.Command.CommandID != command.CommandID {
				return
			}
			if err := a.installSnapshot(snapshot); err != nil {
				a.fail(fmt.Errorf("reconcile expired interaction delivery: %w", err))
				return
			}
			a.restorePendingResume()
		},
	)
	if !started {
		a.status.note("resume outcome unknown · decisions preserved")
	}
}

func (a *app) abortInteractions(reason string) {
	a.clearApprovalProjection()
	a.dialogs.questionnaire = nil
	a.dialogs.interactionReview = nil
	if a.dialogs.reviewDialog != nil {
		a.dialogs.reviewDialog.Controller().Dismiss()
		a.dialogs.reviewDialog = nil
	}
	if runID := a.execution.conversation.RunID(); runID != "" {
		a.cancelRuntime(agent.CancelRun{RunID: runID, Reason: reason})
	}
}

func (a *app) clearApprovalProjection() {
	a.dialogs.approval = nil
	a.dialogs.approvalDraft = nil
	a.dialogs.approvalArguments = ""
	a.dialogs.approvalOverride = nil
	a.dialogs.approvalSections = nil
	a.dismissApprovalEditor()
	a.dialogs.approvalForm = nil
	a.dialogs.approvalPane.form = nil
}

func nonEmptyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
