package bootstrap

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestEnabledPluginCannotBlockRuntimeManagementAfterRestart(t *testing.T) {
	for _, blockedMethod := range []string{"server/discover", "tools/list"} {
		t.Run(blockedMethod, func(t *testing.T) {
			entered := make(chan struct{}, 4)
			settled := make(chan struct{}, 4)
			backend := sdk.NewServer(&sdk.Implementation{Name: "blocked-plugin"}, nil)
			backend.AddTool(&sdk.Tool{Name: "record", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				return &sdk.CallToolResult{}, nil
			})
			handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return backend }, nil)
			transport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var envelope struct {
					Method string `json:"method"`
				}
				_ = json.Unmarshal(body, &envelope)
				if envelope.Method == blockedMethod {
					entered <- struct{}{}
					<-r.Context().Done()
					settled <- struct{}{}
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				handler.ServeHTTP(w, r)
			}))
			defer transport.Close()
			healthyURL := poMCPRemote(t, "ping")
			cfg := poMCPConfig(t)
			first := poMCPOpen(t, cfg)
			source := poMCPPackage(t, map[string]string{
				"plugin.json": poMCPManifest,
				"mcp.json":    fmt.Sprintf(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":{"type":"streamable-http","url":%q},"healthy":{"type":"streamable-http","url":%q}}}`, transport.URL, healthyURL),
			})
			installed := poMCPActivate(first, source)
			if len(installed.Selected.Servers) != 2 {
				t.Fatalf("fixture backend was not admitted: %+v", installed.Selected)
			}
			waitStartupSignal(t, entered, "initial blocked connection")
			first.close()
			waitStartupSignal(t, settled, "initial connection retirement")

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			host, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
			if err != nil {
				t.Fatalf("enabled optional plugin blocked Runtime construction: %v", err)
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
			restarted := &poMCPRuntime{t: t, host: host, api: api, endpoint: endpoint}
			defer restarted.close()
			waitStartupSignal(t, entered, "restarted blocked connection")
			page := restarted.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
			if len(page.Data) != 1 || page.Data[0].ID != installed.ID {
				t.Fatalf("plugin management became unavailable: %+v", page.Data)
			}
			restarted.poMCPTools(&protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "healthy"}, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
			cancel()
			restarted.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
			waitStartupSignal(t, settled, "disabled connection retirement")
			tools := restarted.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{}, "").(*protocol.Page[protocol.MCPTool])
			if len(tools.Data) != 0 {
				t.Fatal("blocked source published executable tools")
			}
		})
	}
}

func waitStartupSignal(t *testing.T, signal <-chan struct{}, phase string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatalf("%s did not settle", phase)
	}
}
