package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type interactionSummaryPane struct {
	viewport *headless.Viewport
	form     *kit.Form
}

type interactionReviewDecision uint8

const (
	interactionReviewSubmit interactionReviewDecision = iota + 1
	interactionReviewBack
	interactionReviewCancel
	interactionReviewCancellationReason = "interactions canceled during terminal review"
)

func (d interactionReviewDecision) Validate() error {
	switch d {
	case interactionReviewSubmit, interactionReviewBack, interactionReviewCancel:
		return nil
	default:
		return fmt.Errorf("interaction review decision %d is invalid", d)
	}
}

func (i *interactionSummaryPane) Draw(frame headless.Frame) {
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(min(i.form.Measure(frame.Bounds().Dx()), 7))},
	}))
	i.viewport.Draw(rows[0])
	i.form.Draw(rows[1])
}

func (i *interactionSummaryPane) Handle(event input.Event) bool {
	if i.form.Handle(event) {
		return true
	}
	return i.viewport.Handle(event)
}

func (i *interactionSummaryPane) Focus(has bool) { i.form.Focus(has) }

func (a *app) openInteractionSummary() {
	review := a.dialogs.interactionReview
	if review == nil || !review.Reviewing() {
		return
	}
	if _, err := review.Responses(); err != nil {
		a.fail(fmt.Errorf("review interactions: %w", err))
		return
	}
	decision := interactionReviewSubmit
	choice := &headless.Select[interactionReviewDecision]{
		Label: "Review complete", Value: headless.Bind(&decision), Rows: 3,
	}
	choice.SetOptions([]headless.Option[interactionReviewDecision]{
		{Label: "Submit all decisions", Value: interactionReviewSubmit},
		{Label: "Go back and edit", Value: interactionReviewBack},
		{Label: "Cancel the run", Value: interactionReviewCancel},
	})
	form := headless.NewForm(choice)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	settled := false
	form.Done = func() {
		if settled || a.dialogs.interactionReview != review || a.dialogs.reviewDialog != dialog {
			return
		}
		if err := decision.Validate(); err != nil {
			a.fail(err)
			return
		}
		settled = true
		dialog.Controller().Dismiss()
		a.dialogs.reviewDialog = nil
		switch decision {
		case interactionReviewSubmit:
			a.resumeInteractions()
		case interactionReviewBack:
			a.backInteraction()
		case interactionReviewCancel:
			a.abortInteractions(interactionReviewCancellationReason)
		}
	}
	form.GaveUp = func() {
		if settled || a.dialogs.interactionReview != review || a.dialogs.reviewDialog != dialog {
			return
		}
		settled = true
		dialog.Controller().Dismiss()
		a.dialogs.reviewDialog = nil
		if !a.backInteraction() {
			a.abortInteractions(interactionReviewCancellationReason)
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	summary := kit.NewParagraph(interactionSummary(review), a.transcript.theme.Text)
	viewport := headless.NewViewport(headless.Static{Of: summary})
	viewport.Scroll().Wheel(a.loop.Environment().Wheel())
	pane := &interactionSummaryPane{viewport: viewport, form: dressed}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Review interactions", Body: pane,
		Where: layout.Placement{Width: 88, Height: 22},
	})
	a.dialogs.reviewDialog = dialog
	dialog.Controller().Show()
}

func interactionSummary(review *interactionReview) string {
	items, answers := review.Items(), review.Answers()
	lines := make([]string, 0, len(items)+2)
	if failure := review.SubmissionFailure(); failure != "" {
		lines = append(lines, failure)
	}
	lines = append(lines, "Nothing is sent to the runtime until you submit this review.")
	for index, item := range items {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, summarizeInteraction(item, answers[index])))
	}
	return strings.Join(lines, "\n\n")
}

func summarizeInteraction(item agent.Interaction, answer agent.Answer) string {
	switch interaction := item.(type) {
	case agent.Approval:
		provided, _ := answer.(agent.ApprovalAnswer)
		decision := "allow once"
		if provided.Decision == agent.ApprovalDeny {
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
		return interaction.Title + " — " + decision
	case agent.Question:
		provided, _ := answer.(agent.QuestionAnswer)
		values := make([]string, 0, len(provided.Values))
		for _, field := range provided.Values {
			values = append(values, strings.Join(field, ", "))
		}
		return interaction.Title + " — " + strings.Join(values, " · ")
	default:
		return "Unknown interaction"
	}
}
