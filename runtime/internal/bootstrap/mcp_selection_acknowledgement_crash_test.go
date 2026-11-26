package bootstrap

import (
	"database/sql/driver"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	modernsqlite "modernc.org/sqlite"
)

const (
	selectionAckHome     = "FLAME_TEST_SELECTION_ACK_HOME"
	selectionAckRunner   = "FLAME_TEST_SELECTION_ACK_RUNNER"
	selectionAckKey      = "choose-original-command"
	selectionAckPayload  = "selection-acknowledgement.json"
	selectionAckPrevious = "previous-connector.id"
)

func TestMCPConnectorSelectionAcknowledgementChild(t *testing.T) {
	home := os.Getenv(selectionAckHome)
	if home == "" {
		return
	}
	if err := modernsqlite.RegisterScalarFunction("test_connector_selection_ack_crash", 1,
		func(_ *modernsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			body, ok := args[0].([]byte)
			if !ok {
				return nil, fmt.Errorf("expected a serialized acknowledgement, got %T", args[0])
			}
			if err := os.WriteFile(filepath.Join(home, selectionAckPayload), body, 0600); err != nil {
				return nil, err
			}
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}

	r := poMCPOpen(t, externalMCPRestartConfig(t, home, os.Getenv(selectionAckRunner)))
	original := installExternalConnector(r)
	if err := os.WriteFile(filepath.Join(home, selectionAckPrevious), []byte(original.ID), 0600); err != nil {
		t.Fatal(err)
	}
	replacement := enableReplacementConnector(r)

	questionCrashDatabase(t, home, `CREATE TRIGGER crash_before_connector_selection_acknowledgement
		BEFORE UPDATE OF payload ON idempotency_records
		WHEN OLD.key = '`+selectionAckKey+`' AND NEW.payload IS NOT NULL
		BEGIN SELECT test_connector_selection_ack_crash(NEW.payload); END`)
	request := protocol.SelectPluginProviderRequest{Role: mcpconnector.Reference, InstallationID: replacement.ID, Contribution: mcpconnector.Export}
	result := r.endpoint.Invoke(t.Context(), delivery.PluginsSelectProvider, request, protocol.CallMetadata{
		IdempotencyKey: selectionAckKey, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	t.Fatalf("selection did not stop before acknowledgement: %v", result.Failure)
}

func TestMCPConnectorSelectionAndAcknowledgementCommitTogether(t *testing.T) {
	home, runnerDir := testsupport.TempDirectory(t), testsupport.TempDirectory(t)
	if _, err := externalMCPRunner(t, runnerDir).Identity(t.Context()); err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMCPConnectorSelectionAcknowledgementChild$", "-test.timeout=35s")
	child.Env = append(os.Environ(), "TMPDIR="+testsupport.TempDirectory(t), selectionAckHome+"="+home, selectionAckRunner+"="+runnerDir)
	child.WaitDelay = 3 * time.Second
	output, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("selection child did not exit at the receipt: %v\n%s", err, output)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER crash_before_connector_selection_acknowledgement")
	payload, err := os.ReadFile(filepath.Join(home, selectionAckPayload))
	if err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(filepath.Join(home, selectionAckPrevious))
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Value protocol.PluginProviderSelection `json:"value"`
	}
	if err := json.Unmarshal(payload, &captured, json.RejectUnknownMembers(true)); err != nil || captured.Value.Role != mcpconnector.Reference {
		t.Fatalf("unexpected candidate receipt: %+v, %v", captured.Value, err)
	}
	r := poMCPOpen(t, externalMCPRestartConfig(t, home, runnerDir))
	selections := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	if len(selections.Data) != 1 || selections.Data[0].InstallationID != string(previous) {
		t.Fatalf("a choice escaped its acknowledgement transaction: %+v; previous=%s", selections.Data, previous)
	}
	request := protocol.SelectPluginProviderRequest{
		Role: captured.Value.Role, InstallationID: captured.Value.InstallationID, Contribution: captured.Value.Contribution,
	}
	replayed := r.must(delivery.PluginsSelectProvider, request, selectionAckKey).(*protocol.PluginProviderSelection)
	if *replayed != captured.Value {
		t.Fatalf("replayed selection = %+v, want %+v", replayed, captured.Value)
	}
}

func TestMCPConnectorKeepsItsConnectionsUntilAChoiceCommits(t *testing.T) {
	remote := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	original := installExternalConnector(r)
	replacement := enableReplacementConnector(r)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: "selection-receipt-server", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-selection-receipt-server")
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" }
	r.poMCPTools(&server, ready)
	requireVisible := func() {
		t.Helper()
		tools := r.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &server}, "").(*protocol.Page[protocol.MCPTool])
		if !ready(tools.Data) {
			t.Fatalf("original connector tools = %+v", tools.Data)
		}
	}

	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	request := protocol.SelectPluginProviderRequest{Role: mcpconnector.Reference, InstallationID: replacement.ID, Contribution: mcpconnector.Export}
	failed := r.endpoint.Invoke(t.Context(), delivery.PluginsSelectProvider, request, protocol.CallMetadata{
		IdempotencyKey: "refused-connector-selection", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	allowReceipt()
	if failed.Failure == nil {
		t.Fatal("connector selection succeeded without its receipt")
	}
	selections := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	if len(selections.Data) != 1 || selections.Data[0].InstallationID != original.ID {
		t.Fatalf("refused receipt changed the connector owner: %+v", selections.Data)
	}
	requireVisible()

	questionCrashDatabase(t, cfg.stores.DataDirectory, `CREATE TRIGGER refuse_duplicate_connector_selection
		BEFORE UPDATE OF installation_id ON plugin_provider_selections
		WHEN NEW.installation_id=OLD.installation_id AND NEW.contribution_id=OLD.contribution_id
		BEGIN SELECT RAISE(ABORT, 'the choice was already recorded'); END`)
	request.InstallationID = original.ID
	r.must(delivery.PluginsSelectProvider, request, "confirm-original-connector-selection")
	requireVisible()
}

func enableReplacementConnector(r *poMCPRuntime) *protocol.PluginInstallation {
	r.t.Helper()
	replacement := r.must(delivery.PluginsInstall,
		protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "mcp")}, "install-replacement").(*protocol.PluginInstallation)
	return r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{
		InstallationID: replacement.ID, Enabled: true,
	}, "enable-replacement").(*protocol.PluginInstallation)
}
