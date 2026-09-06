package agentmemory

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

type fakeCurationStore struct {
	published       bool
	err             error
	appended        []domain.LedgerFact
	pending         []domain.LedgerFact
	state           domain.State
	items           []domain.Item
	appendBatch     domain.FactBatch
	reconcileValues []string
	reconcileAt     time.Time
}

type singleWinnerCurationStore struct {
	fakeCurationStore
	won atomic.Bool
}

func (s *singleWinnerCurationStore) Reconcile(context.Context, domain.Publication) (bool, error) {
	return s.won.CompareAndSwap(false, true), nil
}

func (f *fakeCurationStore) AppendLedger(_ context.Context, batch domain.FactBatch) ([]domain.LedgerFact, error) {
	f.appendBatch = batch
	f.appendBatch.Facts = slices.Clone(batch.Facts)
	return slices.Clone(f.appended), f.err
}

func (f *fakeCurationStore) PendingLedger(context.Context, string, int64, int) ([]domain.LedgerFact, error) {
	return slices.Clone(f.pending), f.err
}

func (f *fakeCurationStore) State(context.Context, string) (domain.State, error) {
	return f.state, f.err
}

func (f *fakeCurationStore) Reconcile(_ context.Context, publication domain.Publication) (bool, error) {
	f.reconcileValues = publication.Contents()
	f.reconcileAt = publication.State().UpdatedAt
	return f.published, f.err
}

func (f *fakeCurationStore) Items(context.Context, domain.Scope, string) ([]domain.Item, error) {
	return cloneMemoryItems(f.items), f.err
}

func TestCurationReconcilePublishesOnlyNewGeneration(t *testing.T) {
	store := &fakeCurationStore{published: true}
	var notices []invalidation.Notice
	c := newCuration(t, CurationConfig{Store: store, Invalidations: func(notice invalidation.Notice) {
		notices = append(notices, notice)
	}})

	now := time.Now()
	published, err := c.PublishGeneration(t.Context(), testPublication(t, domain.State{}, 4, []string{"fact"}, now))
	if err != nil || !published {
		t.Fatalf("Reconcile = (%t, %v), want (true, nil)", published, err)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.AgentMemory {
		t.Fatalf("notices = %+v, want one AgentMemory invalidation", notices)
	}

	store.published = false
	published, err = c.PublishGeneration(t.Context(), testPublication(t, domain.State{}, 4, []string{"stale"}, now))
	if err != nil || published {
		t.Fatalf("stale Reconcile = (%t, %v), want (false, nil)", published, err)
	}
	if len(notices) != 1 {
		t.Fatalf("lost CAS notices = %+v", notices)
	}
}

func TestCurationFailureAndLedgerWritesDoNotPublish(t *testing.T) {
	wantErr := errors.New("store unavailable")
	store := &fakeCurationStore{published: true, err: wantErr}
	var notices []invalidation.Notice
	c := newCuration(t, CurationConfig{Store: store, Invalidations: func(notice invalidation.Notice) {
		notices = append(notices, notice)
	}})

	if published, err := c.PublishGeneration(t.Context(), testPublication(t, domain.State{}, 4, nil, time.Now())); published || !errors.Is(err, wantErr) {
		t.Fatalf("failed Reconcile = (%t, %v), want (false, %v)", published, err, wantErr)
	}
	store.err = nil
	if _, err := c.AppendLedger(t.Context(), validFactBatch("fact")); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 0 {
		t.Fatalf("non-public writes published notices = %+v", notices)
	}
}

func validFactBatch(facts ...string) domain.FactBatch {
	return domain.FactBatch{
		Project: "/repo", SessionID: "ses_1", Day: "2026-09-04", Facts: facts,
		CapturedAt: time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC),
	}
}

func validLedgerFact(sequence int64, content string) domain.LedgerFact {
	batch := validFactBatch(content)
	return domain.LedgerFact{
		Sequence: sequence, Day: batch.Day, Content: content, CapturedAt: batch.CapturedAt,
	}
}

func testPublication(t *testing.T, expected domain.State, through int64, contents []string, now time.Time) domain.Publication {
	t.Helper()
	publication, err := domain.NewPublication("/repo", expected, through, contents, now)
	if err != nil {
		t.Fatalf("prepare curation publication: %v", err)
	}
	return publication
}

func TestCurationValidatesDurableMaterial(t *testing.T) {
	store := &fakeCurationStore{appended: []domain.LedgerFact{validLedgerFact(4, "fact")}}
	curation := newCuration(t, CurationConfig{Store: store})
	appended, err := curation.AppendLedger(t.Context(), validFactBatch(" fact ", "fact"))
	if err != nil || len(appended) != 1 || len(store.appendBatch.Facts) != 1 || store.appendBatch.Facts[0] != "fact" {
		t.Fatalf("AppendLedger = (%+v, %v), normalized batch = %+v", appended, err, store.appendBatch)
	}

	store.appended = []domain.LedgerFact{validLedgerFact(4, "other")}
	if _, err := curation.AppendLedger(t.Context(), validFactBatch("fact")); err == nil {
		t.Fatal("unrequested append acknowledgement was accepted")
	}

	store.pending = []domain.LedgerFact{validLedgerFact(3, "three"), validLedgerFact(2, "two")}
	if _, err := curation.PendingLedger(t.Context(), "/repo", 1, 2); err == nil {
		t.Fatal("out-of-order pending ledger was accepted")
	}
	store.pending = nil
	if _, err := curation.PendingLedger(t.Context(), "/repo", -1, 2); err == nil {
		t.Fatal("negative pending watermark was accepted")
	}

	store.state = domain.State{Watermark: 1}
	if _, err := curation.State(t.Context(), "/repo"); err == nil {
		t.Fatal("invalid curation state was accepted")
	}

	store.items = []domain.Item{readModelItem(t, '1', domain.ScopeProject, "/other", "foreign")}
	if _, err := curation.Items(t.Context(), domain.ScopeProject, "/repo"); err == nil {
		t.Fatal("foreign curation item was accepted")
	}
	first := readModelItem(t, '1', domain.ScopeProject, "/repo", "first")
	second := readModelItem(t, '2', domain.ScopeProject, "/repo", "second")
	store.items = []domain.Item{first, second}
	if items, err := curation.Items(t.Context(), domain.ScopeProject, "/repo"); err != nil ||
		len(items) != 2 || items[0].ID != second.ID {
		t.Fatalf("curation item order = (%+v, %v)", items, err)
	}
}

func TestAppendLedgerTransfersResultsAndAllowsCallerReuse(t *testing.T) {
	store := &fakeCurationStore{appended: []domain.LedgerFact{validLedgerFact(4, "fact")}}
	batch := validFactBatch("fact")
	appended, err := newCuration(t, CurationConfig{Store: store}).AppendLedger(t.Context(), batch)
	if err != nil || len(appended) != 1 || appended[0].Content != "fact" {
		t.Fatalf("AppendLedger = (%+v, %v), want one acknowledged fact", appended, err)
	}
	batch.Facts[0] = "caller reuse"
	appended[0].Content = "result reuse"
	if store.appendBatch.Facts[0] != "fact" || store.appended[0].Content != "fact" {
		t.Fatal("caller reuse changed stored facts")
	}
}

func TestPublishGenerationNormalizesBeforeStore(t *testing.T) {
	store := &fakeCurationStore{published: true}
	curation := newCuration(t, CurationConfig{Store: store})
	local := time.Date(2026, time.September, 4, 16, 0, 0, 0, time.FixedZone("local", 8*60*60))
	expected := domain.State{Watermark: 1, UpdatedAt: local.Add(-time.Hour).UTC()}
	publication := testPublication(t, expected, 2, []string{" fact ", "fact"}, local)
	published, err := curation.PublishGeneration(t.Context(), publication)
	if err != nil || !published {
		t.Fatalf("PublishGeneration = (%t, %v)", published, err)
	}
	if len(store.reconcileValues) != 1 || store.reconcileValues[0] != "fact" || store.reconcileAt.Location() != time.UTC {
		t.Fatalf("reconcile input = %v at %s", store.reconcileValues, store.reconcileAt)
	}
	if _, err := domain.NewPublication("/repo", publication.State(), 2, nil, local); err == nil {
		t.Fatal("non-advancing generation was accepted")
	}
}

func TestConcurrentCurationPublishesExactlyOneWinningGeneration(t *testing.T) {
	store := &singleWinnerCurationStore{}
	var notices atomic.Int32
	c := newCuration(t, CurationConfig{Store: store, Invalidations: func(notice invalidation.Notice) {
		if notice.Resource != invalidation.AgentMemory {
			t.Errorf("notice = %+v, want AgentMemory", notice)
		}
		notices.Add(1)
	}})

	const contenders = 32
	publication := testPublication(t, domain.State{}, 4, []string{"fact"}, time.Now())
	var published atomic.Int32
	var group sync.WaitGroup
	group.Add(contenders)
	for range contenders {
		go func() {
			defer group.Done()
			won, err := c.PublishGeneration(t.Context(), publication)
			if err != nil {
				t.Errorf("Reconcile error = %v", err)
			}
			if won {
				published.Add(1)
			}
		}()
	}
	group.Wait()
	if got := published.Load(); got != 1 {
		t.Fatalf("published generations = %d, want 1", got)
	}
	if got := notices.Load(); got != 1 {
		t.Fatalf("AgentMemory invalidations = %d, want 1", got)
	}
}

func TestCurationRequiresStorage(t *testing.T) {
	var missingStore *fakeCurationStore
	for _, store := range []CurationStore{nil, missingStore} {
		if c, err := NewCuration(CurationConfig{Store: store}); err == nil || c != nil {
			t.Fatalf("NewCuration = (%v, %v), want invalid construction", c, err)
		}
	}
}

func newCuration(t *testing.T, cfg CurationConfig) *Curation {
	t.Helper()
	curation, err := NewCuration(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return curation
}
