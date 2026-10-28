//go:build darwin || linux

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestBlockedStdioPluginReleasesProcessAfterRestartAndDisable(t *testing.T) {
	cfg := poMCPConfig(t)
	marker := filepath.Join(t.TempDir(), "pid")
	source := poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    fmt.Sprintf(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":{"type":"stdio","command":%q,"args":["blocked",%q]}}}`, fixturePluginCommand, marker),
	})
	authorPluginPackage(t, source)
	first := poMCPOpen(t, cfg)
	installed := poMCPActivate(first, source)
	if len(installed.Selected.Servers) != 1 {
		t.Fatalf("stdio fixture was not admitted: %+v", installed.Selected)
	}
	firstPID := waitPluginPID(t, marker, 0)
	first.close()
	reopenPluginStores(t, &cfg)
	assertPluginExited(t, firstPID)
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleWaitBudget(t))
	defer cancel()
	host, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("blocked stdio prevented construction: %v", err)
	}
	defer func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	api, err := pluginProtocolHandler(host, cfg)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.stores.Idempotency, IdempotencyNamespace: cfg.stores.IdempotencyNamespace.String()})
	if err != nil {
		t.Fatal(err)
	}
	host.lifetime.delivery = endpoint
	restarted := &poMCPRuntime{t: t, host: host, api: api, endpoint: endpoint}
	defer restarted.close()
	restartedPID := waitPluginPID(t, marker, firstPID)
	restarted.must(delivery.PluginsList, struct{}{}, "")
	restarted.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	assertPluginExited(t, restartedPID)
}

func waitPluginPID(t *testing.T, path string, previous int) int {
	t.Helper()
	deadline := time.Now().Add(lifecycleWaitBudget(t))
	for {
		body, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
		if err == nil && pid > 0 && pid != previous {
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatal("plugin process did not start")
		}
		time.Sleep(time.Millisecond)
	}
}

func assertPluginExited(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(lifecycleWaitBudget(t))
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("plugin process %d survived retirement", pid)
		}
		time.Sleep(time.Millisecond)
	}
}
