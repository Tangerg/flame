package bootstrap

import (
	"context"
	"database/sql/driver"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	modernsqlite "modernc.org/sqlite"
)

const (
	sessionMutationKey  = "original-session-mutation"
	mutatedSessionTitle = "Original command title"
	laterSessionTitle   = "Later session title"
	sessionAckWrite     = "receipt write"
	sessionAckCommitted = "receipt commit"
)

type sessionMutationCrashInput struct {
	Method  delivery.Name  `json:"method"`
	Request jsontext.Value `json:"request"`
}

func TestSessionMutationAcknowledgementCrashHelper(t *testing.T) {
	cut := os.Getenv("FLAME_TEST_SESSION_MUTATION_ACK_CUT")
	if cut == "" {
		return
	}
	if cut == sessionAckWrite {
		if err := modernsqlite.RegisterScalarFunction("test_crash_session_mutation_ack", 0, func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := os.ReadFile(os.Getenv("FLAME_TEST_SESSION_MUTATION_ACK_INPUT"))
	if err != nil {
		t.Fatal(err)
	}
	var input sessionMutationCrashInput
	if err := json.Unmarshal(encoded, &input); err != nil {
		t.Fatal(err)
	}
	request := decodeSessionMutationCrashInput(t, input)
	host, api, stores := openProtocolRuntimeWithStores(t, newReplyStub("unused"))
	defer host.Close()
	endpoint := host.Endpoint()
	switch cut {
	case sessionAckWrite:
		questionCrashDatabase(t, os.Getenv("FLAME_HOME"), `CREATE TRIGGER session_mutation_ack_crash
			BEFORE UPDATE OF payload ON idempotency_records BEGIN SELECT test_crash_session_mutation_ack(); END`)
	case sessionAckCommitted:
		endpoint, err = delivery.NewEndpoint(api, delivery.EndpointConfig{
			Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
			IdempotencyStore: atomicReceiptCommitCrashStore{ReplayStore: stores.Idempotency},
			ValidateStorage:  stores.ValidateDirectory,
		})
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown mutation cut %q", cut)
	}
	result := endpoint.Invoke(t.Context(), input.Method, request, protocol.CallMetadata{IdempotencyKey: sessionMutationKey})
	t.Fatalf("session mutation did not reach the acknowledgement cut: %+v", result.Failure)
}

func decodeSessionMutationCrashInput(t *testing.T, input sessionMutationCrashInput) any {
	t.Helper()
	switch input.Method {
	case delivery.SessionsUpdate:
		return decodeCrashRequest[protocol.UpdateSessionRequest](t, input.Request)
	case delivery.SessionsDelete:
		return decodeCrashRequest[protocol.DeleteSessionRequest](t, input.Request)
	case delivery.SessionsImport:
		return decodeCrashRequest[protocol.ImportSessionRequest](t, input.Request)
	default:
		t.Fatalf("unknown mutation %q", input.Method)
		return nil
	}
}

func decodeCrashRequest[T any](t *testing.T, data jsontext.Value) T {
	t.Helper()
	var request T
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestSessionMutationAcknowledgementsSurviveProcessCrash(t *testing.T) {
	for _, method := range []delivery.Name{delivery.SessionsUpdate, delivery.SessionsDelete, delivery.SessionsImport} {
		t.Run(string(method), func(t *testing.T) {
			for _, cut := range []string{sessionAckWrite, sessionAckCommitted} {
				t.Run(cut, func(t *testing.T) { testSessionMutationAcknowledgementCrash(t, method, cut) })
			}
		})
	}
}

func testSessionMutationAcknowledgementCrash(t *testing.T, method delivery.Name, cut string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	first, api := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	original, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Before command"})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: original.ID})
	if err != nil || archive == nil || archive.Artifact == nil {
		t.Fatalf("export original Session: %+v, %v", archive, err)
	}
	request := sessionMutationCrashRequest(method, original, *archive.Artifact)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	parameters, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(sessionMutationCrashInput{Method: method, Request: parameters})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session-mutation.json")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	crashCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestSessionMutationAcknowledgementCrashHelper$")
	child.Env = append(os.Environ(), "FLAME_TEST_SESSION_MUTATION_ACK_CUT="+cut, "FLAME_TEST_SESSION_MUTATION_ACK_INPUT="+path)
	output, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("session mutation did not reach the receipt cut: %v\n%s", err, output)
	}
	if cut == sessionAckWrite {
		questionCrashDatabase(t, home, "DROP TRIGGER session_mutation_ack_crash")
	}

	restarted, api := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint := restarted.Endpoint()
	meta := protocol.CallMetadata{IdempotencyKey: sessionMutationKey}
	if cut == sessionAckWrite {
		current, err := api.GetSession(ctx, original.ID)
		if err != nil || current.Title != original.Title || current.Revision != original.Revision {
			t.Fatalf("mutation moved the Session before its receipt committed: %+v, %v", current, err)
		}
		expectSessionMutationReceipt(t, method, endpoint.Invoke(ctx, method, request, meta), original)
	}

	var later *protocol.Session
	current, err := api.GetSession(ctx, original.ID)
	if method == delivery.SessionsDelete {
		if !errors.Is(err, protocol.ErrSessionNotFound) {
			t.Fatalf("committed deletion left its original Session: %+v, %v", current, err)
		}
		artifact := *archive.Artifact
		artifact.Session.Title = laterSessionTitle
		restored, err := api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: artifact})
		if err != nil {
			t.Fatal(err)
		}
		later = restored.Session
	} else {
		if err != nil || current.Title != mutatedSessionTitle || current.Revision != original.Revision+1 {
			t.Fatalf("the committed mutation lost its title or revision: %+v, %v", current, err)
		}
		later, err = api.UpdateSession(ctx, protocol.UpdateSessionRequest{
			SessionID: original.ID, ExpectedRevision: current.Revision, Title: new(laterSessionTitle),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		expectSessionMutationReceipt(t, method, endpoint.Invoke(ctx, method, request, meta), original)
	}
	other, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Different Session"})
	if err != nil {
		t.Fatal(err)
	}
	different := *archive.Artifact
	different.Session.ID = other.ID
	changed := sessionMutationCrashRequest(method, other, different)
	if result := endpoint.Invoke(ctx, method, changed, meta); !errors.Is(result.Failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("changed request reused the original key: %+v", result)
	}
	current, err = api.GetSession(ctx, original.ID)
	if err != nil || current.Title != later.Title || current.Revision != later.Revision {
		t.Fatalf("replayed receipt changed a later Session version: %+v, %v", current, err)
	}
}

func sessionMutationCrashRequest(method delivery.Name, session *protocol.Session, archive protocol.SessionArtifact) any {
	switch method {
	case delivery.SessionsUpdate:
		return protocol.UpdateSessionRequest{
			SessionID: session.ID, ExpectedRevision: session.Revision, Title: new(mutatedSessionTitle),
		}
	case delivery.SessionsDelete:
		return protocol.DeleteSessionRequest{SessionID: session.ID}
	case delivery.SessionsImport:
		archive.Session.Title = mutatedSessionTitle
		return protocol.ImportSessionRequest{Artifact: archive}
	default:
		panic("unknown fixture Session mutation")
	}
}

func expectSessionMutationReceipt(t *testing.T, method delivery.Name, result protocol.OperationResult, original *protocol.Session) {
	t.Helper()
	if result.Failure != nil {
		t.Fatalf("same command could not return its original receipt: %v", result.Failure)
	}
	if method == delivery.SessionsDelete {
		if _, ok := result.Value.(struct{}); !ok {
			t.Fatalf("delete receipt = %+v, want empty acknowledgement", result.Value)
		}
		return
	}
	var session *protocol.Session
	switch value := result.Value.(type) {
	case *protocol.Session:
		session = value
	case *protocol.ImportSessionResponse:
		session = value.Session
	default:
		t.Fatalf("unexpected Session receipt type: %T", result.Value)
	}
	if session == nil || session.ID != original.ID || session.Title != mutatedSessionTitle || session.Revision != original.Revision+1 {
		t.Fatalf("receipt drifted to a later Session: %+v", session)
	}
}
