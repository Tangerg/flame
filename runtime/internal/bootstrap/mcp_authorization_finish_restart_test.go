package bootstrap

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestMCPAuthorizationDeliversRecordedOutcomeBeforeRetiringItsEpoch(t *testing.T) {
	remote := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := installExternalConnector(r)
	server := r.createUserServer(protocol.MCPServerCandidate{
		Name: "durable-authorization-finish", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-durable-finish-server")
	r.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })

	ownerPath := filepath.Join(cfg.stores.DataDirectory, "plugins", "data", installed.ID, "external-servers.db")
	owner, err := sql.Open("sqlite", "file:"+ownerPath+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := owner.ExecContext(t.Context(), `CREATE TRIGGER postpone_test_authorization_finish
		BEFORE UPDATE OF status ON attempts WHEN NEW.status='succeeded'
		BEGIN SELECT RAISE(ABORT, 'postpone the authorization outcome'); END`); err != nil {
		t.Fatal(err)
	}

	request := protocol.CreateMCPAuthorizationAttemptRequest{Server: server}
	ref := r.must(delivery.MCPAuthorizationAttemptsCreate, request, "begin-durable-finish").(*protocol.MCPAuthorizationAttemptRef)
	runtimeStore, err := persistencefixture.OpenRawSQL(cfg.stores.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimeStore.Close()
	awaitAuthorizationOutcome(t, runtimeStore, `SELECT outcome FROM pending_mcp_authorization_finishes WHERE attempt=?`, ref.AttemptID, mcpconnector.Succeeded)
	r.close()
	if _, err := owner.ExecContext(t.Context(), "DROP TRIGGER postpone_test_authorization_finish"); err != nil {
		t.Fatal(err)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	awaitAuthorizationOutcome(t, owner, `SELECT status FROM attempts WHERE id=?`, ref.AttemptID, mcpconnector.Succeeded)
	attempt, err := r.api.GetMCPAuthorizationAttempt(t.Context(), ref.AttemptID)
	if err != nil || attempt.Status.Type != protocol.MCPAuthorizationAttemptSucceeded {
		t.Fatalf("recorded result after restart = %+v, %v", attempt, err)
	}
}

func awaitAuthorizationOutcome(t *testing.T, db *sql.DB, query, id string, want mcpconnector.Outcome) {
	t.Helper()
	var outcome mcpconnector.Outcome
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		err := db.QueryRowContext(t.Context(), query, id).Scan(&outcome)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
		if outcome == want {
			return
		}
		if outcome != "" && outcome != mcpconnector.Pending {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("attempt %s recorded outcome %q, want %q", id, outcome, want)
}
