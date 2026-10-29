package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/bootstrap"
	"github.com/Tangerg/flame/runtime/internal/config"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
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

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	instance, _, err := bootstrap.OpenInstance(t.Context(), bootstrap.InstanceConfig{
		FirstPartyDirectory:  filepath.Join(filepath.Dir(executable), "plugins"),
		RunnerExecutable:     testsupport.RunnerExecutable(t),
		UserHome:             testsupport.TempDirectory(t),
		DefaultWorkspacePath: testsupport.TempDirectory(t),
		DataDirectory:        testsupport.TempDirectory(t),
		ConfigDirectories:    []string{testsupport.TempDirectory(t)},
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
