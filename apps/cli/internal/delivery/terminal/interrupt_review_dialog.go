package terminal

import (
	"fmt"
	"github.com/Tangerg/oolong/ui/core/scene"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"
)

type interruptSummaryPane = confirmationContentPane

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
	choice := base.NewSelect(base.SelectConfig[interruptReviewDecision]{
		Same:  base.Equal[interruptReviewDecision],
		Label: "Review complete", Initial: decision, Rows: 3,
	})
	choice.SetOptions([]base.Option[interruptReviewDecision]{
		{Label: "Submit all decisions", Value: interruptReviewSubmit},
		{Label: "Go back and edit", Value: interruptReviewBack},
		{Label: "Cancel the run", Value: interruptReviewCancel},
	})
	form := kit.FormConfig{Fields: []base.Field{choice}, Theme: p.theme, Glyphs: p.glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	settled := false
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if settled || p.review != review || p.reviewDialog != dialog {
			return scene.Unchanged, nil
		}
		selected, selectedOK := choice.Chosen()
		if !selectedOK {
			return scene.Unchanged, fmt.Errorf("review decision is required")
		}
		decision := selected.Value
		if err := decision.Validate(); err != nil {
			p.run.fail(err)
			return scene.Unchanged, nil
		}
		settled = true
		dialog.Dismiss()
		p.reviewDialog = nil
		switch decision {
		case interruptReviewSubmit:
			p.run.resumeInterrupts(p.review)
		case interruptReviewBack:
			p.backInterrupt()
		case interruptReviewCancel:
			p.abort(interruptReviewCancellationReason)
		}
		return scene.Applied, nil
	}
	leave := func() {
		if settled || p.review != review || p.reviewDialog != dialog {
			return
		}
		settled = true
		dialog.Dismiss()
		p.reviewDialog = nil
		if !p.backInterrupt() {
			p.abort(interruptReviewCancellationReason)
		}
	}
	dressed := kit.NewForm(form)
	summary := kit.NewParagraph(interruptSummary(review), p.theme.Text)
	viewport := base.NewViewport(base.ViewportConfig{Content: passiveElement(summary)})
	viewport.Scroll().Wheel(p.wheel)
	pane := &interruptSummaryPane{viewport: viewport, form: dressed}
	dialog = newPresentationDialog(p.stack, kit.DialogConfig{Theme: p.theme, Glyphs: p.glyphs,
		Title: "Review interrupts", Body: formNavigation(pane.Element(), leave),
		Where: layout.Placement{Width: 88, Height: 22},
	})
	p.reviewDialog = dialog
	dialog.Show()
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
