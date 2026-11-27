package bootstrap

import (
	"context"
	"database/sql"
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
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/process"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	sdk "github.com/Tangerg/go-sdk/mcp"
	modernsqlite "modernc.org/sqlite"
)

const (
	authorizationAckCrashHome   = "FLAME_TEST_MCP_AUTH_ACK_HOME"
	authorizationAckCrashRunner = "FLAME_TEST_MCP_AUTH_ACK_RUNNER"
	authorizationAckCrashRemote = "FLAME_TEST_MCP_AUTH_ACK_REMOTE"
	authorizationAckCrashKey    = "start-original-authorization"
	authorizationAckFixture     = "original-authorization.json"
	authorizationAckServer      = "authorization-fixture"
)

func TestMCPAuthorizationAcknowledgementCrashChild(t *testing.T) {
	home := os.Getenv(authorizationAckCrashHome)
	if home == "" {
		return
	}
	if err := modernsqlite.RegisterScalarFunction("test_mcp_authorization_ack_crash", 1,
		func(_ *modernsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			body, ok := args[0].([]byte)
			if !ok {
				return nil, fmt.Errorf("expected a serialized acknowledgement, got %T", args[0])
			}
			if err := os.WriteFile(filepath.Join(home, authorizationAckFixture), body, 0600); err != nil {
				return nil, err
			}
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	r := poMCPOpen(t, externalMCPRestartConfig(t, home, os.Getenv(authorizationAckCrashRunner)))
	installExternalConnector(r)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: authorizationAckServer, Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: os.Getenv(authorizationAckCrashRemote)},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-authorization-fixture")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })
	questionCrashDatabase(t, home, `CREATE TRIGGER interrupt_mcp_authorization_acknowledgement
		BEFORE UPDATE OF payload ON idempotency_records
		WHEN OLD.key = '`+authorizationAckCrashKey+`' AND NEW.kind = 'response'
		BEGIN SELECT test_mcp_authorization_ack_crash(NEW.payload); END`)
	result := r.endpoint.Invoke(t.Context(), delivery.MCPAuthorizationAttemptsCreate, protocol.CreateMCPAuthorizationAttemptRequest{Server: server}, protocol.CallMetadata{IdempotencyKey: authorizationAckCrashKey})
	t.Fatalf("authorization did not stop before acknowledging: %v", result.Failure)
}

func TestMCPAuthorizationAcknowledgementRecoversTheOriginalAttempt(t *testing.T) {
	home, runnerDir := crashMCPAuthorization(t, "TestMCPAuthorizationAcknowledgementCrashChild")
	questionCrashDatabase(t, home, "DROP TRIGGER interrupt_mcp_authorization_acknowledgement")
	body, err := os.ReadFile(filepath.Join(home, authorizationAckFixture))
	if err != nil {
		t.Fatal(err)
	}
	var original struct {
		Value protocol.MCPAuthorizationAttemptRef `json:"value"`
	}
	if err := json.Unmarshal(body, &original, json.RejectUnknownMembers(true)); err != nil || original.Value.AttemptID == "" {
		t.Fatalf("original attempt receipt = %+v, %v", original.Value, err)
	}
	r := poMCPOpen(t, externalMCPRestartConfig(t, home, runnerDir))
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: authorizationAckServer}
	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	value := r.must(delivery.MCPAuthorizationAttemptsCreate, request, authorizationAckCrashKey).(*protocol.MCPAuthorizationAttemptRef)
	if *value != original.Value {
		t.Fatalf("recovered authorization acknowledged %q, want %q", value.AttemptID, original.Value.AttemptID)
	}
	attempt := awaitMCPAuthorization(t, r, original.Value.AttemptID)
	if attempt.ID != original.Value.AttemptID || attempt.Server != server ||
		attempt.Status.Type != protocol.MCPAuthorizationAttemptUnconfirmed {
		t.Fatalf("connector did not recover the interrupted attempt: %+v", attempt)
	}
}

func TestMCPAuthorizationRetryPreservesTheAttemptWhileItsStatusIsUnavailable(t *testing.T) {
	remote := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	connector := installExternalConnector(r)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: authorizationAckServer, Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-authorization-fixture")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })

	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	initial := r.endpoint.Invoke(t.Context(), delivery.MCPAuthorizationAttemptsCreate, request, protocol.CallMetadata{IdempotencyKey: authorizationAckCrashKey})
	if !errors.Is(initial.Failure, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("unacknowledged authorization = %+v", initial)
	}
	allowReceipt()

	store := authorizationCrashStore(t, filepath.Join(cfg.stores.DataDirectory, "plugins", "data", connector.ID, "external-servers.db"))
	var original string
	if err := store.QueryRowContext(t.Context(), `SELECT id FROM attempts WHERE name=?`, server.Name).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExecContext(t.Context(), `ALTER TABLE attempts RENAME COLUMN started TO unavailable_started`); err != nil {
		t.Fatal(err)
	}
	interrupted := r.endpoint.Invoke(t.Context(), delivery.MCPAuthorizationAttemptsCreate, request, protocol.CallMetadata{IdempotencyKey: authorizationAckCrashKey})
	if _, err := store.ExecContext(t.Context(), `ALTER TABLE attempts RENAME COLUMN unavailable_started TO started`); err != nil {
		t.Fatal(err)
	}
	if interrupted.Failure == nil || !errors.Is(interrupted.Failure, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("status read failure settled the original command: %+v", interrupted)
	}

	accepted := r.must(delivery.MCPAuthorizationAttemptsCreate, request, authorizationAckCrashKey).(*protocol.MCPAuthorizationAttemptRef)
	if accepted.AttemptID != original {
		t.Fatalf("retry acknowledged %q, want %q", accepted.AttemptID, original)
	}
	attempt := awaitMCPAuthorization(t, r, accepted.AttemptID)
	if attempt.Server != server || attempt.Status.Type != protocol.MCPAuthorizationAttemptSucceeded {
		t.Fatalf("recovered launch did not settle the accepted authorization: %+v", attempt)
	}
}

const unacceptedAuthorizationStore = "unaccepted-authorization-store"

func TestMCPAuthorizationUnacceptedStartCrashChild(t *testing.T) {
	home := os.Getenv(authorizationAckCrashHome)
	if home == "" {
		return
	}
	r := poMCPOpen(t, externalMCPRestartConfig(t, home, os.Getenv(authorizationAckCrashRunner)))
	connector := installExternalConnector(r)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: authorizationAckServer, Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: os.Getenv(authorizationAckCrashRemote)},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-authorization-fixture")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })
	path := filepath.Join(home, "plugins", "data", connector.ID, "external-servers.db")
	store := authorizationCrashStore(t, path)
	if _, err := store.ExecContext(t.Context(), `CREATE TRIGGER refuse_test_authorization_start
		BEFORE INSERT ON attempts
		BEGIN SELECT RAISE(ABORT, 'refuse authorization before recording its receipt'); END`); err != nil {
		t.Fatal(err)
	}
	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	result := r.endpoint.Invoke(t.Context(), delivery.MCPAuthorizationAttemptsCreate, request, protocol.CallMetadata{IdempotencyKey: authorizationAckCrashKey})
	if !errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("interrupted authorization = %+v", result)
	}
	if err := os.WriteFile(filepath.Join(home, unacceptedAuthorizationStore), []byte(path), 0600); err != nil {
		t.Fatal(err)
	}
	os.Exit(commandCrashExit)
}

func TestMCPAuthorizationSettlesAnUnacceptedStartAfterRestart(t *testing.T) {
	home, runnerDir := crashMCPAuthorization(t, "TestMCPAuthorizationUnacceptedStartCrashChild")
	path, err := os.ReadFile(filepath.Join(home, unacceptedAuthorizationStore))
	if err != nil {
		t.Fatal(err)
	}
	store := authorizationCrashStore(t, string(path))
	if _, err := store.ExecContext(t.Context(), "DROP TRIGGER refuse_test_authorization_start"); err != nil {
		t.Fatal(err)
	}
	assertAttempts := func(want int) {
		t.Helper()
		var count int
		err := store.QueryRowContext(t.Context(), "SELECT count(*) FROM attempts").Scan(&count)
		if err != nil || count != want {
			t.Fatalf("connector recorded %d attempts, want %d: %v", count, want, err)
		}
	}
	assertAttempts(0)
	cfg := externalMCPRestartConfig(t, home, runnerDir)
	starts, err := cfg.stores.MCPServers.PendingAuthorizationStarts(t.Context())
	if err != nil || len(starts) != 1 {
		t.Fatalf("interrupted start was not retained: %v, %v", starts, err)
	}
	r := poMCPOpen(t, cfg)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: authorizationAckServer}
	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	retry := func() {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.MCPAuthorizationAttemptsCreate, request, protocol.CallMetadata{IdempotencyKey: authorizationAckCrashKey})
		if !errors.Is(result.Failure, protocol.ErrPluginCommandRetired) {
			t.Fatalf("original authorization should have a terminal retirement receipt: %+v", result)
		}
	}
	retry()
	assertAttempts(0)
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })
	fresh := r.must(delivery.MCPAuthorizationAttemptsCreate, request, "start-fresh-authorization").(*protocol.MCPAuthorizationAttemptRef)
	if attempt := awaitMCPAuthorization(t, r, fresh.AttemptID); attempt.Status.Type != protocol.MCPAuthorizationAttemptSucceeded {
		t.Fatalf("fresh authorization failed after retirement: %+v", attempt)
	}
	retry()
	assertAttempts(1)
	r.close()

	if decision := deliverLateAuthorizationStart(t, filepath.Dir(string(path)), starts[0].Intent); decision != mcpconnector.StartRetired {
		t.Fatalf("late start changed the original receipt: %s", decision)
	}
	assertAttempts(1)
}

func deliverLateAuthorizationStart(t *testing.T, data string, intent mcpapp.AuthorizationIntent) mcpconnector.StartDecision {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(firstPartyPackages, "mcp", "bin", "connector"))
	command.Env = append(os.Environ(), process.DataVariable+"="+data)
	client := sdk.NewClient(&sdk.Implementation{Name: "late-authorization-fixture", Version: "1"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	methods, err := connectormcp.NewMethods()
	if err != nil {
		t.Fatal(err)
	}
	capability, err := pluginmcp.ResolveCapability(ctx, session, mcpconnector.Export, methods.Contract)
	if err != nil {
		t.Fatal(err)
	}
	start, err := methods.Start.Bind(capability)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = pluginmcp.WithInvocation(ctx, intent.Invocation)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := start.CallCommand(ctx, pluginmcp.CommandID(intent.Invocation.ID), intent.Start)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func authorizationCrashStore(t *testing.T, path string) *sql.DB {
	t.Helper()
	store, err := persistencefixture.OpenExistingSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func crashMCPAuthorization(t *testing.T, childTest string) (home, runnerDir string) {
	t.Helper()
	home, runnerDir = testsupport.TempDirectory(t), testsupport.TempDirectory(t)
	if _, err := externalMCPRunner(t, runnerDir).Identity(t.Context()); err != nil {
		t.Fatal(err)
	}
	remote := poMCPRemote(t, "record")
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+childTest+"$", "-test.timeout=35s")
	child.Env = append(os.Environ(), "TMPDIR="+testsupport.TempDirectory(t), authorizationAckCrashHome+"="+home,
		authorizationAckCrashRunner+"="+runnerDir, authorizationAckCrashRemote+"="+remote)
	child.WaitDelay = 3 * time.Second
	output, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("authorization child did not exit at its failure point: %v\n%s", err, output)
	}
	return home, runnerDir
}
