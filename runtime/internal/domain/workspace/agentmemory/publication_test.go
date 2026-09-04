package agentmemory

import (
	"slices"
	"testing"
	"time"
)

func TestPublicationOwnsOneExactCurationAdvance(t *testing.T) {
	now := time.Date(2026, time.September, 5, 9, 0, 0, 0, time.FixedZone("offset", 8*60*60))
	expected := State{Watermark: 2, UpdatedAt: now.Add(-time.Hour).UTC()}
	contents := []string{" first ", "second", "first"}
	publication, err := NewPublication("/repo", expected, 4, contents, now)
	if err != nil {
		t.Fatalf("NewPublication: %v", err)
	}
	contents[0] = "changed"
	read := publication.Contents()
	read[0] = "changed again"
	if publication.Project() != "/repo" || publication.ExpectedState() != expected ||
		publication.State().Watermark != 4 || publication.State().UpdatedAt.Location() != time.UTC ||
		!slices.Equal(publication.Contents(), []string{"first", "second"}) {
		t.Fatalf("Publication lost its exact owned decision: %+v", publication)
	}
}

func TestPublicationRejectsInvalidStateTransitions(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	tests := []struct {
		name     string
		project  string
		expected State
		through  int64
		at       time.Time
	}{
		{name: "project", through: 1, at: now},
		{name: "expected", project: "/repo", expected: State{Watermark: -1}, through: 1, at: now},
		{name: "watermark", project: "/repo", expected: State{Watermark: 1, UpdatedAt: now}, through: 1, at: now},
		{name: "time", project: "/repo", expected: State{Watermark: 1, UpdatedAt: now}, through: 2, at: now.Add(-time.Second)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewPublication(test.project, test.expected, test.through, nil, test.at); err == nil {
				t.Fatal("NewPublication accepted an invalid transition")
			}
		})
	}
}
