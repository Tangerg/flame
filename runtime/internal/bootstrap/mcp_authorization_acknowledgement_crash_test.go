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

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
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
	home, runnerDir := testsupport.TempDirectory(t), testsupport.TempDirectory(t)
	if _, err := externalMCPRunner(t, runnerDir).Identity(t.Context()); err != nil {
		t.Fatal(err)
	}
	remote := poMCPRemote(t, "record")
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMCPAuthorizationAcknowledgementCrashChild$", "-test.timeout=35s")
	child.Env = append(os.Environ(), "TMPDIR="+testsupport.TempDirectory(t), authorizationAckCrashHome+"="+home,
		authorizationAckCrashRunner+"="+runnerDir, authorizationAckCrashRemote+"="+remote)
	child.WaitDelay = 3 * time.Second
	output, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("authorization child did not exit at the receipt: %v\n%s", err, output)
	}
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
	attempt, err := r.api.GetMCPAuthorizationAttempt(t.Context(), original.Value.AttemptID)
	if err != nil || attempt == nil || attempt.ID != original.Value.AttemptID || attempt.Server != server ||
		attempt.Status.Type != protocol.MCPAuthorizationAttemptUnconfirmed {
		t.Fatalf("connector did not recover the interrupted attempt: %+v, %v", attempt, err)
	}
	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	value := r.must(delivery.MCPAuthorizationAttemptsCreate, request, authorizationAckCrashKey).(*protocol.MCPAuthorizationAttemptRef)
	if *value != original.Value {
		t.Fatalf("recovered authorization acknowledged %q, want %q", value.AttemptID, original.Value.AttemptID)
	}
}
