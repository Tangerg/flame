package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type interleavedItemPage struct {
	*TranscriptStore
	afterRead func() error
}

func (p *interleavedItemPage) complete(rows []transcript.SequencedItem, err error) ([]transcript.SequencedItem, error) {
	if err != nil {
		return nil, err
	}
	if p.afterRead != nil {
		write := p.afterRead
		p.afterRead = nil
		if err := write(); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (p *interleavedItemPage) PageSessionItems(ctx context.Context, id string, order transcript.SequenceOrder, from int64, limit int) ([]transcript.SequencedItem, error) {
	return p.complete(p.TranscriptStore.PageSessionItems(ctx, id, order, from, limit))
}

func (p *interleavedItemPage) PageRunItems(ctx context.Context, id string, order transcript.SequenceOrder, from int64, limit int) ([]transcript.SequencedItem, error) {
	return p.complete(p.TranscriptStore.PageRunItems(ctx, id, order, from, limit))
}

func (p *interleavedItemPage) PageRunTreeItems(ctx context.Context, id string, order transcript.SequenceOrder, from int64, limit int) ([]transcript.SequencedItem, error) {
	return p.complete(p.TranscriptStore.PageRunTreeItems(ctx, id, order, from, limit))
}

func TestItemPageKeepsOneSnapshotAcrossHistoryReplacement(t *testing.T) {
	for _, scopeName := range []string{"session", "run", "subtree"} {
		for _, mutation := range []string{"rollback", "replacement"} {
			t.Run(scopeName+"/"+mutation, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "runtime.db")
				reader, err := openDatabase(t.Context(), path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = reader.Close() })
				writer, err := openDatabase(t.Context(), path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = writer.Close() })
				created := time.Unix(1, 0).UTC()
				sess := testsupport.MustRestoreSession(session.Snapshot{ID: "ses_snapshot", Workspace: testsupport.MustWorkspace("/work"), CreatedAt: created, UpdatedAt: created})
				if err := newSessionStore(writer).Insert(t.Context(), sess); err != nil {
					t.Fatal(err)
				}
				root := testsupport.MustRestoreRun(run.Snapshot{ID: "run_snapshot", SessionID: sess.ID(), State: run.Completed, CreatedAt: created})
				if err := newRunStore(writer).Restore(t.Context(), root); err != nil {
					t.Fatal(err)
				}
				item := testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item_original", SessionID: sess.ID(), RunID: root.ID(), OccurredAt: created})
				if err := newTranscriptStore(writer).AppendItem(t.Context(), item); err != nil {
					t.Fatal(err)
				}
				pages := &interleavedItemPage{TranscriptStore: newTranscriptStore(reader), afterRead: func() error {
					return sqlite.RunInTx(t.Context(), writer, func(ctx context.Context) error {
						if err := newTranscriptStore(writer).DeleteRun(ctx, sess.ID(), root.ID()); err != nil {
							return err
						}
						if err := newRunStore(writer).Delete(ctx, sess.ID(), root.ID()); err != nil || mutation == "rollback" {
							return err
						}
						snapshot := root.Snapshot()
						snapshot.State = run.Canceled
						outcome := run.OutcomeCanceled
						snapshot.Outcome = &outcome
						if err := newRunStore(writer).Restore(ctx, testsupport.MustRestoreRun(snapshot)); err != nil {
							return err
						}
						replacement := testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item_replacement", SessionID: sess.ID(), RunID: root.ID(), OccurredAt: created})
						return newTranscriptStore(writer).AppendItem(ctx, replacement)
					})
				}}
				models, err := NewModelInvocationReader(newModelInvocationStore(reader))
				if err != nil {
					t.Fatal(err)
				}
				queries, err := sessions.NewQueryCoordinator(sessions.QueryDependencies{
					ReadSnapshot: func(ctx context.Context, read func(context.Context) error) error {
						return sqlite.RunReadSnapshot(ctx, reader, read)
					},
					Transcript: pages, Interrupts: newInterruptStore(reader), Runs: newRunStore(reader),
					Sessions: newSessionStore(reader), Plan: newPlanStore(reader),
					Trajectory: &trajectoryReader{store: newTrajectoryStore(reader)}, ModelInvocations: models,
				})
				if err != nil {
					t.Fatal(err)
				}
				scope := sessions.Items(sess.ID())
				switch scopeName {
				case "run":
					scope = sessions.RunItems(root.ID())
				case "subtree":
					scope = sessions.RunTreeItems(root.ID())
				}
				page, err := queries.ListItemPage(t.Context(), scope, transcript.OldestFirst, "", pagination.DefaultLimit())
				if err != nil || len(page.Items) != 1 || page.Items[0].ID() != item.ID() || len(page.Runs) != 1 {
					t.Fatalf("page lost its original history closure: items=%d, runs=%d, error=%v", len(page.Items), len(page.Runs), err)
				}
				if page.Runs[0].State() != run.Completed {
					t.Fatalf("original Item joined replacement Run state %s", page.Runs[0].State())
				}
				current, err := queries.ListItemPage(t.Context(), scope, transcript.OldestFirst, "", pagination.DefaultLimit())
				if mutation == "rollback" {
					if scopeName == "session" {
						if err != nil || len(current.Items) != 0 || len(current.Runs) != 0 {
							t.Fatalf("next Session page retained deleted history: %+v, %v", current, err)
						}
					} else if !errors.Is(err, transcript.ErrRunNotFound) {
						t.Fatalf("next Run page = %v, want deleted Run", err)
					}
				} else if err != nil || len(current.Items) != 1 || current.Items[0].ID() != "item_replacement" || len(current.Runs) != 1 || current.Runs[0].State() != run.Canceled {
					t.Fatalf("next page missed committed replacement: %+v, %v", current, err)
				}
			})
		}
	}
}
