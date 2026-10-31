package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

type interruptSummaryPane struct {
	viewport *headless.Viewport
	form     *kit.Form
}

// interruptReviewCancellationReason is the reason a cancel decision carries,
// not a fourth decision. It stays out of the block below: a spelled-out value
// there reads as a member, and dropping the value would silently make it one.
const interruptReviewCancellationReason = "interrupts canceled during terminal review"

type interruptReviewDecision uint8

const (
	interruptReviewSubmit interruptReviewDecision = iota + 1
	interruptReviewBack
	interruptReviewCancel
)

func (d interruptReviewDecision) Validate() error {
	switch d {
	case interruptReviewSubmit, interruptReviewBack, interruptReviewCancel:
		return nil
	default:
		return fmt.Errorf("interrupt review decision %d is invalid", d)
	}
}

func (i *interruptSummaryPane) Draw(frame headless.Frame) {
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(min(i.form.HeightForWidth(frame.Bounds().Dx()), 7))},
	}))
	i.viewport.Draw(rows[0])
	i.form.Draw(rows[1])
}

func (i *interruptSummaryPane) Handle(event input.Event) bool {
	if i.form.Handle(event) {
		return true
	}
	return i.viewport.Handle(event)
}

func (i *interruptSummaryPane) Focus(has bool) { i.form.Focus(has) }

func (p *interruptPrompts) openInterruptSummary() {
	review := p.review
	if review == nil || !review.Reviewing() {
		return
	}
	if _, err := review.Responses(); err != nil {
		p.run.fail(fmt.Errorf("review interrupts: %w", err))
		return
	}
	decision := interruptReviewSubmit
	choice := headless.NewSelect(headless.SelectConfig[interruptReviewDecision]{
		Same:  headless.Equal[interruptReviewDecision],
		Label: "Review complete", Value: headless.Bind(&decision), Rows: 3,
	})
	choice.SetOptions([]headless.Option[interruptReviewDecision]{
		{Label: "Submit all decisions", Value: interruptReviewSubmit},
		{Label: "Go back and edit", Value: interruptReviewBack},
		{Label: "Cancel the run", Value: interruptReviewCancel},
	})
	form := headless.NewForm(choice)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	settled := false
	form.Done = func() {
		if settled || p.review != review || p.reviewDialog != dialog {
			return
		}
		if err := decision.Validate(); err != nil {
			p.run.fail(err)
			return
		}
		settled = true
		dialog.Controller().Dismiss()
		p.reviewDialog = nil
		switch decision {
		case interruptReviewSubmit:
			p.run.resumeInterrupts(p.review)
		case interruptReviewBack:
			p.backInterrupt()
		case interruptReviewCancel:
			p.abort(interruptReviewCancellationReason)
		}
	}
	form.GaveUp = func() {
		if settled || p.review != review || p.reviewDialog != dialog {
			return
		}
		settled = true
		dialog.Controller().Dismiss()
		p.reviewDialog = nil
		if !p.backInterrupt() {
			p.abort(interruptReviewCancellationReason)
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: p.theme, Glyphs: p.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	summary := kit.NewParagraph(interruptSummary(review), p.theme.Text)
	viewport := headless.NewViewport(headless.Static{Of: summary})
	viewport.Scroll().Wheel(p.wheel)
	pane := &interruptSummaryPane{viewport: viewport, form: dressed}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: p.stack, Theme: p.theme, Glyphs: p.glyphs,
		Title: "Review interrupts", Body: pane,
		Where: layout.Placement{Width: 88, Height: 22},
	})
	p.reviewDialog = dialog
	dialog.Controller().Show()
}

func interruptSummary(review *interruptReview) string {
	items, answers := review.Items(), review.Answers()
	lines := make([]string, 0, len(items)+2)
	if failure := review.SubmissionFailure(); failure != "" {
		lines = append(lines, failure)
	}
	lines = append(lines, "Nothing is sent to the runtime until you submit this review.")
	for index, item := range items {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, summarizeInterrupt(item, answers[index])))
	}
	return strings.Join(lines, "\n\n")
}

func summarizeInterrupt(item conversation.Interrupt, answer conversation.Answer) string {
	switch interrupt := item.(type) {
	case conversation.Approval:
		provided, _ := answer.(conversation.ApprovalAnswer)
		decision := "allow once"
		if provided.Decision == protocol.ApprovalDeny {
			decision = "deny once"
			if provided.Remember != "" {
				decision = "deny for " + string(provided.Remember)
			}
			if strings.TrimSpace(provided.Reason) != "" {
				decision += " — " + strings.TrimSpace(provided.Reason)
			}
		} else if provided.Remember != "" {
			decision = "allow for " + string(provided.Remember)
		}
		if provided.ArgumentOverride != nil {
			decision += " with edited arguments: " + string(provided.ArgumentOverride.JSON())
		}
		return interrupt.Title + " — " + decision
	case conversation.Question:
		provided, _ := answer.(conversation.QuestionAnswer)
		values := make([]string, 0, len(provided.Values))
		for _, field := range provided.Values {
			values = append(values, strings.Join(field, ", "))
		}
		return interrupt.Title + " — " + strings.Join(values, " · ")
	default:
		return "Unknown interrupt"
	}
}
