package runtimebinding

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRemoteConnectionRecoversLostDeletionAcknowledgement(t *testing.T) {
	target, owner, connection, created, attempts := remoteDeletionConnection(t, false)
	profile := connection.Profile()
	policy, err := CommandReplayPolicy(&profile)
	if err != nil {
		t.Fatal(err)
	}
	store, err := workbench.OpenMemory(workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err := session.Delete(t.Context(), connection, store, created.ID, policy, mutation.AcknowledgementBackoff())
	if err != nil || result.Outcome != mutation.Confirmed {
		t.Fatalf("remote deletion = %+v, %v", result, err)
	}
	keys := attempts()
	if len(keys) != 2 || keys[0] == "" || keys[1] != keys[0] || keys[0] != string(result.Request.CommandID) {
		t.Fatalf("mutation identities across lost reply = %v", keys)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Discover(t.Context(), flameruntime.CallOptions{}); err != nil {
		t.Fatalf("closing remote CLI connection stopped shared Runtime: %v", err)
	}
	if _, err := target.GetSession(t.Context(), protocol.GetSessionRequest{SessionID: created.ID}, flameruntime.CallOptions{}); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Fatalf("committed deletion = %v", err)
	}
}

func TestRemoteMalformedAcknowledgementRetainsDurableIntentWithoutRetry(t *testing.T) {
	_, _, connection, created, attempts := remoteDeletionConnection(t, true)
	profile := connection.Profile()
	policy, err := CommandReplayPolicy(&profile)
	if err != nil {
		t.Fatal(err)
	}
	store, err := workbench.OpenMemory(workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err := session.Delete(t.Context(), connection, store, created.ID, policy, mutation.AcknowledgementBackoff())
	if result.Outcome != mutation.Unknown || !errors.Is(err, agent.ErrIncompatibleRuntime) || !mutation.OutcomeUnknown(err) {
		t.Fatalf("malformed acknowledgement = %+v, %v", result, err)
	}
	if mutation.AcknowledgementUncertain(err) || len(attempts()) != 1 {
		t.Fatalf("malformed acknowledgement was retried: %v", attempts())
	}
	pending, found := store.PendingSessionDeletion(created.ID)
	if !found || pending.CommandID != result.Request.CommandID {
		t.Fatalf("unknown mutation intent was lost: %+v, %v", pending, found)
	}
}

// The real Endpoint commits through the public embedded binding. The HTTP peer
// then loses or corrupts that acknowledgement, independently of CLI recovery.
func remoteDeletionConnection(t *testing.T, malformed bool) (
	*flameruntime.Runtime, *Owner, *Connection, protocol.Session, func() []string,
) {
	t.Helper()
	configureIntegrationRuntime(t)
	target, err := flameruntime.Open(t.Context(), flameruntime.Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: t.TempDir(), UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := target.Close(); err != nil {
			t.Error(err)
		}
	})
	created, err := target.CreateSession(t.Context(), protocol.CreateSessionRequest{Title: "shared target"}, flameruntime.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/rpc" || request.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("remote request path/auth = %s, %t", request.URL.Path, request.Header.Get("Authorization") != "")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var envelope struct {
			ID     jsontext.Value `json:"id"`
			Method string         `json:"method"`
			Params jsontext.Value `json:"params"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Error(err)
			return
		}
		var result any
		switch envelope.Method {
		case "runtime.discover":
			result, err = target.Discover(request.Context(), flameruntime.CallOptions{RequestMeta: requestMeta("test")})
		case "sessions.delete":
			var parameters protocol.DeleteSessionRequest
			if err := json.Unmarshal(envelope.Params, &parameters); err != nil {
				t.Error(err)
				return
			}
			key := request.Header.Get("Idempotency-Key")
			err = target.DeleteSession(request.Context(), parameters, flameruntime.CommandOptions{
				RequestMeta: requestMeta("test"), IdempotencyKey: key,
				IdempotencyNamespace: request.Header.Get("Idempotency-Namespace"),
			})
			mu.Lock()
			keys = append(keys, key)
			attempt := len(keys)
			mu.Unlock()
			if err == nil && attempt == 1 {
				if malformed {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{}`)
					return
				}
				transport, _, hijackErr := w.(http.Hijacker).Hijack()
				if hijackErr != nil {
					t.Error(hijackErr)
					return
				}
				_ = transport.Close()
				return
			}
			result = struct{}{}
		default:
			t.Errorf("unexpected remote operation %s", envelope.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		encoded, err := json.Marshal(struct {
			JSONRPC string         `json:"jsonrpc"`
			ID      jsontext.Value `json:"id"`
			Result  any            `json:"result"`
		}{JSONRPC: "2.0", ID: envelope.ID, Result: result})
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	}))
	t.Cleanup(server.Close)
	owner := NewOwner(Config{RemoteToken: "test-token", ClientVersion: "test"})
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	connection, err := owner.Connection(t.Context(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return target, owner, connection, *created, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), keys...)
	}
}
