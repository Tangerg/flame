package agentmemory

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

func testItemID(t *testing.T, digit byte) ItemID {
	t.Helper()
	id, err := ParseItemID(ItemIDPrefix + strings.Repeat(
		string(digit),
		MaximumItemIDCharacters-len(ItemIDPrefix),
	))
	if err != nil {
		t.Fatalf("test Item identity: %v", err)
	}
	return id
}

func TestItemIdentityIsCanonicalAndBounded(t *testing.T) {
	id, err := NewItemID()
	if err != nil || id.Validate() != nil || len(id.String()) != MaximumItemIDCharacters {
		t.Fatalf("NewItemID = %q, %v", id.String(), err)
	}
	for _, raw := range []string{"", "mem_1", "mem_" + strings.Repeat("A", 32), " mem_" + strings.Repeat("a", 32)} {
		if _, err := ParseItemID(raw); !errors.Is(err, ErrInvalidItemID) {
			t.Fatalf("ParseItemID(%q) error = %v", raw, err)
		}
	}
}

func TestFactBatchNormalizeValidatesIdentity(t *testing.T) {
	batch := FactBatch{
		Project:    " /repo ",
		SessionID:  "session",
		Day:        "2026-07-19",
		Facts:      []string{"one", "one", "two", " "},
		CapturedAt: time.Now(),
	}
	normalized, err := batch.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Project != "/repo" || normalized.SessionID != "session" || !slices.Equal(normalized.Facts, []string{"one", "two"}) {
		t.Fatalf("normalized batch = %+v", normalized)
	}
	padded := batch
	padded.SessionID = " session "
	if _, err := padded.Normalize(); err == nil {
		t.Fatal("non-exact Session identity was accepted")
	}
	batch.Day = "2026-7-19"
	if _, err := batch.Normalize(); err == nil {
		t.Fatal("non-canonical day was accepted")
	}
}

func TestFactBatchNormalizeRejectsUnboundedExtractionCardinality(t *testing.T) {
	facts := make([]string, 33)
	for index := range facts {
		facts[index] = fmt.Sprintf("fact %d", index)
	}
	batch := FactBatch{
		Project: "/repo", SessionID: "session", Day: "2026-08-24",
		Facts: facts, CapturedAt: time.Now(),
	}
	if _, err := batch.Normalize(); err == nil {
		t.Fatal("fact batch with 33 distinct facts was accepted")
	}
}

func TestClosedVocabularyRoundTrip(t *testing.T) {
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		parsed, err := ParseScope(scope.String())
		if err != nil || parsed != scope {
			t.Fatalf("scope round-trip failed for %v", scope)
		}
	}
	for _, status := range []Status{StatusActive, StatusPending, StatusRejected} {
		parsed, err := ParseStatus(status.String())
		if err != nil || parsed != status {
			t.Fatalf("status round-trip failed for %v", status)
		}
	}
	for _, origin := range []Origin{OriginAuto, OriginUser} {
		parsed, err := ParseOrigin(origin.String())
		if err != nil || parsed != origin {
			t.Fatalf("origin round-trip failed for %v", origin)
		}
	}
	if _, err := ParseScope("garbage"); err == nil {
		t.Fatal("unknown scope was accepted")
	}
	if _, err := ParseStatus("garbage"); err == nil {
		t.Fatal("unknown status was accepted")
	}
	if _, err := ParseOrigin("garbage"); err == nil {
		t.Fatal("unknown origin was accepted")
	}
}

func TestReviewDecisionOwnsResultingStatus(t *testing.T) {
	for _, test := range []struct {
		decision ReviewDecision
		want     Status
	}{
		{decision: ReviewApprove, want: StatusActive},
		{decision: ReviewReject, want: StatusRejected},
	} {
		got, err := test.decision.Result()
		if err != nil || got != test.want {
			t.Fatalf("%q.Result() = (%q, %v), want %q", test.decision, got, err, test.want)
		}
	}
	if _, err := ReviewDecision("later").Result(); err == nil {
		t.Fatal("unknown review decision was accepted")
	}
}

func TestItemConstructionRejectsInvalidPartition(t *testing.T) {
	now := time.Now()
	if _, err := NewProposal(testItemID(t, '1'), "", "fact", now); err == nil {
		t.Fatal("project proposal without project was accepted")
	}
	if _, err := NewUserItem(testItemID(t, '2'), ScopeUser, "/repo", "fact", now); err == nil {
		t.Fatal("user item with project was accepted")
	}
}

func TestItemValidateForProtectsExactTarget(t *testing.T) {
	now := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC)
	item, err := NewUserItem(testItemID(t, '3'), ScopeProject, "/repo", "fact", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := item.ValidateFor(ScopeProject, "/repo"); err != nil {
		t.Fatalf("ValidateFor exact target: %v", err)
	}
	for _, target := range []struct {
		scope   Scope
		project string
	}{
		{scope: ScopeProject, project: "/other"},
		{scope: ScopeUser},
		{scope: ScopeProject},
	} {
		if err := item.ValidateFor(target.scope, target.project); err == nil {
			t.Fatalf("ValidateFor(%q, %q) accepted mismatched or invalid target", target.scope, target.project)
		}
	}
	if err := (Item{}).ValidateFor(ScopeProject, "/repo"); err == nil {
		t.Fatal("zero item was accepted for an exact target")
	}
}

func TestItemActivateFromUserPreservesIdentityAndClearsProposalState(t *testing.T) {
	createdAt := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	proposal, err := NewProposal(testItemID(t, 'a'), "/repo", "old fact", createdAt)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Status = StatusRejected
	proposal.Pinned = true
	proposal.SessionID = "session"
	proposal.EmbeddingSpace = "provider:model"
	proposal.Embedding = []float32{1, 2}
	activated, err := proposal.ActivateFromUser("new fact", updatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if activated.ID != proposal.ID || !activated.CreatedAt.Equal(proposal.CreatedAt) {
		t.Fatalf("activation changed stable identity: %+v", activated)
	}
	if activated.Content != "new fact" || activated.Origin != OriginUser || activated.Status != StatusActive {
		t.Fatalf("activation did not adopt user authorship: %+v", activated)
	}
	if activated.Pinned || activated.SessionID != "" || activated.EmbeddingSpace != "" || activated.Embedding != nil {
		t.Fatalf("activation retained proposal or derived state: %+v", activated)
	}
}

func TestCurationStateOwnsWatermarkTimestampCoherence(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	for _, test := range []struct {
		name    string
		state   State
		wantErr bool
	}{
		{name: "never curated"},
		{name: "negative watermark", state: State{Watermark: -1}, wantErr: true},
		{name: "empty with timestamp", state: State{UpdatedAt: epoch}, wantErr: true},
		{name: "advanced without timestamp", state: State{Watermark: 1}, wantErr: true},
		{name: "advanced at epoch", state: State{Watermark: 1, UpdatedAt: epoch}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.state.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestItemConstructionBoundsContentForModelContext(t *testing.T) {
	now := time.Now()
	if _, err := NewUserItem(
		testItemID(t, 'b'),
		ScopeUser,
		"",
		strings.Repeat("界", MaxContentCharacters),
		now,
	); err != nil {
		t.Fatalf("boundary content was rejected: %v", err)
	}
	if _, err := NewUserItem(
		testItemID(t, 'c'),
		ScopeUser,
		"",
		strings.Repeat("界", MaxContentCharacters+1),
		now,
	); err == nil {
		t.Fatal("content larger than one context-safe memory item was accepted")
	}
	if _, err := NewUserItem(
		testItemID(t, 'd'),
		ScopeUser,
		"",
		string([]byte{0xff}),
		now,
	); err == nil {
		t.Fatal("invalid UTF-8 content was accepted")
	}

	batch := FactBatch{
		Project: "/repo", SessionID: "session", Day: "2026-08-24",
		Facts: []string{strings.Repeat("界", MaxContentCharacters+1)}, CapturedAt: now,
	}
	if _, err := batch.Normalize(); err == nil {
		t.Fatal("oversized ledger fact was accepted")
	}
}

func TestEmbeddingUpdateBindsContentAndDefensivelyCopiesVector(t *testing.T) {
	item := Item{ID: testItemID(t, 'e'), Content: "current content"}
	vector := []float32{1, 2}
	update, err := NewEmbeddingUpdate(item, "provider:model", vector)
	if err != nil {
		t.Fatal(err)
	}
	vector[0] = 9
	if update.ItemID != item.ID || update.ContentDigest != Digest(item.Content) || update.Space != "provider:model" || !slices.Equal(update.Vector, []float32{1, 2}) {
		t.Fatalf("embedding update = %+v", update)
	}
	if err := update.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEmbeddingUpdate(item, "provider:model", []float32{float32(math.NaN())}); err == nil {
		t.Fatal("non-finite embedding vector was accepted")
	}
}
