package main

import (
	"testing"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend"
)

func TestAnalyzerFindingsReferToTheInspectedSnapshot(t *testing.T) {
	snapshot := reviews.Snapshot{ID: reviews.ExampleReviewID, Title: "Evidence", Status: reviews.Open, Revision: 4}
	result, err := analyze(snapshot)
	if err != nil || len(result.Findings) != 1 || result.Findings[0].Validate(snapshot) != nil {
		t.Fatalf("open review analysis: %+v, %v", result, err)
	}
	snapshot.Status = reviews.Resolved
	if result, err := analyze(snapshot); err != nil || len(result.Findings) != 0 {
		t.Fatalf("resolved review analysis: %+v, %v", result, err)
	}
	snapshot.Revision = 0
	if _, err := analyze(snapshot); err == nil {
		t.Fatal("analyzed an invalid snapshot")
	}
}
