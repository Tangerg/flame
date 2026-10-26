package coordination

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	modeltext "github.com/Tangerg/flame/packages/go-sdk/model"

	agentmemory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/flame/plugins/memory/internal/sqlite"
)

type scriptedReply struct {
	text string
	err  error
}

type scriptedModel struct {
	mu       sync.Mutex
	replies  []scriptedReply
	requests []Prompt
}

func (s *scriptedModel) Complete(_ context.Context, request Prompt) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if len(s.replies) == 0 {
		return "", errors.New("scripted model exhausted")
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]
	if reply.err != nil {
		return "", reply.err
	}
	return reply.text, nil
}

func memoryConsolidationFixture(t *testing.T, replies ...scriptedReply) (*MemoryConsolidator, *sqlite.AgentMemoryStore, *scriptedModel) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	memory := sqlite.NewAgentMemoryStore(db)
	memoryCuration, err := NewCuration(CurationConfig{Store: memory})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{replies: replies}
	consolidator, err := NewMemoryConsolidator(
		capturedHistory{Conversation{MessageCount: 4, Transcript: "[user] first\n[assistant] reply\n[user] second\n[assistant] reply\n"}},
		memoryCuration,
		model,
	)
	if err != nil {
		t.Fatal(err)
	}
	consolidator.policy.minPendingFacts = 1
	consolidator.now = func() time.Time { return time.Date(2026, 7, 19, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60)) }
	return consolidator, memory, model
}

func TestConsolidatorRejectsMissingDependencies(t *testing.T) {
	valid, _, _ := memoryConsolidationFixture(t)
	for name, construct := range map[string]func() error{
		"conversation": func() error {
			_, err := NewMemoryConsolidator(nil, valid.memory, valid.client)
			return err
		},
		"typed-nil conversation": func() error {
			_, err := NewMemoryConsolidator((*capturedHistory)(nil), valid.memory, valid.client)
			return err
		},
		"memory": func() error {
			_, err := NewMemoryConsolidator(valid.history, nil, valid.client)
			return err
		},
		"typed-nil memory": func() error {
			_, err := NewMemoryConsolidator(valid.history, (*Curation)(nil), valid.client)
			return err
		},
		"completion": func() error {
			_, err := NewMemoryConsolidator(valid.history, valid.memory, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := construct(); err == nil {
				t.Fatal("construction accepted missing dependency")
			}
		})
	}
}

func TestMemoryConsolidatorAppendsDailyLedgerAndCuratesItems(t *testing.T) {
	consolidator, memory, model := memoryConsolidationFixture(t,
		scriptedReply{text: "- use make test\n- prefer concise errors"},
		scriptedReply{text: "- Run `make test`.\n- Prefer concise errors."},
	)
	if err := consolidator.Consolidate(t.Context(), "ses_1", "/repo"); err != nil {
		t.Fatal(err)
	}
	state, err := memory.State(t.Context(), "/repo")
	if err != nil || state.Watermark == 0 {
		t.Fatalf("curation watermark = %+v, err=%v", state, err)
	}

	items, err := memory.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(items) != 2 {
		t.Fatalf("curated items = (%+v, %v)", items, err)
	}
	found := false
	for _, item := range items {
		if item.Status() != agentmemory.StatusPending {
			t.Fatalf("proposal not pending: %+v", item)
		}
		if strings.Contains(item.Content(), "make test") {
			found = true
		}
	}
	if !found {
		t.Fatalf("curated items missing the make-test fact: %+v", items)
	}
	ledger, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(ledger) != 2 || ledger[0].Day != "2026-07-19" {
		t.Fatalf("ledger = (%+v, %v)", ledger, err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("model calls = %d, want extraction + curation", len(model.requests))
	}
	for index, request := range model.requests {
		if request.MaxOutputTokens != int64(defaultMemoryCurationMaxTokens) {
			t.Errorf("model request %d MaxOutputTokens = %v, want %d", index, request.MaxOutputTokens, defaultMemoryCurationMaxTokens)
		}
	}
	curationPrompt := model.requests[1].UserPrompt
	if !strings.Contains(curationPrompt, "[2026-07-19 #") {
		t.Fatalf("curation prompt lacks daily provenance: %q", curationPrompt)
	}
}

func TestBoundedLedgerPrefixNeverSkipsPastOmittedFact(t *testing.T) {
	facts := []agentmemory.LedgerFact{
		{Sequence: 41, Day: "2026-08-24", Content: strings.Repeat("a", 20)},
		{Sequence: 42, Day: "2026-08-24", Content: strings.Repeat("b", 20)},
		{Sequence: 43, Day: "2026-08-24", Content: strings.Repeat("c", 20)},
	}
	firstLineBytes := len(facts[0].Day) + len("41") + len(facts[0].Content) + len("[ #] \n")
	bounded := boundedLedgerPrefix(facts, firstLineBytes)
	if len(bounded) != 1 || bounded[0].Sequence != 41 {
		t.Fatalf("bounded ledger = %+v, want exact prefix through sequence 41", bounded)
	}
}

func TestMemoryConsolidatorLeavesWatermarkOnCurationFailureThenRecovers(t *testing.T) {
	providerFailure := errors.New("provider unavailable")
	consolidator, memory, _ := memoryConsolidationFixture(t,
		scriptedReply{text: "- durable fact"},
		scriptedReply{err: providerFailure},
		scriptedReply{text: "- durable fact"},
	)
	if err := consolidator.Consolidate(t.Context(), "ses_1", "/repo"); !errors.Is(err, providerFailure) {
		t.Fatalf("first extraction error = %v", err)
	}
	state, err := memory.State(t.Context(), "/repo")
	if err != nil || state.Watermark != 0 {
		t.Fatalf("failed curation advanced watermark: (%+v, %v)", state, err)
	}
	if items, itemsErr := memory.Items(t.Context(), agentmemory.ScopeProject, "/repo"); itemsErr != nil || len(items) != 0 {
		t.Fatalf("failed curation published items: (%+v, %v)", items, itemsErr)
	}
	pending, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("durable pending ledger = (%+v, %v)", pending, err)
	}

	consolidator.minMsgs = 100
	if err := consolidator.Consolidate(t.Context(), "ses_1", "/repo"); err != nil {
		t.Fatal(err)
	}
	state, _ = memory.State(t.Context(), "/repo")
	items, _ := memory.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if state.Watermark != pending[0].Sequence || len(items) != 1 || items[0].Content() != "durable fact" {
		t.Fatalf("recovered curation: state=%+v items=%+v", state, items)
	}
}

func TestCurationGateAndTokenEstimate(t *testing.T) {
	consolidator := &MemoryConsolidator{policy: memoryCurationPolicy{minPendingFacts: 3, maxAge: time.Hour}}
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	state := agentmemory.State{Watermark: 1, UpdatedAt: now}
	if consolidator.curationDue(state, 2, now) {
		t.Fatal("small fresh backlog should not curate")
	}
	if !consolidator.curationDue(state, 3, now) {
		t.Fatal("fact threshold should curate")
	}
	if !consolidator.curationDue(state, 1, now.Add(time.Hour)) {
		t.Fatal("age threshold should curate")
	}
	if !consolidator.curationDue(agentmemory.State{}, 1, now) {
		t.Fatal("first generation should curate immediately")
	}

	if tokens := modeltext.EstimateTextTokens(strings.Repeat("界", 100)); tokens != 100 {
		t.Fatalf("CJK token estimate = %d, want 100", tokens)
	}
}

func TestMemoryConsolidatorDoesNotAdvanceWatermarkForOversizedCuration(t *testing.T) {
	consolidator, memory, _ := memoryConsolidationFixture(t,
		scriptedReply{text: "- durable fact"},
		scriptedReply{text: strings.Repeat("界", 20)},
	)
	consolidator.policy.maxTokens = 10
	if err := consolidator.Consolidate(t.Context(), "ses_1", "/repo"); err == nil || !strings.Contains(err.Error(), "limit is 10") {
		t.Fatalf("oversized curation error = %v", err)
	}
	state, err := memory.State(t.Context(), "/repo")
	if err != nil || state.Watermark != 0 {
		t.Fatalf("oversized curation advanced watermark: (%+v, %v)", state, err)
	}
	pending, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("oversized curation lost pending facts: (%+v, %v)", pending, err)
	}
}
