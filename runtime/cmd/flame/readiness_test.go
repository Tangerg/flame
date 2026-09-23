package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/bootstrap"
	"github.com/Tangerg/flame/runtime/internal/config"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestStorageHealthProbeSeparatesBusyFromBroken(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want flamehttp.HealthStatus
	}{
		{name: "answers", want: flamehttp.HealthOK},
		{name: "busy", err: context.DeadlineExceeded, want: flamehttp.HealthDegraded},
		{name: "abandoned", err: context.Canceled, want: flamehttp.HealthDegraded},
		{name: "broken", err: errors.New("database is locked"), want: flamehttp.HealthUnhealthy},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := storageHealthProbe(func(context.Context) error { return test.err })
			check := probe.Probe(t.Context())
			if probe.Name != "storage" || check.Status != test.want {
				t.Fatalf("probe %q = %+v, want %q", probe.Name, check, test.want)
			}
			if test.want != flamehttp.HealthOK && check.Detail == "" {
				t.Fatal("a probe that is not ok reported no detail for the operator log")
			}
		})
	}
}

// TestServedReadinessReportsTheStorageCheck pins the wiring, not the mapping:
// the readiness contract carries a per-check map that clients render, and a
// server assembled without probes answers "ok" with nothing in it forever.
func TestServedReadinessReportsTheStorageCheck(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("FLAME_MCP_SERVERS", "")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")

	instance, _, err := bootstrap.OpenInstance(t.Context(), bootstrap.InstanceConfig{
		UserHome:             t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		DataDirectory:        t.TempDir(),
		ConfigDirectories:    []string{t.TempDir()},
		BuildID:              "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		ServerInfo:           protocol.ServerInfo{Name: "test-runtime", Version: "test-version"},
	})
	if err != nil {
		t.Fatalf("OpenInstance: %v", err)
	}
	t.Cleanup(func() { _ = instance.Close() })

	server, err := buildHTTPServer(instance, config.Server{Listen: "127.0.0.1:0"}, "test-token")
	if err != nil {
		t.Fatalf("buildHTTPServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/v2/health/ready", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("readiness status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var readiness flamehttp.ReadinessStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &readiness); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	if readiness.Status != flamehttp.HealthOK {
		t.Fatalf("readiness = %q, want %q", readiness.Status, flamehttp.HealthOK)
	}
	if readiness.Checks["storage"] != flamehttp.HealthOK {
		t.Fatalf("readiness checks = %v, want a storage check reporting ok", readiness.Checks)
	}
}
