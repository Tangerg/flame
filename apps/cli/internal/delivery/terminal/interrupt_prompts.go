package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/highlight"
)

// interruptRun is what answering a waiting Run's interrupts asks of the Run:
// resume it with a completed review, or cancel it.
type interruptRun interface {
	resumeInterrupts(review *interruptReview)
	abortRun(reason string)
	fail(err error)
	message(label string)
}

// interruptPrompts walks the user through a waiting Run's interrupts: one
// approval or question at a time, then a summary. It owns the review and its
// dialogs; the Run owns delivery of the answers.
type interruptPrompts struct {
	run         interruptRun
	editContext func(contextEditorRequest) *contextEditorSession
	remember    func() settings.RememberPreference

	stack     *headless.Stack
	theme     kit.Theme
	glyphs    kit.Glyphs
	syntax    highlight.Renderer
	wheel     input.Wheel
	clipboard program.Clipboard

	review *interruptReview

	approval          *conversation.Approval
	approvalDraft     *approvalDecisionDraft
	approvalArguments string
	approvalOverride  *conversation.ToolArgumentOverride
	approvalSections  []toolSection
	approvalEditor    *contextEditorSession
	approvalForm      *headless.Form
	approvalPane      approvalPane
	approvalDialog    *presentationDialog

	questionnaire  *questionnaire
	questionDialog *kit.Dialog
	reviewDialog   *kit.Dialog
}

type interruptPromptsConfig struct {
	run         interruptRun
	editContext func(contextEditorRequest) *contextEditorSession
	remember    func() settings.RememberPreference
	stack       *headless.Stack
	theme       kit.Theme
	glyphs      kit.Glyphs
	syntax      highlight.Renderer
	wheel       input.Wheel
	clipboard   program.Clipboard
}

func newInterruptPrompts(cfg interruptPromptsConfig) *interruptPrompts {
	p := &interruptPrompts{
		run: cfg.run, editContext: cfg.editContext, remember: cfg.remember,
		stack: cfg.stack, theme: cfg.theme, glyphs: cfg.glyphs, syntax: cfg.syntax, wheel: cfg.wheel, clipboard: cfg.clipboard,
	}
	p.approvalPane = approvalPane{theme: cfg.theme, glyphs: cfg.glyphs, detail: kit.NewParagraph("", cfg.theme.Text)}
	p.approvalPane.scroll.Wheel(cfg.wheel)
	p.approvalPane.view = kit.Transcript{
		Content: &p.approvalPane.preview, Scroll: &p.approvalPane.scroll,
		Theme: cfg.theme, Glyphs: cfg.glyphs,
	}
	p.setApprovalForm(approvalAllowOnce)
	p.approvalDialog = newPresentationDialog(kit.DialogConfig{
		Stack: cfg.stack, Theme: cfg.theme, Glyphs: cfg.glyphs, Title: "Tool approval", Body: &p.approvalPane,
		Where: layout.Placement{Width: 88, Height: 24},
	})
	return p
}

// answering reports whether an approval or question is waiting for the user.
func (p *interruptPrompts) answering() bool {
	return p.approval != nil || p.questionnaire != nil
}

// decline answers the open prompt the way cancel does: deny an approval, or
// cancel a question.
func (p *interruptPrompts) decline() bool {
	switch {
	case p.approval != nil:
		p.answerApproval(approvalDenyOnce)
	case p.questionnaire != nil:
		p.finishQuestionnaire(true)
	default:
		return false
	}
	return true
}

// adopt resumes delivery of a review recovered from the durable outbox.
func (p *interruptPrompts) adopt(review *interruptReview) {
	p.dismiss()
	p.review = review
}

// release ends the review once the Run accepted or retired its answers.
func (p *interruptPrompts) release() {
	p.review = nil
}

// abort drops the review and cancels the Run it belonged to.
func (p *interruptPrompts) abort(reason string) {
	p.clearApprovalProjection()
	p.questionnaire = nil
	p.review = nil
	p.closeReviewDialog()
	p.run.abortRun(reason)
}

// dismiss drops only the obsolete terminal-side answer draft. It never answers
// or cancels the runtime interrupt.
func (p *interruptPrompts) dismiss() {
	p.clearApprovalProjection()
	p.questionnaire = nil
	p.review = nil
	p.approvalDialog.Dismiss()
	if p.questionDialog != nil {
		p.questionDialog.Controller().Dismiss()
		p.questionDialog = nil
	}
	p.closeReviewDialog()
}

func (p *interruptPrompts) closeReviewDialog() {
	if p.reviewDialog != nil {
		p.reviewDialog.Controller().Dismiss()
		p.reviewDialog = nil
	}
}
