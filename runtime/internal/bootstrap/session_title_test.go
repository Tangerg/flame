package bootstrap

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
)

func TestGeneratedSessionTitleFollowsItsOriginalHistory(t *testing.T) {
	const generatedTitle = "Title From Original History"
	const manualTitle = "My chosen title"
	for _, test := range []struct {
		name      string
		mutate    func(context.Context, *testing.T, *delivery.Handler, protocol.SessionArtifact, string) error
		wantTitle string
	}{
		{
			name: "restore history",
			mutate: func(ctx context.Context, _ *testing.T, api *delivery.Handler, empty protocol.SessionArtifact, _ string) error {
				_, err := api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: empty})
				return err
			},
		},
		{
			name: "delete and reimport",
			mutate: func(ctx context.Context, _ *testing.T, api *delivery.Handler, empty protocol.SessionArtifact, _ string) error {
				if err := api.DeleteSession(ctx, empty.Session.ID); err != nil {
					return err
				}
				_, err := api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: empty})
				return err
			},
		},
		{
			name: "rollback history",
			mutate: func(ctx context.Context, _ *testing.T, api *delivery.Handler, empty protocol.SessionArtifact, _ string) error {
				_, err := api.RollbackSession(ctx, protocol.RollbackSessionRequest{
					SessionID: empty.Session.ID, RestoreType: protocol.RestoreHistory,
				})
				return err
			},
		},
		{
			name: "rollback keeps original Run",
			mutate: func(ctx context.Context, t *testing.T, api *delivery.Handler, empty protocol.SessionArtifact, originalRunID string) error {
				_, events, err := api.StartRun(ctx, protocol.StartRunRequest{
					SessionID: empty.Session.ID,
					Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "A follow-up to discard."}},
				})
				if err != nil {
					return err
				}
				waitForRunEvents(t, collectRunEvents(events), "follow-up Run to finish")
				result, err := api.RollbackSession(ctx, protocol.RollbackSessionRequest{
					SessionID: empty.Session.ID, ToRunID: originalRunID, RestoreType: protocol.RestoreHistory,
				})
				if err == nil && len(result.DroppedRuns) != 1 {
					t.Fatalf("rollback dropped %d Runs, want one follow-up", len(result.DroppedRuns))
				}
				return err
			},
			wantTitle: generatedTitle,
		},
		{
			name: "edit favorite",
			mutate: func(ctx context.Context, _ *testing.T, api *delivery.Handler, empty protocol.SessionArtifact, _ string) error {
				current, err := api.GetSession(ctx, empty.Session.ID)
				if err != nil {
					return err
				}
				_, err = api.UpdateSession(ctx, protocol.UpdateSessionRequest{
					SessionID: current.ID, ExpectedRevision: current.Revision, Favorite: new(true),
				})
				return err
			},
			wantTitle: generatedTitle,
		},
		{
			name: "explicit title",
			mutate: func(ctx context.Context, _ *testing.T, api *delivery.Handler, empty protocol.SessionArtifact, _ string) error {
				current, err := api.GetSession(ctx, empty.Session.ID)
				if err != nil {
					return err
				}
				_, err = api.UpdateSession(ctx, protocol.UpdateSessionRequest{
					SessionID: current.ID, ExpectedRevision: current.Revision, Title: new(manualTitle),
				})
				return err
			},
			wantTitle: manualTitle,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("FLAME_HOME", home)
			titleEntered := make(chan struct{}, 1)
			releaseTitle := make(chan struct{})
			model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				if len(request.Tools) != 0 {
					return completedTextResponse("Original work completed."), nil
				}
				select {
				case titleEntered <- struct{}{}:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				select {
				case <-releaseTitle:
					return completedTextResponse(generatedTitle), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})}
			host, api := openProtocolRuntime(t, model)
			t.Cleanup(func() {
				if err := host.Close(); err != nil {
					t.Error(err)
				}
			})
			ctx, cancel := context.WithTimeout(t.Context(), lifecycleWaitBudget(t))
			defer cancel()
			if _, err := api.SetUtilityRole(ctx, protocol.UtilityRole{Provider: "anthropic", Model: "claude-opus-5"}); err != nil {
				t.Fatal(err)
			}
			created, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
			if err != nil {
				t.Fatal(err)
			}
			empty, err := api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: created.ID})
			if err != nil || empty.Artifact == nil {
				t.Fatalf("export empty Session: %v", err)
			}
			started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
				SessionID: created.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Inspect original history."}},
			})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "originating Run to finish")
			select {
			case <-titleEntered:
			case <-ctx.Done():
				t.Fatal("utility model did not begin generating the title")
			}
			if err := test.mutate(ctx, t, api, *empty.Artifact, started.RunID); err != nil {
				t.Fatal(err)
			}
			updated, err := api.GetSession(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			close(releaseTitle)
			if err := host.lifetime.runEffectTasks.Drain(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case <-titleEntered:
				t.Fatal("a second title generation started while the original title was pending")
			default:
			}
			settled, err := api.GetSession(ctx, created.ID)
			if err != nil || settled.Title != test.wantTitle || settled.Favorite != updated.Favorite {
				t.Fatalf("settled Session = %+v, %v, want title %q and favorite %t", settled, err, test.wantTitle, updated.Favorite)
			}
			wantRevision := updated.Revision
			if test.wantTitle == generatedTitle {
				wantRevision++
			}
			if settled.Revision != wantRevision {
				t.Fatalf("settled Session revision = %d, want %d", settled.Revision, wantRevision)
			}
		})
	}
}
