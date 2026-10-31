package terminal

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
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

func (a approvalDecisionDraft) answer(action approvalAction, override *conversation.ToolArgumentOverride) (conversation.ApprovalAnswer, bool) {
	decision, ok := action.Answer()
	if !ok {
		return conversation.ApprovalAnswer{}, false
	}
	if decision.Decision == protocol.ApprovalDeny {
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
	formRows := min(form.HeightForWidth(width), max(height-1, 0))
	detailRows := min(a.detail.HeightForWidth(width), min(4, max(height-formRows-1, 0)))
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

func (p *interruptPrompts) setApprovalForm(initial approvalAction) {
	review := p.review
	approval := p.approval
	draft := p.approvalDraft
	if draft == nil {
		draft = &approvalDecisionDraft{}
		p.approvalDraft = draft
	}
	rememberable := p.approval == nil || p.approval.Rememberable
	choiceValue, ok := initial.Normalize(rememberable)
	if !ok {
		p.approvalDraft = nil
		p.approvalForm = nil
		p.approvalPane.form = nil
		p.run.message("invalid approval action")
		return
	}
	draft.choice = choiceValue
	choice := headless.NewSelect(headless.SelectConfig[approvalAction]{
		Same:  headless.Equal[approvalAction],
		Label: "How should flame proceed?", Value: headless.Bind(&draft.choice), Rows: 3,
	})
	choice.SetOptions(approvalOptions(rememberable))
	reason := headless.NewText(headless.TextConfig{
		Label: "Denial feedback (optional)", Placeholder: "Explain what should change before retrying",
		Value: headless.Bind(&draft.reason),
	})
	reason.Clipboard = p.clipboard
	keys := headless.DefaultFormKeys()
	p.approvalForm = headless.NewForm(choice, reason)
	p.approvalForm.Keys = keys
	p.approvalForm.Done = func() {
		if p.review == review && p.approval == approval && p.approvalDraft == draft {
			p.answerApproval(draft.choice)
		}
	}
	p.approvalForm.GaveUp = func() {
		if p.review == review && p.approval == approval && p.approvalDraft == draft {
			p.backOrCancelApproval()
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: p.approvalPane.theme, Glyphs: p.approvalPane.glyphs, Controller: p.approvalForm,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	p.approvalPane.form = dressed
}

func (p *interruptPrompts) openApproval(approval conversation.Approval) {
	cloned := approval.Clone()
	p.approval = &cloned
	p.approvalDraft = &approvalDecisionDraft{}
	p.approvalArguments = editableApprovalArguments(approval.Tool)
	p.approvalOverride = nil
	initial := defaultApprovalAction(p.remember())
	if answer, ok := p.review.CurrentAnswer().(conversation.ApprovalAnswer); ok {
		initial = approvalActionFromAnswer(answer)
		p.approvalDraft.reason = answer.Reason
		if answer.ArgumentOverride != nil {
			p.approvalOverride = answer.ArgumentOverride.Clone()
			p.approvalArguments = prettyJSON(p.approvalOverride.JSON())
		}
	}
	p.setApprovalForm(initial)
	p.approvalPane.title = approval.Title
	call := approval.Tool.Clone()
	if strings.TrimSpace(call.Diff) == "" {
		call.Diff = approval.Diff
	}
	presentation, err := selectToolPresentation(defaultToolPresenters(), call)
	if err != nil {
		presentation = toolPresentation{
			Label:    toolLabel(call),
			Sections: []toolSection{{Title: "Presentation error", Style: toolSectionParagraph, Text: err.Error()}},
		}
	}
	p.approvalSections = slices.Clone(presentation.Sections)
	details := []string{p.review.SubmissionFailure(), approval.Detail, presentation.Label}
	if approval.Risk != "" {
		details = append(details, "risk: "+string(approval.Risk))
	}
	if approval.RuleHint != "" {
		details = append(details, "rule: "+approval.RuleHint)
	}
	p.approvalPane.detail.SetText([]text.Line{text.Of(strings.Join(nonEmptyStrings(details), "\n"), p.approvalPane.theme.Text)})
	p.setApprovalPreview(p.approvalPreviewSections())
	p.approvalDialog.Controller().SetDescription(approval.Title)
	p.approvalDialog.Show()
}

func (p *interruptPrompts) setApprovalPreview(sections []toolSection) {
	p.approvalPane.preview = headless.Transcript{}
	p.approvalPane.view.Content = &p.approvalPane.preview
	presentation := blockPresentation{
		Theme: p.approvalPane.theme, Glyphs: p.approvalPane.glyphs, Syntax: p.syntax,
	}
	blockCount := 0
	for _, section := range sections {
		for _, block := range renderToolSections(presentation, []toolSection{section}, false) {
			id := p.approvalPane.preview.Append(newReaderSectionBlock(p.approvalPane.theme, section.Title, block))
			p.approvalPane.preview.Finish(id)
			blockCount++
		}
	}
	if blockCount == 0 {
		id := p.approvalPane.preview.Append(&kit.Entry{Theme: p.approvalPane.theme, Body: "This request has no additional preview."})
		p.approvalPane.preview.Finish(id)
	}
	p.approvalPane.scroll = headless.Scroll{}
	p.approvalPane.scroll.Wheel(p.wheel)
	p.approvalPane.scroll.ToTop()
	p.approvalPane.view.Scroll = &p.approvalPane.scroll
}

// open starts reviewing the interrupts a waiting Run raised; it reports
// whether a review opened.
func (p *interruptPrompts) open(interrupts []conversation.Interrupt) bool {
	if p.review != nil {
		p.run.fail(errors.New("runtime opened interrupts while another set is active"))
		return false
	}
	review, err := newInterruptReview(interrupts)
	if err != nil {
		p.run.fail(fmt.Errorf("runtime interrupts: %w", err))
		return false
	}
	p.review = review
	p.openCurrentInterrupt()
	return true
}

func (p *interruptPrompts) openCurrentInterrupt() {
	if p.review == nil {
		return
	}
	if p.review.Reviewing() {
		p.openInterruptSummary()
		return
	}
	interrupt, ok := p.review.Current()
	if !ok {
		p.run.fail(errors.New("interrupt review has no current item"))
		return
	}
	switch item := interrupt.(type) {
	case conversation.Approval:
		p.openApproval(item)
	case conversation.Question:
		p.openQuestion(item)
	default:
		p.run.fail(errors.New("runtime returned an unknown interrupt"))
	}
}

func (p *interruptPrompts) answerApproval(action approvalAction) {
	approval := p.approval
	draft := p.approvalDraft
	if approval == nil || draft == nil {
		return
	}
	if action == approvalEditArgs {
		p.openApprovalArgumentEditor()
		return
	}
	decision, ok := draft.answer(action, p.approvalOverride)
	if !ok {
		p.run.fail(fmt.Errorf("approval action %q is unsupported", action))
		return
	}
	p.submitApproval(decision)
}

func (p *interruptPrompts) submitApproval(decision conversation.ApprovalAnswer) {
	if p.review == nil {
		return
	}
	if err := p.review.Record(decision); err != nil {
		p.run.fail(fmt.Errorf("record approval: %w", err))
		return
	}
	p.clearApprovalProjection()
	p.approvalDialog.Dismiss()
	p.advanceInterruptReview()
}

func (p *interruptPrompts) backOrCancelApproval() {
	if p.backInterrupt() {
		return
	}
	p.answerApproval(approvalDenyOnce)
}

func (p *interruptPrompts) advanceInterruptReview() {
	if p.review == nil {
		return
	}
	if p.review.Advance() || p.review.Reviewing() {
		p.openCurrentInterrupt()
		return
	}
	p.run.resumeInterrupts(p.review)
}

func (p *interruptPrompts) backInterrupt() bool {
	if p.review == nil || !p.review.Back() {
		return false
	}
	if p.approval != nil {
		p.clearApprovalProjection()
		p.approvalDialog.Dismiss()
	}
	if p.questionnaire != nil {
		p.questionnaire = nil
		p.questionDialog.Controller().Dismiss()
		p.questionDialog = nil
	}
	if p.reviewDialog != nil {
		p.reviewDialog.Controller().Dismiss()
		p.reviewDialog = nil
	}
	p.openCurrentInterrupt()
	return true
}

// reopenCompletedInterruptReview restores the UI owner of a completed HITL
// draft when the draft could not be durably staged or its delivery was
// definitively refused. A multi-item draft returns to its review summary; a
// single-item draft returns to the answered interrupt so the user can retry
// or revise it.
func (p *interruptPrompts) reopenCompletedInterruptReview(review *interruptReview) error {
	if review == nil || p.review != review {
		return errors.New("completed interrupt review is no longer active")
	}
	if !review.completed() {
		return errors.New("interrupt review is not complete")
	}
	if review.Reviewing() {
		p.openInterruptSummary()
		return nil
	}
	if !review.Back() {
		return errors.New("interrupt review cannot return to its submitted answer")
	}
	p.openCurrentInterrupt()
	return nil
}

func (p *interruptPrompts) clearApprovalProjection() {
	p.approval = nil
	p.approvalDraft = nil
	p.approvalArguments = ""
	p.approvalOverride = nil
	p.approvalSections = nil
	p.dismissApprovalEditor()
	p.approvalForm = nil
	p.approvalPane.form = nil
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
