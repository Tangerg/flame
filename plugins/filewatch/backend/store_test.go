package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/sqlite"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/backend/domain"
	"github.com/Tangerg/flame/plugins/filewatch/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/google/uuid"
)

func fixtureDraft() filewatch.Draft {
	return filewatch.Draft{Title: "Original trigger", Workspace: "/workspace", Paths: []string{"input.txt"}, Instructions: "Original instructions.", Provider: "openai", Model: "private-model"}
}

func configureFixture(t *testing.T, s *store, revision int64, draft *filewatch.Draft) filewatch.Snapshot {
	t.Helper()
	input := rpc.ConfigureInput{ExpectedRevision: revision, Trigger: draft}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	value, err := pluginstorage.Command(t.Context(), s.db, "configure", uuid.NewString(), input, admitted, classify, func(ctx context.Context, input rpc.ConfigureInput) (filewatch.Snapshot, error) {
		return s.configure(ctx, input.ExpectedRevision, input.Trigger)
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func captureFixture(t *testing.T, sequence uint64, generation string) filewatch.Observation {
	t.Helper()
	value, err := filewatch.NewObservation(filewatch.Capture{Generation: generation, Sequence: sequence, Revision: 2, Kind: filewatch.Changed, Paths: []string{"input.txt"}, ObservedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), Trigger: fixtureDraft(), SessionID: "ses_" + uuid.NewString(), RunID: "run_" + uuid.NewString(), Invocation: capability.Invocation{ID: uuid.NewString(), Origin: capability.Automatic, Target: capability.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCaptureSurvivesDefinitionChangesAndReceiptReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "filewatch.db")
	s, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	draft := fixtureDraft()
	configureFixture(t, s, 1, &draft)
	if err := pluginstorage.BindRuntimeTarget(t.Context(), s.db, "original-runtime"); err != nil {
		t.Fatal(err)
	}
	observation := captureFixture(t, 1, uuid.NewString())
	if err := s.capture(t.Context(), observation); err != nil {
		t.Fatal(err)
	}
	if err := s.capture(t.Context(), observation); err != nil {
		t.Fatal(err)
	}
	configureFixture(t, s, 2, nil)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if err := pluginstorage.CheckRuntimeTarget(t.Context(), s.db, "replacement-runtime"); err == nil {
		t.Fatal("retargeted original intent to another Runtime")
	}
	pending, err := s.pending(t.Context())
	if err != nil || len(pending) != 1 || !reflect.DeepEqual(pending[0].Capture, observation.Capture()) {
		t.Fatalf("original intent changed after disable/reopen: %+v %v", pending, err)
	}
	if _, err := s.db.ExecContext(t.Context(), `UPDATE events SET capture='{}'`); err == nil {
		t.Fatal("captured intent was mutable")
	}
	if err := s.acceptRun(t.Context(), observation.Capture().Invocation.ID); err != nil {
		t.Fatal(err)
	}
	if pending, err := s.pending(t.Context()); err != nil || len(pending) != 0 {
		t.Fatalf("confirmed intent remained pending: %v %v", pending, err)
	}
	input := rpc.ConfigureInput{ExpectedRevision: 3, Trigger: &draft}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func() (filewatch.Snapshot, error) {
		return pluginstorage.Command(t.Context(), s.db, "configure", "original-command", input, admitted, classify, func(ctx context.Context, input rpc.ConfigureInput) (filewatch.Snapshot, error) {
			return s.configure(ctx, input.ExpectedRevision, input.Trigger)
		})
	}
	original, err := invoke()
	if err != nil {
		t.Fatal(err)
	}
	configureFixture(t, s, 4, nil)
	replay, err := invoke()
	if err != nil || !reflect.DeepEqual(original, replay) {
		t.Fatalf("command replay advanced configuration: %+v %+v %v", original, replay, err)
	}
}

func TestPendingCapacityNeverEvictsUnconfirmedIntents(t *testing.T) {
	s, err := openStore(t.Context(), filepath.Join(t.TempDir(), "filewatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	draft := fixtureDraft()
	configureFixture(t, s, 1, &draft)
	generation := uuid.NewString()
	for sequence := uint64(1); sequence <= maxPending; sequence++ {
		if err := s.capture(t.Context(), captureFixture(t, sequence, generation)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.capture(t.Context(), captureFixture(t, maxPending+1, generation)); !errors.Is(err, filewatch.ErrCapacity) {
		t.Fatalf("pending overflow was not explicit: %v", err)
	}
	pending, err := s.pending(t.Context())
	if err != nil || len(pending) != maxPending {
		t.Fatalf("pending intent was pruned: %d %v", len(pending), err)
	}
	if err := s.acceptRun(t.Context(), pending[0].Capture.Invocation.ID); err != nil {
		t.Fatal(err)
	}
	for sequence := uint64(maxPending + 1); sequence <= maxEvents+8; sequence++ {
		observation := captureFixture(t, sequence, generation)
		if err := s.capture(t.Context(), observation); err != nil {
			t.Fatal(err)
		}
		if err := s.acceptRun(t.Context(), observation.Capture().Invocation.ID); err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.headers(t.Context())
	if err != nil || len(events) != maxEvents {
		t.Fatalf("archive is not bounded: %d %v", len(events), err)
	}
	pending, err = s.pending(t.Context())
	if err != nil || len(pending) != maxPending-1 {
		t.Fatalf("archive pruning removed pending facts: %d %v", len(pending), err)
	}
}

func TestCorruptCaptureIsNotAKnownBusinessRefusal(t *testing.T) {
	s, err := openStore(t.Context(), filepath.Join(t.TempDir(), "filewatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	draft := fixtureDraft()
	configureFixture(t, s, 1, &draft)
	capture := captureFixture(t, 1, uuid.NewString()).Capture()
	capture.Trigger.Paths = []string{"../escape"}
	encoded, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), `INSERT INTO events(id,generation,sequence,capture,accepted) VALUES(?,?,?,?,0)`, capture.Invocation.ID, capture.Generation, capture.Sequence, string(encoded)); err != nil {
		t.Fatal(err)
	}
	_, err = s.pending(t.Context())
	if err == nil || classify(err) != "" {
		t.Fatalf("corruption became a completed refusal: %v", err)
	}
}
