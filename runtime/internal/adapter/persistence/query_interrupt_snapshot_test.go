package persistence

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type pausedInterruptPage struct {
	sessions.QueryInterruptReader
	observed chan struct{}
	resume   chan struct{}
}

func (p *pausedInterruptPage) ListPage(ctx context.Context, sessionID, rootRunID string, createdAt int64, after string, limit int) ([]runs.Pending, error) {
	rows, err := p.QueryInterruptReader.ListPage(ctx, sessionID, rootRunID, createdAt, after, limit)
	if err != nil || p.observed == nil {
		return rows, err
	}
	close(p.observed)
	p.observed = nil
	select {
	case <-p.resume:
		return rows, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestInterruptPageReadsTheHandoffAndItemsFromOneSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	reader, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	writer, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	created := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	sess := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_snapshot", Workspace: testsupport.MustWorkspace("/work"), CreatedAt: created, UpdatedAt: created})
	if err := sqlite.NewSessionStore(writer).Insert(t.Context(), sess); err != nil {
		t.Fatal(err)
	}
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	runStore := sqlite.NewRunStore(writer)
	draft := run.Draft{RunID: "run_snapshot", SessionID: sess.ID(), SegmentID: "seg_snapshot", ModelSelection: testsupport.DefaultModelSelection(), Capabilities: capabilities, CreatedAt: created}
	if err := runStore.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	active, found, err := runStore.Run(t.Context(), draft.RunID)
	if err != nil || !found {
		t.Fatalf("admitted run: found=%t, %v", found, err)
	}
	root, err := active.Suspend(created.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := runStore.Suspend(t.Context(), root, draft.SegmentID, identity.CommitID{}); err != nil {
		t.Fatal(err)
	}
	item := testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item_snapshot", SessionID: sess.ID(), RunID: root.ID(), Kind: transcript.QuestionItem, OccurredAt: created, Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}}})
	if err := sqlite.NewTranscriptStore(writer).AppendItem(t.Context(), item); err != nil {
		t.Fatal(err)
	}
	pending := runs.Pending{RootRunID: root.ID(), ExecutorID: "execution_snapshot", CreatedAt: created.Add(time.Second),
		Interrupts:    []runs.OpenInterrupt{{ItemID: item.ID()}},
		Bindings:      []runs.InterruptBinding{{InterruptItemID: item.ID(), MemberID: "member_snapshot", RequestID: "request_snapshot"}},
		Continuations: []runs.Continuation{{RunID: root.ID(), MemberID: "member_snapshot"}},
	}
	if err := NewInterruptStore(sqlite.NewInterruptStore(writer)).Open(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	pages := &pausedInterruptPage{QueryInterruptReader: NewInterruptStore(sqlite.NewInterruptStore(reader)), observed: make(chan struct{}), resume: make(chan struct{})}
	observed := pages.observed
	resume := sync.OnceFunc(func() { close(pages.resume) })
	defer resume()
	models, err := NewModelInvocationReader(sqlite.NewModelInvocationStore(reader))
	if err != nil {
		t.Fatal(err)
	}
	queries, err := sessions.NewQueryCoordinator(sessions.QueryDependencies{
		ReadSnapshot: func(ctx context.Context, read func(context.Context) error) error {
			return sqlite.RunInTx(ctx, reader, read)
		},
		Transcript: sqlite.NewTranscriptStore(reader), Interrupts: pages,
		Runs: sqlite.NewRunStore(reader), Sessions: sqlite.NewSessionStore(reader), Plan: sqlite.NewPlanStore(reader),
		Trajectory: &TrajectoryReader{store: sqlite.NewTrajectoryStore(reader)}, ModelInvocations: models,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		page, err := queries.ListPendingInterruptPage(t.Context(), sess.ID(), root.ID(), capabilities, "", pagination.DefaultLimit())
		if err == nil {
			if len(page.Rows) != 1 || len(page.Rows[0].Interrupts) != 1 || page.Rows[0].Interrupts[0].Question.Answered() {
				err = fmt.Errorf("interrupt page did not preserve its unanswered snapshot: %+v", page)
			}
		}
		result <- err
	}()
	select {
	case <-observed:
	case err := <-result:
		t.Fatalf("query ended before its handoff read: %v", err)
	}
	change, err := transcript.Replace(item, func(current transcript.Item) (transcript.Item, error) {
		return current.AnswerQuestion([][]string{{"Yes"}})
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sqlite.RunInTx(t.Context(), writer, func(ctx context.Context) error {
		if _, found, err := sqlite.NewInterruptStore(writer).ClaimResume(ctx, root.ID()); err != nil || !found {
			return fmt.Errorf("claim interrupt: found=%t: %w", found, err)
		}
		return sqlite.NewTranscriptStore(writer).ReplaceItem(ctx, change)
	})
	resume()
	if err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	current, err := queries.ListPendingInterruptPage(t.Context(), sess.ID(), root.ID(), capabilities, "", pagination.DefaultLimit())
	if err != nil || len(current.Rows) != 0 {
		t.Fatalf("next page retained the consumed handoff: %+v, %v", current, err)
	}
}
