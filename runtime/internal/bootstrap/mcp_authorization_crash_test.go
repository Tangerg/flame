package bootstrap

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/frame/process/runner"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

const externalAuthorizationCrashHome = "FLAME_TEST_MCP_AUTH_CRASH_HOME"
const externalAuthorizationCrashRunner = "FLAME_TEST_MCP_AUTH_CRASH_RUNNER"
const externalAuthorizationCrashEndpoint = "FLAME_TEST_MCP_AUTH_CRASH_ENDPOINT"

func externalAuthorizationConfig(t *testing.T, data, runnerDir string) assemblyConfig {
	t.Helper()
	cfg := poMCPConfig(t)
	store, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: data})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg.processRunner = externalAuthorizationRunner(t, runnerDir)
	cfg.stores = store
	cfg.providerRegistry = store.Providers
	return cfg
}

func externalAuthorizationRunner(t *testing.T, runnerDir string) *runner.Daemon {
	t.Helper()
	daemon, err := runner.NewDaemon(runner.DaemonConfig{Executable: testsupport.RunnerExecutable(t), Directory: runnerDir, StartTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := daemon.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return daemon
}

func TestExternalAuthorizationCrashChild(t *testing.T) {
	data := os.Getenv(externalAuthorizationCrashHome)
	if data == "" {
		return
	}
	cfg := externalAuthorizationConfig(t, data, os.Getenv(externalAuthorizationCrashRunner))
	r := poMCPOpen(t, cfg)
	installExternalConnector(r)
	server := r.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
		Name: "interrupted", Enabled: true, Connection: protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: os.Getenv(externalAuthorizationCrashEndpoint)},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-server").(*protocol.MCPServer)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		row, err := r.api.ListMCPServers(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		ready := false
		for _, item := range row.Data {
			if item.ID == server.ID {
				ready = item.Status.Type == protocol.MCPServerFailed
			}
		}
		if ready {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	attempt, err := r.api.CreateMCPAuthorizationAttempt(t.Context(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Status.Type != protocol.MCPAuthorizationAttemptPending {
		t.Fatalf("unexpected starting state: %s", attempt.Status.Type)
	}
	observed := false
	for limit := time.Now().Add(10 * time.Second); time.Now().Before(limit); {
		res, err := http.Get(os.Getenv(externalAuthorizationCrashEndpoint) + "/observed")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			observed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !observed {
		t.Fatal("external authorization did not reach its transport")
	}
	if err := os.WriteFile(filepath.Join(data, "attempt-id"), []byte(attempt.ID), 0600); err != nil {
		t.Fatal(err)
	}
	os.Exit(commandCrashExit)
}

func TestCrashDuringExternalAuthorizationRestoresUnconfirmedReceipt(t *testing.T) {
	data := testsupport.TempDirectory(t)
	runnerDir, err := os.MkdirTemp("", "mcp-crash-runner-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runnerDir) })
	if _, err := externalAuthorizationRunner(t, runnerDir).Identity(t.Context()); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	release := make(chan struct{})
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/observed" {
			if requests.Load() < 2 {
				w.WriteHeader(http.StatusAccepted)
			}
			return
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() { close(release); remote.Close() }()
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExternalAuthorizationCrashChild$", "-test.timeout=30s")
	child.Env = append(os.Environ(), externalAuthorizationCrashHome+"="+data, externalAuthorizationCrashEndpoint+"="+remote.URL, externalAuthorizationCrashRunner+"="+runnerDir)
	child.WaitDelay = 3 * time.Second
	log, err := child.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != commandCrashExit {
		t.Fatalf("child did not stop after durable admission: %v: %s", err, log)
	}
	id, err := os.ReadFile(filepath.Join(data, "attempt-id"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := externalAuthorizationConfig(t, data, runnerDir)
	restored := poMCPOpen(t, cfg)
	attempt, err := restored.api.GetMCPAuthorizationAttempt(t.Context(), string(id))
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Status.Type != protocol.MCPAuthorizationAttemptUnconfirmed || attempt.FinishedAt == nil {
		t.Fatalf("restored authorization = %+v", attempt)
	}
}
