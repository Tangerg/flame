package main

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
)

func TestAuthorOwnsFocusAndSealsReviewedContent(t *testing.T) {
	current := planworkflow.Step{Description: "build", Status: planworkflow.PlanStatusInProgress}
	decision, err := author(planworkflow.Proposal{Steps: []planworkflow.Step{current, current}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decision.Open(); !errors.Is(err, planworkflow.ErrRefused) {
		t.Fatalf("two active steps: %v", err)
	}
	decision, err = author(planworkflow.Proposal{Steps: []planworkflow.Step{current}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := decision.Open()
	if err != nil {
		t.Fatal(err)
	}
	text, err := review(document)
	if err != nil || text != "[~] build\n" {
		t.Fatalf("review = %q, %v", text, err)
	}
	document.Steps[0].Description = "changed after capture"
	if _, err := review(document); err == nil {
		t.Fatal("reviewed different content under the same version")
	}
}
