package bootstrap

import (
	"database/sql/driver"
	"errors"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	modernsqlite "modernc.org/sqlite"
)

const (
	directoryCrashHome     = "FLAME_TEST_MCP_DIRECTORY_CRASH_HOME"
	directoryCrashRunner   = "FLAME_TEST_MCP_DIRECTORY_CRASH_RUNNER"
	directoryCrashEndpoint = "FLAME_TEST_MCP_DIRECTORY_CRASH_ENDPOINT"
	directoryCrashServer   = "create-before-grant-confirmation"
	directoryCrashKey      = "create-with-new-grant"
)

func TestExternalDirectoryCrashChild(t *testing.T) {
	home := os.Getenv(directoryCrashHome)
	if home == "" {
		return
	}
	if err := modernsqlite.RegisterScalarFunction("test_mcp_directory_crash", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	cfg := externalMCPRestartConfig(t, home, os.Getenv(directoryCrashRunner))
	host := poMCPOpen(t, cfg)
	installExternalConnector(host)

	questionCrashDatabase(t, home, `CREATE TRIGGER interrupt_mcp_grant_confirmation
		BEFORE UPDATE OF proposal ON mcp_launch_grants
		WHEN OLD.proposal IS NOT NULL AND NEW.proposal IS NULL
		BEGIN SELECT test_mcp_directory_crash(); END`)
	result := host.endpoint.Invoke(t.Context(), delivery.MCPServersCreate, directoryCrashCandidate(os.Getenv(directoryCrashEndpoint)), protocol.CallMetadata{IdempotencyKey: directoryCrashKey})
	t.Fatalf("grant confirmation crash was not reached: %+v (%v)", result, errors.Unwrap(result.Failure))
}

func TestMCPDirectoryRecoversTheOriginalDirectoryReceiptAfterAbruptExit(t *testing.T) {
	home := testsupport.TempDirectory(t)
	runnerDir := testsupport.TempDirectory(t)
	if _, err := externalMCPRunner(t, runnerDir).Identity(t.Context()); err != nil {
		t.Fatal(err)
	}
	remote := poMCPRemote(t, "record")
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExternalDirectoryCrashChild$", "-test.timeout=30s")
	child.Env = append(os.Environ(), "TMPDIR="+testsupport.TempDirectory(t), directoryCrashHome+"="+home,
		directoryCrashRunner+"="+runnerDir, directoryCrashEndpoint+"="+remote)
	child.WaitDelay = 3 * time.Second
	output, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("child did not stop after connector acceptance: %v\n%s", err, output)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER interrupt_mcp_grant_confirmation")

	cfg := externalMCPRestartConfig(t, home, runnerDir)
	restored := poMCPOpen(t, cfg)
	id := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: directoryCrashServer}
	const newerDescription = "updated after directory recovery"
	assertDescription := func(expected string) {
		t.Helper()
		page, err := restored.api.ListMCPServers(t.Context())
		if err != nil {
			t.Fatalf("list servers after connector acceptance: %v", err)
		}
		if !slices.ContainsFunc(page.Data, func(server protocol.MCPServer) bool {
			return server.ID == id && server.Connection.URL == remote && server.Description == expected
		}) {
			t.Fatalf("restored directory = %+v, want %v with description %q", page.Data, id, expected)
		}
	}
	assertDescription("")
	restored.poMCPTools(&id, func(tools []protocol.MCPTool) bool { return len(tools) == 1 && tools[0].Name == "record" })
	replayCreate := func() {
		t.Helper()
		original := restored.endpoint.Invoke(t.Context(), delivery.MCPServersCreate, directoryCrashCandidate(remote), protocol.CallMetadata{IdempotencyKey: directoryCrashKey})
		if original.Failure != nil || original.Value != (struct{}{}) {
			t.Fatalf("original create after the connector committed = %+v, %v", original.Value, original.Failure)
		}
	}
	replayCreate()
	restored.must(delivery.MCPServersUpdate, protocol.UpdateMCPServerRequest{MCPDirectoryTarget: restored.userServerTarget(id), Description: new(newerDescription)}, "update-after-recovery")
	replayCreate()
	assertDescription(newerDescription)
}

func directoryCrashCandidate(url string) protocol.MCPServerCandidate {
	return protocol.MCPServerCandidate{
		Name: directoryCrashServer, Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: url},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}
}
