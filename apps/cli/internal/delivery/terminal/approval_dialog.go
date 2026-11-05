package terminal

import (
	"errors"
	"fmt"
	"github.com/Tangerg/oolong/ui/core/scene"
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

type approvalPane struct {
	theme       kit.Theme
	glyphs      kit.Glyphs
	title       string
	detail      *kit.Paragraph
	preview     *base.Transcript
	scroll      *base.Scroll
	view        *kit.Transcript
	group       *scene.Group
	heading     scene.Element
	description scene.Element
	form        *kit.Form
	formBody    scene.Element
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

func (a *approvalPane) initialize(wheel input.Wheel) {
	a.heading = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Label{Text: a.title, Style: a.theme.Strong, Ellipsis: a.glyphs.Ellipsis}.Draw(frame.View)
	}})
	a.description = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { a.detail.Draw(frame.View) }})
	a.group = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
		formRows := 0
		if a.form != nil {
			formRows = min(a.form.Element().Measure(layout.Down, space.X), max(space.Y-1, 0))
		}
		detailRows := min(a.detail.HeightForWidth(space.X), min(4, max(space.Y-formRows-1, 0)))
		slots := []layout.Slot{{Size: layout.Fixed(1)}, {Size: layout.Fixed(detailRows)}, {Size: layout.Flex(1)}}
		if a.form != nil {
			slots = append(slots, layout.Slot{Size: layout.Fixed(formRows)})
		}
		return (layout.Flow{Axis: layout.Down}).Rects(space, slots)
	}})
	a.replacePreview(nil, wheel)
}

func (a *approvalPane) Element() scene.Element { return a.group.Element() }

func (a *approvalPane) setForm(form *kit.Form, leave func()) {
	a.form = form
	a.formBody = scene.Element{}
	if form != nil {
		a.formBody = formNavigation(form.Element(), leave)
	}
	a.compose()
}

func (a *approvalPane) compose() {
	items := []scene.Item{{Element: a.heading}, {Element: a.description}, {Element: a.view.Element()}}
	if a.form != nil {
		items = append(items, scene.Item{Element: a.formBody})
	}
	if err := a.group.Set(items...); err != nil {
		panic(err)
	}
}

func (a *approvalPane) replacePreview(blocks []base.Block, wheel input.Wheel) {
	if a.preview != nil {
		a.preview.Close()
	}
	view, err := kit.NewTranscript(kit.TranscriptConfig{Behavior: base.TranscriptConfig{Key: "approval-preview", MaxItems: max(len(blocks), 1)}, Theme: a.theme, Glyphs: a.glyphs})
	if err != nil {
		panic(err)
	}
	a.view, a.preview = view, view.Controller()
	// Approval decisions own keyboard focus. The preview still accepts pointer
	// selection and wheel input without displacing the active form field.
	if err := view.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	a.scroll = a.preview.Scroll()
	a.scroll.Wheel(wheel)
	a.scroll.ToTop()
	for _, block := range blocks {
		if _, err := a.preview.Add(block); err != nil {
			panic(err)
		}
	}
	a.compose()
}

func (p *interruptPrompts) setApprovalForm(initial approvalAction) {
	review, approval := p.review, p.approval
	draft := p.approvalDraft
	if draft == nil {
		draft = &approvalDecisionDraft{}
		p.approvalDraft = draft
	}
	rememberable := approval == nil || approval.Rememberable
	value, ok := initial.Normalize(rememberable)
	if !ok {
		p.approvalDraft = nil
		p.approvalForm = nil
		p.approvalPane.setForm(nil, nil)
		p.run.message("invalid approval action")
		return
	}
	draft.choice = value
	choice := base.NewSelect(base.SelectConfig[approvalAction]{Same: base.Equal[approvalAction], Label: "How should flame proceed?", Initial: value, Options: approvalOptions(rememberable), Rows: 3})
	reason := base.NewText(base.TextConfig{Label: "Denial feedback (optional)", Placeholder: "Explain what should change before retrying", Initial: draft.reason, Clipboard: p.clipboard})
	capture := func() bool {
		if p.review != review || p.approval != approval || p.approvalDraft != draft {
			return false
		}
		selected, ok := choice.Chosen()
		if !ok {
			return false
		}
		draft.choice, draft.reason = selected.Value, reason.Text()
		return true
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: p.approvalPane.theme, Glyphs: p.approvalPane.glyphs, Fields: []base.Field{choice, reason}, Keys: base.DefaultFormKeys(), Hints: []keymap.Action{base.Submit, base.Cancel},
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			if !capture() {
				return scene.Unchanged, nil
			}
			p.answerApproval(draft.choice)
			return scene.Applied, nil
		},
	})
	p.approvalForm = dressed.Controller()
	p.approvalPane.setForm(dressed, func() {
		if capture() {
			p.backOrCancelApproval()
		}
	})
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
	presentation := blockPresentation{Theme: p.approvalPane.theme, Glyphs: p.approvalPane.glyphs, Syntax: p.syntax}
	var blocks []base.Block
	for _, section := range sections {
		for _, block := range renderToolSections(presentation, []toolSection{section}, false) {
			blocks = append(blocks, newReaderSectionBlock(p.approvalPane.theme, section.Title, block))
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, &kit.Entry{Theme: p.approvalPane.theme, Body: "This request has no additional preview."})
	}
	p.approvalPane.replacePreview(blocks, p.wheel)
}

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
		p.questionDialog.Dismiss()
		p.questionDialog = nil
	}
	if p.reviewDialog != nil {
		p.reviewDialog.Dismiss()
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
	p.approvalPane.setForm(nil, nil)
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
