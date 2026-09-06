package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRuntimePreservesCallerCancellation(t *testing.T) {
	for _, name := range []string{
		"FLAME_PROVIDER", "FLAME_MODEL", "FLAME_APIKEY", "FLAME_BASEURL", "ANTHROPIC_API_KEY",
		"FLAME_MCP_SERVERS", "FLAME_A2A_AGENTS", "FLAME_A2A_RPC_ORIGINS",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("FLAME_PROVIDER", "anthropic")
	runtime, err := Open(t.Context(), Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: t.TempDir(),
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("close Runtime: %v", err)
		}
	})

	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			if cause == context.DeadlineExceeded {
				cancel()
				ctx, cancel = context.WithDeadline(t.Context(), time.Time{})
			}
			cancel()
			page, err := runtime.ListProviders(ctx, CallOptions{})
			if page != nil || !errors.Is(err, cause) {
				t.Fatalf("ListProviders = (%+v, %v), want caller %v", page, err, cause)
			}
			if problem, ok := errors.AsType[protocol.ProblemError](err); !ok ||
				problem.Problem().Type != protocol.ProblemInternalError {
				t.Fatalf("canceled call lost its protocol problem: %v", err)
			}
		})
	}
	if _, err := runtime.ListProviders(t.Context(), CallOptions{}); err != nil {
		t.Fatalf("query after caller cancellation: %v", err)
	}
}

func TestRuntimeRestartDoesNotUndoAStoredProviderClear(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "")
	t.Setenv("FLAME_APIKEY", "")
	t.Setenv("FLAME_BASEURL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("FLAME_MCP_SERVERS", "")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")

	configDirectory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(configDirectory, "config.yaml"),
		[]byte("provider: anthropic\napiKey: sk-file\nbaseURL: https://config.example.test\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	config := Config{
		DataDirectory:        t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		UserHomePath:         t.TempDir(),
		ConfigDirectories:    []string{configDirectory},
	}

	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.UpdateProvider(t.Context(), protocol.UpdateProviderRequest{
		Provider: "anthropic",
		APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigClear},
	}, CommandOptions{IdempotencyKey: "clear-provider-before-restart"}); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	providers, err := reopened.ListProviders(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range providers.Data {
		if candidate.ID != "anthropic" {
			continue
		}
		if candidate.Credential != nil || candidate.Configured {
			t.Fatalf("restart re-enabled cleared provider = %+v", candidate)
		}
		if candidate.BaseURL == nil || *candidate.BaseURL != "https://config.example.test" {
			t.Fatalf("restart changed stored provider endpoint = %+v", candidate)
		}
		return
	}
	t.Fatal("anthropic provider missing after restart")
}

func TestRuntimeCanConfigureRequiredKeyProviderAfterStartup(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "")
	t.Setenv("FLAME_MODEL", "")
	t.Setenv("FLAME_APIKEY", "")
	t.Setenv("FLAME_BASEURL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("FLAME_MCP_SERVERS", "")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")

	configDirectory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(configDirectory, "config.yaml"),
		[]byte("provider: anthropic\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	config := Config{
		DataDirectory:        t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		UserHomePath:         t.TempDir(),
		ConfigDirectories:    []string{configDirectory},
	}

	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := runtime.ListProviders(t.Context(), CallOptions{})
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	configured := providerByID(t, providers.Data, "anthropic")
	if configured.Configured || configured.Credential != nil {
		_ = runtime.Close()
		t.Fatalf("provider started configured without a credential = %+v", configured)
	}

	key := "sk-stored-after-startup"
	configured, err = runtime.UpdateProvider(t.Context(), protocol.UpdateProviderRequest{
		Provider: "anthropic",
		APIKey: &protocol.ProviderConfigChange{
			Type:  protocol.ProviderConfigSet,
			Value: &key,
		},
	}, CommandOptions{IdempotencyKey: "configure-provider-after-startup"})
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if !configured.Configured || configured.Credential == nil || configured.Credential.Source != protocol.ProviderKeySourceStored {
		_ = runtime.Close()
		t.Fatalf("provider update did not establish stored configuration = %+v", configured)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	providers, err = reopened.ListProviders(t.Context(), CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	configured = providerByID(t, providers.Data, "anthropic")
	if !configured.Configured || configured.Credential == nil || configured.Credential.Source != protocol.ProviderKeySourceStored {
		t.Fatalf("stored provider configuration did not survive restart = %+v", configured)
	}
}

func providerByID(t *testing.T, providers []protocol.Provider, id string) *protocol.Provider {
	t.Helper()
	for index := range providers {
		if providers[index].ID == id {
			return &providers[index]
		}
	}
	t.Fatalf("provider %q is missing", id)
	return nil
}

func TestResolveConfigUsesExplicitStableDefaults(t *testing.T) {
	data := t.TempDir()
	home := t.TempDir()
	resolved, err := (Config{DataDirectory: data, UserHomePath: home}).resolve()
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if resolved.DefaultWorkspacePath != home {
		t.Fatalf("default workspace = %q, want user home %q", resolved.DefaultWorkspacePath, home)
	}
	if len(resolved.ConfigDirectories) != 1 || resolved.ConfigDirectories[0] != data {
		t.Fatalf("config directories = %v, want [%s]", resolved.ConfigDirectories, data)
	}

	for _, test := range []struct {
		name   string
		config Config
	}{
		{name: "missing data directory", config: Config{UserHomePath: home}},
		{name: "relative data directory", config: Config{DataDirectory: "data", UserHomePath: home}},
		{name: "relative home", config: Config{DataDirectory: data, UserHomePath: "home"}},
		{name: "relative workspace", config: Config{DataDirectory: data, UserHomePath: home, DefaultWorkspacePath: "workspace"}},
		{name: "relative config directory", config: Config{DataDirectory: data, UserHomePath: home, ConfigDirectories: []string{"config"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.config.resolve(); err == nil {
				t.Fatal("resolveConfig accepted an ambiguous host path")
			}
		})
	}
}

func TestRuntimeOpenCallIdempotencyStreamAndClose(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("FLAME_MCP_SERVERS", "")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")

	config := Config{
		DataDirectory:        t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		UserHomePath:         t.TempDir(),
		ConfigDirectories:    []string{t.TempDir()},
	}
	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	discovery, err := runtime.Discover(t.Context(), CallOptions{})
	if err != nil || discovery.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("Discover = (%+v, %v)", discovery, err)
	}
	if discovery.ServerInfo.Name != runtimeidentity.ProductName {
		t.Fatalf("Discover server brand = %q, want %q", discovery.ServerInfo.Name, runtimeidentity.ProductName)
	}
	if _, discoverErr := runtime.Discover(t.Context(), CallOptions{RequestMeta: protocol.RequestMeta{
		ProtocolVersion: "1900-01-01",
	}}); !errors.Is(discoverErr, protocol.ErrInvalidProtocolVersion) {
		t.Fatalf("unsupported protocol error = %v", discoverErr)
	} else {
		var problem protocol.ProblemError
		if !errors.As(discoverErr, &problem) || problem.Problem().Type != protocol.ErrInvalidProtocolVersion.Error() {
			t.Fatalf("structured unsupported protocol error = %T %v", discoverErr, discoverErr)
		}
	}

	create := protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: config.DefaultWorkspacePath},
		Title:     "in-process",
	}
	first, err := runtime.CreateSession(t.Context(), create, CommandOptions{IdempotencyKey: "create-once"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	replayed, err := runtime.CreateSession(t.Context(), create, CommandOptions{IdempotencyKey: "create-once"})
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("replayed CreateSession = (%+v, %v), want %s", replayed, err, first.ID)
	}
	create.Title = "different"
	if _, createSessionErr := runtime.CreateSession(t.Context(), create, CommandOptions{IdempotencyKey: "create-once"}); !errors.Is(createSessionErr, protocol.ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error = %v", createSessionErr)
	}

	second, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("second Open sharing data directory: %v", err)
	}

	_, events, err := second.SubscribeRuntime(t.Context(), protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged},
	}, SubscriptionOptions{})
	if err != nil {
		t.Fatalf("SubscribeRuntime: %v", err)
	}
	streamDone := make(chan struct{})
	eventReceived := make(chan protocol.RuntimeEvent, 1)
	go func() {
		defer close(streamDone)
		for event, err := range events {
			if err != nil {
				return
			}
			eventReceived <- event
			return
		}
	}()
	if _, createSessionErr := runtime.CreateSession(t.Context(), protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: config.DefaultWorkspacePath},
		Title:     "notifies",
	}, CommandOptions{IdempotencyKey: "create-notification"}); createSessionErr != nil {
		t.Fatalf("CreateSession for notification: %v", createSessionErr)
	}
	select {
	case event := <-eventReceived:
		if event.Type != protocol.RuntimeResync ||
			!slices.Equal(event.Topics, []protocol.RuntimeTopic{protocol.TopicSessionsChanged}) {
			t.Fatalf("cross-Runtime event = %+v, want scoped resync", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("peer Runtime commit produced no scoped resync")
	}
	if closeErr := second.Close(); closeErr != nil {
		t.Fatalf("close second Runtime: %v", closeErr)
	}
	<-streamDone

	if closeErr := runtime.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	if _, discoverErr := runtime.Discover(t.Context(), CallOptions{}); !errors.Is(discoverErr, ErrClosed) {
		t.Fatalf("Discover after Close error = %v, want ErrClosed", discoverErr)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened Runtime: %v", err)
	}
}

func TestResolveConfigCleansPaths(t *testing.T) {
	root := t.TempDir()
	resolved, err := (Config{
		DataDirectory:        filepath.Join(root, "data", "..", "data"),
		DefaultWorkspacePath: filepath.Join(root, "workspace", "."),
		UserHomePath:         filepath.Join(root, "home", "."),
	}).resolve()
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if resolved.DataDirectory != filepath.Join(root, "data") ||
		resolved.DefaultWorkspacePath != filepath.Join(root, "workspace") ||
		resolved.UserHomePath != filepath.Join(root, "home") {
		t.Fatalf("resolved paths = %+v", resolved)
	}
}
