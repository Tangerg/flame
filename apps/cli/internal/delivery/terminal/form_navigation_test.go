package terminal

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestFormNavigationLeavesAnUnsubmittedDraftWithoutCancellingASubmission(t *testing.T) {
	field := base.NewText(base.TextConfig{Initial: "retained draft"})
	leaves, cancellations := 0, 0
	form := kit.NewForm(kit.FormConfig{
		Fields: []base.Field{field},
		Cancel: func(base.SubmissionID) error { cancellations++; return nil },
	})
	root := newSceneTestRoot(t, formNavigation(form.Element(), func() { leaves++ }))
	root.Draw(grid.NewSurface(40, 5).View())
	if root.Handle(input.Key{Code: input.Esc, Transition: input.Release}) || leaves != 0 || cancellations != 0 {
		t.Fatal("releasing Escape left the draft or cancelled a submission")
	}
	if !root.Handle(input.Key{Code: input.Esc}) || leaves != 1 || cancellations != 0 {
		t.Fatalf("edit navigation = %d leaves, %d submission cancellations", leaves, cancellations)
	}
	if field.Text() != "retained draft" || form.Controller().Submission().State != base.SubmissionIdle {
		t.Fatal("leaving the edit changed the draft or manufactured a submission")
	}
}

func TestFormNavigationPreservesThePendingSubmissionCancellationOwner(t *testing.T) {
	field := base.NewText(base.TextConfig{Initial: "submitted draft"})
	leaves := 0
	var admitted, cancelled base.SubmissionID
	form := kit.NewForm(kit.FormConfig{
		Fields: []base.Field{field},
		Submit: func(id base.SubmissionID) (scene.ActionState, error) {
			admitted = id
			return scene.Started, nil
		},
		Cancel: func(id base.SubmissionID) error { cancelled = id; return nil },
	})
	root := newSceneTestRoot(t, formNavigation(form.Element(), func() { leaves++ }))
	root.Draw(grid.NewSurface(40, 5).View())
	root.Handle(input.Key{Code: input.Enter})
	root.Handle(input.Key{Code: input.Esc})
	if cancelled != admitted || leaves != 0 || form.Controller().Submission().State != base.SubmissionPending {
		t.Fatalf("pending cancellation = %v for %v; leaves = %d, state = %v", cancelled, admitted, leaves, form.Controller().Submission().State)
	}
	if !form.Controller().Settle(admitted, base.SubmissionOutcome{Cancelled: true}) {
		t.Fatal("the actual submission owner could not settle its cancellation")
	}
	root.Handle(input.Key{Code: input.Esc})
	if leaves != 1 {
		t.Fatal("settled form could not leave its editing step")
	}
}
