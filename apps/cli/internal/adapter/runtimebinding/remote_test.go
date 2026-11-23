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
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	flameruntime "github.com/Tangerg/flame/runtime"
)

func TestRemoteConnectionRecoversLostDeletionAcknowledgement(t *testing.T) {
	target, owner, connection, created, attempts := remoteDeletionConnection(t, false)
	profile := connection.Profile()
	policy, err := mutation.PolicyFromProfile(&profile, time.Now)
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
	if _, err := target.Discover(t.Context(), protocol.CallOptions{}); err != nil {
		t.Fatalf("closing remote CLI connection stopped shared Runtime: %v", err)
	}
	if _, err := target.GetSession(t.Context(), protocol.GetSessionRequest{SessionID: created.ID}, protocol.CallOptions{}); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Fatalf("committed deletion = %v", err)
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
		DataDirectory: runtimefixture.TempDirectory(t), DefaultWorkspacePath: runtimefixture.TempDirectory(t), UserHomePath: runtimefixture.TempDirectory(t), ConfigDirectories: []string{runtimefixture.TempDirectory(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := target.Close(); err != nil {
			t.Error(err)
		}
	})
	created, err := target.CreateSession(t.Context(), protocol.CreateSessionRequest{Title: "shared target"}, protocol.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteDeletionServer{t: t, target: target, malformed: malformed}
	server := httptest.NewServer(remote)
	t.Cleanup(server.Close)
	owner := NewOwner(Config{RemoteToken: "test-token", ClientVersion: "test"})
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	connection, err := owner.Connection(t.Context(), settings.Runtime{Mode: settings.RuntimeRemote, Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return target, owner, connection, *created, remote.attemptKeys
}

// remoteDeletionServer serves the Runtime over JSON-RPC and loses the first
// successful deletion reply (a dropped connection, or a malformed body), so
// the client must retry with the same idempotency key.
type remoteDeletionServer struct {
	t         *testing.T
	target    *flameruntime.Runtime
	malformed bool
	mu        sync.Mutex
	keys      []string
}

func (s *remoteDeletionServer) attemptKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.keys...)
}

func (s *remoteDeletionServer) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/v2/rpc" || request.Header.Get("Authorization") != "Bearer test-token" {
		s.t.Errorf("remote request path/auth = %s, %t", request.URL.Path, request.Header.Get("Authorization") != "")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var envelope struct {
		ID     jsontext.Value `json:"id"`
		Method string         `json:"method"`
		Params jsontext.Value `json:"params"`
	}
	if err := json.UnmarshalRead(request.Body, &envelope); err != nil {
		s.t.Error(err)
		return
	}
	var result any
	var err error
	switch envelope.Method {
	case "runtime.discover":
		result, err = s.target.Discover(request.Context(), protocol.CallOptions{RequestMeta: requestMeta("test")})
	case "sessions.delete":
		var lost bool
		if lost, err = s.deleteSession(w, request, envelope.Params); lost {
			return
		}
		result = struct{}{}
	default:
		s.t.Errorf("unexpected remote operation %s", envelope.Method)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err != nil {
		s.t.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.writeResult(w, envelope.ID, result)
}

// deleteSession reports whether it deliberately lost the reply.
func (s *remoteDeletionServer) deleteSession(w http.ResponseWriter, request *http.Request, params jsontext.Value) (bool, error) {
	var parameters protocol.DeleteSessionRequest
	if err := json.Unmarshal(params, &parameters); err != nil {
		return false, err
	}
	key := request.Header.Get("Idempotency-Key")
	err := s.target.DeleteSession(request.Context(), parameters, protocol.CommandOptions{
		RequestMeta: requestMeta("test"), IdempotencyKey: key,
		IdempotencyNamespace: request.Header.Get("Idempotency-Namespace"),
	})
	s.mu.Lock()
	s.keys = append(s.keys, key)
	attempt := len(s.keys)
	s.mu.Unlock()
	if err != nil || attempt != 1 {
		return false, err
	}
	if s.malformed {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
		return true, nil
	}
	transport, _, hijackErr := w.(http.Hijacker).Hijack()
	if hijackErr != nil {
		s.t.Error(hijackErr)
		return true, nil
	}
	_ = transport.Close()
	return true, nil
}

func (s *remoteDeletionServer) writeResult(w http.ResponseWriter, id jsontext.Value, result any) {
	encoded, err := json.Marshal(struct {
		JSONRPC string         `json:"jsonrpc"`
		ID      jsontext.Value `json:"id"`
		Result  any            `json:"result"`
	}{JSONRPC: "2.0", ID: id, Result: result})
	if err != nil {
		s.t.Error(err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}
