package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/kit"
)

type interruptRun interface {
	resumeInterrupts(review *interruptReview)
	abortRun(reason string)
	fail(err error)
	message(label string)
}

type interruptPrompts struct {
	run         interruptRun
	editContext func(contextEditorRequest) *contextEditorSession
	remember    func() settings.RememberPreference

	stack     *workspaceScene
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
	approvalForm      *base.Form
	approvalPane      approvalPane
	approvalDialog    *presentationDialog

	questionnaire  *questionnaire
	questionDialog *presentationDialog
	reviewDialog   *presentationDialog
}

type interruptPromptsConfig struct {
	run         interruptRun
	editContext func(contextEditorRequest) *contextEditorSession
	remember    func() settings.RememberPreference
	stack       *workspaceScene
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
	p.approvalPane.initialize(cfg.wheel)
	p.setApprovalForm(approvalAllowOnce)
	p.approvalDialog = newPresentationDialog(cfg.stack, kit.DialogConfig{
		Behavior: base.DialogConfig{
			Title:   "Tool approval",
			Content: p.approvalPane.Element(),
		},
		Theme:  cfg.theme,
		Glyphs: cfg.glyphs,
		Where:  layout.Placement{Width: standardDialogWidth, Height: tallDialogHeight},
	})
	return p
}

func (p *interruptPrompts) answering() bool {
	return p.approval != nil || p.questionnaire != nil
}

func (p *interruptPrompts) decline() bool {
	switch {
	case p.approval != nil:
		p.answerApproval(approvalDenyOnce)
	case p.questionnaire != nil:
		p.cancelQuestionnaire()
	default:
		return false
	}
	return true
}

func (p *interruptPrompts) adopt(review *interruptReview) {
	p.dismiss()
	p.review = review
}

func (p *interruptPrompts) release() {
	p.review = nil
}

func (p *interruptPrompts) abort(reason string) {
	p.clearApprovalProjection()
	p.questionnaire = nil
	p.review = nil
	p.closeReviewDialog()
	p.run.abortRun(reason)
}

func (p *interruptPrompts) dismiss() {
	p.clearApprovalProjection()
	p.questionnaire = nil
	p.review = nil
	p.approvalDialog.Dismiss()
	if p.questionDialog != nil {
		p.questionDialog.Dismiss()
		p.questionDialog = nil
	}
	p.closeReviewDialog()
}

func (p *interruptPrompts) closeReviewDialog() {
	if p.reviewDialog != nil {
		p.reviewDialog.Dismiss()
		p.reviewDialog = nil
	}
}
