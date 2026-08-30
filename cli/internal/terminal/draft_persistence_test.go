package terminal

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
)

type recordingDraftRepository struct {
	mu           sync.Mutex
	writes       []draftSnapshot
	active       int
	maxActive    int
	first        chan struct{}
	releaseFirst chan struct{}
}

func (r *recordingDraftRepository) SaveDraft(sessionID string, message agent.Message) error {
	r.mu.Lock()
	r.active++
	r.maxActive = max(r.maxActive, r.active)
	index := len(r.writes)
	r.writes = append(r.writes, draftSnapshot{sessionID: sessionID, message: message.Clone()})
	if index == 0 && r.first != nil {
		close(r.first)
	}
	r.mu.Unlock()

	if index == 0 && r.releaseFirst != nil {
		<-r.releaseFirst
	}

	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	return nil
}

func (r *recordingDraftRepository) snapshot() ([]draftSnapshot, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]draftSnapshot(nil), r.writes...), r.maxActive
}

func scheduleDraft(t testing.TB, persistence *draftPersistence, text string) {
	t.Helper()
	if err := persistence.Schedule("session", agent.Message{Text: text}); err != nil {
		t.Fatalf("schedule draft %q: %v", text, err)
	}
}

func TestDraftPersistenceSerializesAndCoalescesWrites(t *testing.T) {
	repository := &recordingDraftRepository{first: make(chan struct{}), releaseFirst: make(chan struct{})}
	results := make(chan draftPersistenceResult, 3)
	persistence := newDraftPersistence(repository, func(result draftPersistenceResult) { results <- result })
	scheduleDraft(t, persistence, "first")
	select {
	case <-repository.first:
	case <-time.After(time.Second):
		t.Fatal("first autosave did not start")
	}

	scheduleDraft(t, persistence, "superseded")
	scheduleDraft(t, persistence, "latest")
	close(repository.releaseFirst)
	for range 2 {
		select {
		case <-results:
		case <-time.After(time.Second):
			t.Fatal("autosave result did not arrive")
		}
	}
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	writes, maxActive := repository.snapshot()
	if maxActive != 1 {
		t.Fatalf("maximum concurrent writes = %d, want 1", maxActive)
	}
	if len(writes) != 2 || writes[0].message.Text != "first" || writes[1].message.Text != "latest" {
		t.Fatalf("writes = %+v, want first then latest", writes)
	}
}

func TestDraftPersistenceFlushSupersedesPendingAutosave(t *testing.T) {
	repository := &recordingDraftRepository{first: make(chan struct{}), releaseFirst: make(chan struct{})}
	persistence := newDraftPersistence(repository, nil)
	scheduleDraft(t, persistence, "pending")
	select {
	case <-repository.first:
	case <-time.After(time.Second):
		t.Fatal("pending autosave did not start")
	}
	flushed := make(chan error, 1)
	go func() { flushed <- persistence.Flush("session", agent.Message{Text: "barrier"}) }()
	close(repository.releaseFirst)
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	writes, maxActive := repository.snapshot()
	if maxActive != 1 || len(writes) != 2 || writes[0].message.Text != "pending" || writes[1].message.Text != "barrier" {
		t.Fatalf("writes = %+v, max concurrency = %d", writes, maxActive)
	}
}

func TestDraftPersistenceCloseFlushesPendingAutosave(t *testing.T) {
	repository := &recordingDraftRepository{}
	persistence := newDraftPersistence(repository, nil)
	scheduleDraft(t, persistence, "last visible value")
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	writes, _ := repository.snapshot()
	if len(writes) != 1 || writes[0].message.Text != "last visible value" {
		t.Fatalf("writes = %+v", writes)
	}
	if err := persistence.Flush("session", agent.Message{Text: "too late"}); !errors.Is(err, errDraftPersistenceClosed) {
		t.Fatalf("flush after close error = %v", err)
	}
}

func TestDraftPersistenceRevisionExhaustionPreservesPendingSnapshot(t *testing.T) {
	pending := draftSnapshot{
		revision:  math.MaxUint64,
		sessionID: "session",
		message:   agent.Message{Text: "last addressable draft"},
	}
	persistence := &draftPersistence{
		pending:  &pending,
		revision: math.MaxUint64,
	}

	if err := persistence.Schedule("session", agent.Message{Text: "wrapped draft"}); !errors.Is(err, errDraftPersistenceRevisionExhausted) {
		t.Fatalf("schedule after revision exhaustion error = %v", err)
	}
	if persistence.revision != math.MaxUint64 {
		t.Fatalf("revision = %d, want exhausted revision to remain unchanged", persistence.revision)
	}
	if persistence.pending != &pending || persistence.pending.message.Text != "last addressable draft" {
		t.Fatalf("pending snapshot = %+v, want the last addressable draft to remain authoritative", persistence.pending)
	}
	if err := persistence.Flush("session", agent.Message{Text: "wrapped barrier"}); !errors.Is(err, errDraftPersistenceRevisionExhausted) {
		t.Fatalf("flush after revision exhaustion error = %v", err)
	}
}
