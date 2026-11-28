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

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	adapterhooks "github.com/Tangerg/flame/runtime/internal/adapter/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/frame"
	framehost "github.com/Tangerg/frame/host"
)

func TestPluginShutdownReleasesBackendWithReservedUse(t *testing.T) {
	url := poMCPRemote(t, "record")
	source := poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":` + testMCPServerJSON(t, url) + `}}`,
	})
	authorPluginPackage(t, source)
	cfg := poMCPConfig(t)
	var framework framehost.Host
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), func(ctx context.Context, deps toolEnvironmentDependencies) (toolEnvironment, error) {
		framework = deps.framework.host
		return buildToolEnvironment(ctx, deps)
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &poMCPRuntime{t: t, host: host}
	t.Cleanup(runtime.close)
	api, err := pluginProtocolHandler(host, cfg)
	if err != nil {
		t.Fatal(err)
	}
	startTestRuntime(t, host, api, cfg.stores)
	runtime.endpoint = host.Endpoint()
	installed := poMCPActivate(runtime, source)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	runtime.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })

	instances := framework.Observations()
	if len(instances) != 1 || instances[0].Source.InstallationID.String() != installed.ID || instances[0].State != framehost.Ready {
		t.Fatalf("expected the connected backend instance, got %+v", instances)
	}
	scope, err := cfg.stores.Admission.OpenScope(t.Context(), "flame.mcp.catalog")
	if err != nil {
		t.Fatal(err)
	}
	use, err := framework.AcquireUnit(t.Context(), instances[0].Source, framehost.UseConfig{Owner: frame.NewLifetime(t.Context()), Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleWaitBudget(t))
	defer cancel()
	defer func() {
		if _, err := use.Close(ctx); err != nil {
			t.Errorf("release the reserved backend use: %v", err)
		}
	}()
	if current := framework.Observations(); len(current) != 1 || current[0].ID != instances[0].ID {
		t.Fatalf("reserved use did not retain the connected backend: %+v", current)
	}
	runtime.closed = true
	if err := host.Close(); err != nil {
		t.Fatalf("close Runtime: %v", err)
	}
	if err := framework.WaitInstanceRetired(ctx, instances[0].ID); err != nil {
		t.Fatalf("retire original plugin backend: %v", err)
	}
}

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
	firstPID := waitPluginPID(t, marker, 0, lifecycleWaitBudget(t))
	first.close()
	reopenPluginStores(t, &cfg)
	assertPluginExited(t, firstPID, time.Now().Add(lifecycleWaitBudget(t)))
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
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.stores.Idempotency, IdempotencyNamespace: cfg.stores.IdempotencyNamespace.String(), ValidateStorage: cfg.stores.ValidateDirectory})
	if err != nil {
		t.Fatal(err)
	}
	host.lifetime.delivery = endpoint
	restarted := &poMCPRuntime{t: t, host: host, api: api, endpoint: endpoint}
	defer restarted.close()
	restartedPID := waitPluginPID(t, marker, firstPID, lifecycleWaitBudget(t))
	restarted.must(delivery.PluginsList, struct{}{}, "")
	restarted.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	assertPluginExited(t, restartedPID, time.Now().Add(lifecycleWaitBudget(t)))
}

func TestInstalledHookCancellationStopsItsCommandGroup(t *testing.T) {
	cfg := poMCPConfig(t)
	writeHookSource(t, cfg.userHome, `{"hooks":[{"event":"Stop","command":"sh -c 'echo $$ > hook-started; exec sleep 30' & wait"}]}`)
	r := poMCPOpen(t, cfg)
	activateHookProvider(t, r.api)
	resolver, err := newHookResolver(cfg.userHome, cfg.stores.Trust, r.host.application.delivery.PluginCalls.(adapterhooks.Source))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := resolver.For(t.Context(), cfg.defaultWorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bound.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := bound.Run(ctx, hook.Input{Event: hook.Stop, SessionID: "session:hooks", CWD: cfg.defaultWorkspacePath})
		finished <- err
	}()
	const wait = 3 * time.Second
	pid := waitPluginPID(t, filepath.Join(cfg.defaultWorkspacePath, "hook-started"), 0, wait)
	deadline := time.Now().Add(wait)
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("installed hook cancellation lost its cause: %v", err)
		}
	case <-time.After(time.Until(deadline)):
		t.Fatal("installed command did not observe cancellation")
	}
	assertPluginExited(t, pid, deadline)
}

func waitPluginPID(t *testing.T, path string, previous int, wait time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(wait)
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

func assertPluginExited(t *testing.T, pid int, deadline time.Time) {
	t.Helper()
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
