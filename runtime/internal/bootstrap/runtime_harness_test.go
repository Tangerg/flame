package bootstrap

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type noMaintenance struct{}

func (noMaintenance) Maintain(
	context.Context,
	executionadapter.RunMaintenanceInput,
) executionadapter.RunMaintenanceResult {
	return executionadapter.RunMaintenanceResult{}
}

func openProtocolRuntime(t *testing.T, model chat.Model) (*Instance, *delivery.Handler) {
	t.Helper()
	dataDirectory := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: dataDirectory, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	cfg := protocolRuntimeConfig(t, stores, model)
	return buildProtocolRuntime(t, cfg, stores.DataDirectory)
}

func protocolRuntimeConfig(t *testing.T, stores *persistence.Bundle, model chat.Model) assemblyConfig {
	t.Helper()
	client, err := chatclient.New(model)
	if err != nil {
		_ = stores.Close()
		t.Fatalf("build chat client: %v", err)
	}
	cfg := composeConfig(
		config.Settings{DefaultSelection: testsupport.MustModelSelection("anthropic", "claude-test")},
		stores,
		stores.Providers,
		"sha256:0000000000000000000000000000000000000000000000000000000000000000",
	)
	cfg.modelProviders = testChatResolver(client)
	cfg.userHome = stores.DataDirectory
	cfg.defaultWorkspacePath = stores.DataDirectory
	cfg.maintenance = noMaintenance{}
	cfg.processRunner = testProcessRunner(t)
	return cfg
}

func buildProtocolRuntime(t *testing.T, cfg assemblyConfig, cwd string) (*Instance, *delivery.Handler) {
	t.Helper()
	host := assembleTestRuntime(t, cfg)
	api, err := protocolHandler(host, cwd)
	if err != nil {
		_ = host.Close()
		t.Fatalf("build protocol server: %v", err)
	}
	startTestRuntime(t, host, api, cfg.stores)
	return host, api
}

func startTestRuntime(t *testing.T, host *Instance, api *delivery.Handler, stores *persistence.Bundle) {
	t.Helper()
	if err := startTestRuntimeWithReplayStore(t, host, api, stores, stores.Idempotency); err != nil {
		t.Fatalf("recover uncertain executions: %v; close: %v", err, host.Close())
	}
}

func startTestRuntimeWithReplayStore(t *testing.T, host *Instance, api *delivery.Handler, stores *persistence.Bundle, replayStore operation.ReplayStore) error {
	t.Helper()
	info, err := api.Discover(t.Context())
	if err != nil {
		t.Fatalf("discover Runtime: %v; close: %v", err, host.Close())
	}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		IdempotencyStore:     replayStore,
		IdempotencyNamespace: info.Capabilities.Limits.Idempotency.Namespace,
		Lifetime:             t.Context(),
		ValidateStorage:      stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatalf("construct Runtime endpoint: %v; close: %v", err, host.Close())
	}
	host.lifetime.delivery = endpoint
	if err := host.application.publishCallbacks(endpoint); err != nil {
		t.Fatalf("publish Runtime callbacks: %v; close: %v", err, host.Close())
	}
	return host.application.recoverUncertainExecutions(t.Context())
}

func assembleTestRuntime(t *testing.T, cfg assemblyConfig) *Instance {
	t.Helper()
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	if recoverStartupErr := host.application.recoverStartup(t.Context()); recoverStartupErr != nil {
		_ = host.Close()
		t.Fatalf("recover runtime: %v", recoverStartupErr)
	}
	return host
}

func protocolHandler(host *Instance, cwd string) (*delivery.Handler, error) {
	return host.application.newDeliveryHandler(protocol.ServerInfo{
		Name: "conformance-test", Version: "0.0.0-test", InstanceID: testsupport.RuntimeInstanceID,
		DefaultWorkspace: protocol.WorkspaceRef{Path: cwd}, Home: cwd,
	}, testsupport.IdempotencyNamespace)
}

func waitForSignal(t *testing.T, signal <-chan struct{}, phase string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatalf("timed out waiting for %s", phase)
	}
}
