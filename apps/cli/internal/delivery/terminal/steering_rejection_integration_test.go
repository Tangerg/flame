package terminal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestRefusedSteerRestoresItsAttachmentsWithoutAnIntermediateDraftWrite(t *testing.T) {
	provider, entered, finishModel := openSteeringModel(t)
	connection, source := openTerminalIntegrationRuntime(t, provider)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	arrived := make(chan authoring.SteerRun, 1)
	refused := make(chan error, 1)
	release := make(chan struct{})
	backend := &heldTerminalSteerDispatch{Connection: connection, arrived: arrived, refused: refused, release: release}
	directory := t.TempDir()
	config := sessionNavigationConfig(connection, backend, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, currentStore := runUIFromConfig(t, config)
	defer stop()
	host.Shows(t, "Ask flame")
	submit := func(text string) {
		t.Helper()
		if !host.Type(text) || !host.Press(input.Enter) {
			t.Fatalf("terminal refused %q", text)
		}
	}
	submit("start the held model call")
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("Runtime did not enter the loopback provider", context.Cause(ctx))
	}
	host.Shows(t, "holding the first model call")
	const name = "retained-steer-input.txt"
	attachment := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(attachment, []byte("retained input content"), 0o600); err != nil {
		t.Fatal(err)
	}
	submit("/attach " + attachment)
	host.Shows(t, "attached "+name)
	submit("/steer keep the rejected input")
	var original authoring.SteerRun
	select {
	case original = <-arrived:
	case <-ctx.Done():
		t.Fatal("the original steer was not ready for dispatch", context.Cause(ctx))
	}
	if len(original.Message.Attachments) != 1 || original.Message.Attachments[0].Name != name {
		t.Fatalf("original steer attachments = %+v", original.Message.Attachments)
	}
	const draft = "draft after a refused steer"
	database := inspectSessionWorkbench(t, directory)
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER refuse_intermediate_steer_draft
		BEFORE INSERT ON authoring_records WHEN new.key GLOB 'session/*'
		AND json_extract(CAST(new.body AS TEXT), '$.draft.text') = 'draft after a refused steer'
		AND coalesce(json_array_length(CAST(new.body AS TEXT), '$.draft.attachments'), 0) = 0
		BEGIN SELECT RAISE(ABORT, 'intermediate steer draft refused'); END`); err != nil {
		t.Fatal(err)
	}
	if !host.Type(draft) {
		t.Fatal("terminal refused the following draft")
	}
	host.Shows(t, draft)
	host.Shows(t, "workbench: commit workbench state")
	finishModel()
	waitForSteeringState(t, ctx, "the original run to finish", func() (bool, error) {
		snapshot, err := connection.GetSession(ctx, source.ID)
		_, active := snapshot.ActiveRun()
		return !active, err
	})
	close(release)
	select {
	case err := <-refused:
		if err == nil {
			t.Fatal("Runtime accepted steering on a finished run")
		}
	case <-ctx.Done():
		t.Fatal("Runtime did not answer the original steering request", context.Cause(ctx))
	}
	waitForSteeringState(t, ctx, "the refused steer to leave the Workbench journal", func() (bool, error) {
		_, pending := currentStore().PendingSteer(source.ID)
		return !pending, nil
	})
	host.Shows(t, draft)
	host.Shows(t, name)
	host.Hides(t, "workbench: commit workbench state")
	stop()
	reopened, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	want := authoring.Message{Text: draft, Attachments: original.Message.Attachments}
	if saved, _ := reopened.Draft(source.ID); !saved.Equal(want) {
		t.Fatalf("restored draft = %+v, want %+v", saved, want)
	}
	if _, pending := reopened.PendingSteer(source.ID); pending {
		t.Fatal("refused steer survived reopening")
	}
}

func openSteeringModel(t *testing.T) (string, <-chan struct{}, func()) {
	t.Helper()
	entered := make(chan struct{})
	finish, finishModel := context.WithCancel(t.Context())
	var first sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		const opening = `{"id":"chatcmpl_steer_terminal","object":"chat.completion.chunk","created":1,"model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant","content":"holding the first model call"},"finish_reason":null}]}`
		if _, err := fmt.Fprintf(w, "data: %s\n\n", opening); err != nil {
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
		first.Do(func() { close(entered) })
		select {
		case <-finish.Done():
			const closing = `{"id":"chatcmpl_steer_terminal","object":"chat.completion.chunk","created":1,"model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", closing)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(provider.Close)
	t.Cleanup(finishModel)
	return provider.URL, entered, finishModel
}

func waitForSteeringState(t *testing.T, ctx context.Context, label string, read func() (bool, error)) {
	t.Helper()
	const probeInterval = 20 * time.Millisecond
	probe := time.NewTicker(probeInterval)
	defer probe.Stop()
	for {
		ready, err := read()
		if err != nil {
			t.Fatalf("read %s: %v", label, err)
		}
		if ready {
			return
		}
		select {
		case <-probe.C:
		case <-ctx.Done():
			t.Fatalf("waiting for %s: %v", label, context.Cause(ctx))
		}
	}
}

type heldTerminalSteerDispatch struct {
	*runtimebinding.Connection
	arrived chan<- authoring.SteerRun
	refused chan<- error
	release <-chan struct{}
}

func (r *heldTerminalSteerDispatch) SteerRun(ctx context.Context, input authoring.SteerRun) (protocol.SteerRunResponse, error) {
	r.arrived <- input
	select {
	case <-r.release:
	case <-ctx.Done():
		return protocol.SteerRunResponse{}, context.Cause(ctx)
	}
	receipt, err := r.Connection.SteerRun(ctx, input)
	r.refused <- err
	return receipt, err
}
