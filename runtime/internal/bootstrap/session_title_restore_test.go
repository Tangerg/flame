package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRefusedHistoryChangePreservesOriginalGeneratedTitle(t *testing.T) {
	const (
		provider  = "openai"
		model     = "gpt-5.6-sol"
		wantTitle = "Title From Original History"
	)
	type titleAttempt struct {
		started   chan struct{}
		proceed   chan struct{}
		exchanges atomic.Int32
	}
	var active atomic.Pointer[titleAttempt]
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/responses/input_tokens"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"object":"response.input_tokens","input_tokens":16}`)
		case strings.HasSuffix(r.URL.Path, "/responses"):
			attempt := active.Load()
			if attempt == nil {
				t.Error("generation began before the test admitted a Run")
				http.Error(w, "unexpected call", http.StatusBadRequest)
				return
			}
			text := "Original work completed"
			switch attempt.exchanges.Add(1) {
			case 1:
			case 2:
				close(attempt.started)
				select {
				case <-attempt.proceed:
				case <-r.Context().Done():
					return
				}
				text = wantTitle
			default:
				t.Error("unexpected additional generation call")
				http.Error(w, "unexpected call", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf(`{"type":"response.output_text.delta","item_id":"message","output_index":0,"content_index":0,"delta":%q}`, text))
			_, _ = fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf(`{"type":"response.completed","response":{"id":"answer","model":%q,"status":"completed","output":[{"type":"message","id":"message","role":"assistant","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":7,"output_tokens":3}}}`, model, text))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection(provider, model)
	runtime := poMCPOpen(t, cfg)
	activateModelProviders(runtime)
	runtime.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{
		Provider: provider,
		APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: new("test-key")},
		BaseURL:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &upstream.URL},
	}, "configure-title-provider")
	if _, err := runtime.api.SetUtilityRole(t.Context(), protocol.UtilityRole{Provider: provider, Model: model}); err != nil {
		t.Fatal(err)
	}

	directory, err := localruntime.DataDirectoryAt(cfg.stores.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", directory.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, test := range []struct {
		name        string
		refuseWrite bool
		want        error
		mutate      func(context.Context, protocol.SessionArtifact) error
	}{
		{
			name: "Plan author refused restore", want: protocol.ErrPluginUnavailable,
			mutate: func(ctx context.Context, artifact protocol.SessionArtifact) error {
				artifact.Plan = []protocol.PlanStep{{ID: "step-1", Description: "Review restored checklist", Status: protocol.PlanStatusPending}}
				_, err := runtime.api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: artifact})
				return err
			},
		},
		{
			name: "restore transaction failed", refuseWrite: true,
			mutate: func(ctx context.Context, artifact protocol.SessionArtifact) error {
				_, err := runtime.api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: artifact})
				return err
			},
		},
		{
			name: "delete transaction failed", refuseWrite: true,
			mutate: func(ctx context.Context, artifact protocol.SessionArtifact) error {
				return runtime.api.DeleteSession(ctx, artifact.Session.ID)
			},
		},
		{
			name: "rollback transaction failed", refuseWrite: true,
			mutate: func(ctx context.Context, artifact protocol.SessionArtifact) error {
				_, err := runtime.api.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: artifact.Session.ID, RestoreType: protocol.RestoreHistory})
				return err
			},
		},
	} {
		if !t.Run(test.name, func(t *testing.T) {
			attempt := &titleAttempt{started: make(chan struct{}), proceed: make(chan struct{})}
			active.Store(attempt)
			releaseTitle := sync.OnceFunc(func() { close(attempt.proceed) })
			t.Cleanup(releaseTitle)
			ctx := protocolLifecycleContext(t.Context())
			session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{})
			if err != nil {
				t.Fatal(err)
			}
			empty, err := runtime.api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: session.ID})
			if err != nil || empty.Artifact == nil {
				t.Fatalf("export empty Session: %v", err)
			}
			run, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Inspect the original history."}},
			})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "original Run to settle")
			select {
			case <-attempt.started:
			case <-ctx.Done():
				t.Fatal("utility provider did not begin the original title call")
			}

			if test.refuseWrite {
				if _, err := db.ExecContext(ctx, `CREATE TRIGGER refuse_title_history BEFORE DELETE ON runs BEGIN SELECT RAISE(ABORT, 'title history write refused'); END`); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := db.Exec("DROP TRIGGER refuse_title_history"); err != nil {
						t.Error(err)
					}
				}()
			}
			err = test.mutate(ctx, *empty.Artifact)
			if test.refuseWrite {
				if err == nil || !strings.Contains(err.Error(), "title history write refused") {
					t.Fatalf("expected storage to refuse the history write, got %v", err)
				}
			} else if !errors.Is(err, test.want) {
				t.Fatalf("history mutation = %v, want %v", err, test.want)
			}
			if _, err := runtime.api.GetRun(ctx, protocol.GetRunRequest{RunID: run.RunID}); err != nil {
				t.Fatalf("refused mutation changed the original Run: %v", err)
			}
			releaseTitle()
			if err := runtime.host.lifetime.runEffectTasks.Drain(ctx); err != nil {
				t.Fatal(err)
			}
			titled, err := runtime.api.GetSession(ctx, session.ID)
			if err != nil || titled.Title != wantTitle || attempt.exchanges.Load() != 2 {
				t.Fatalf("Session = %+v, %v, generations = %d, want original title %q", titled, err, attempt.exchanges.Load(), wantTitle)
			}
		}) {
			break
		}
	}
}
