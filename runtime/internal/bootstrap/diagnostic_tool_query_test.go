package bootstrap

import (
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestDiagnosticReadObservesCurrentBytesWithoutCommandIdentity(t *testing.T) {
	const filename = "diagnostic-notes.txt"
	t.Setenv("FLAME_HOME", testsupport.TempDirectory(t))
	workspace := t.TempDir()
	host, _ := openProtocolRuntime(t, newReplyStub("unused"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	request := protocol.InvokeToolRequest{
		Name: string(builtin.Read), Arguments: map[string]any{"path": filename},
		Workspace: &protocol.WorkspaceRef{Path: workspace},
	}
	ctx := t.Context()
	read := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspace, filename), []byte(content+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		result := host.Endpoint().Invoke(ctx, delivery.ToolsInvoke, request, protocol.CallMetadata{})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		encoded, err := json.Marshal(result.Value)
		if err != nil || !strings.Contains(string(encoded), content) {
			t.Fatalf("read result = %s, %v, want current contents %q", encoded, err, content)
		}
	}
	read("initial diagnostic contents")
	keyed := host.Endpoint().Invoke(ctx, delivery.ToolsInvoke, request, protocol.CallMetadata{IdempotencyKey: "obsolete-diagnostic-receipt"})
	if keyed.Failure == nil || !errors.Is(keyed.Failure, protocol.ErrInvalidParams) {
		t.Fatalf("diagnostic query accepted a command identity: %+v", keyed)
	}
	read("updated diagnostic contents")
}
