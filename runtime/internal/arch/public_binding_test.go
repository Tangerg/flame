package arch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicBindingsCompileForAnExternalModule proves the public Go bindings
// are usable without the Runtime module's internal-package privilege. Exact
// operation coverage is enforced by the root binding's own surface test.
func TestPublicBindingsCompileForAnExternalModule(t *testing.T) {
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := moduleRoot(t)
	goMod := `module example.com/runtimeconsumer

go 1.27.3


`
	source := `package runtimeconsumer

import (
"github.com/Tangerg/flame/packages/go-sdk/extension"
"context"
"errors"
"testing"
"path/filepath"

flameruntime "github.com/Tangerg/flame/runtime"
runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
"github.com/Tangerg/flame/qimu/portable"
"github.com/Tangerg/flame/qimu/jsonvalue"
"github.com/Tangerg/flame/packages/local-runtime"
modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
"github.com/Tangerg/flame/packages/go-sdk/model/catalog"
pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
qimumcp "github.com/Tangerg/flame/qimu/mcp"
pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
sqlitebase "github.com/Tangerg/flame/qimu/sqlite"
"github.com/Tangerg/flame/packages/go-sdk/protocol"
"github.com/Tangerg/scope/core/chat"
)

var _ = flameruntime.Open
var _ = runtimeclient.ConnectCallback
var _ = runtimeclient.BindPluginStreamingMethod[struct{}, uint64, bool]
var _ runtimeclient.PluginStreamer = (*runtimeclient.Client)(nil)
var _ runtimeclient.PluginStreamer = (*runtimeclient.PluginCallbackCaller)(nil)
var _ *flameruntime.Runtime
var _ = localruntime.ReadToken
var _ = modelchoice.NewWithReasoningEffort
var _ = (catalog.Selections{}).AdmitSelection
var _ protocol.RunEvent
var _ = protocol.DecodeRequest
var _ = pluginsdk.CallTextEmbedding
var _ = pluginsdk.NewTextEmbeddingCallback
var _ = pluginsdk.RegisterTools
var _ = qimumcp.RegisterClient
var _ = (*qimumcp.Method[string, string]).CallCommand
var _ qimumcp.Command[string]
var _ = qimumcp.NewMethod[struct{}, string]
var _ = qimumcp.NewStreamingMethod[struct{}, uint64, bool]
var _ portable.MethodDescriptor
var _ = qimumcp.NewBinding
var _ = qimumcp.Resolve
var _ *qimumcp.Binding
var _ = jsonvalue.Encode[struct{}]
var _ = pluginsdk.ReadToolReceipt
var _ = qimumcp.RequestInput
var _ = qimumcp.NewCallbacks
var _ qimumcp.CallbackRef
var _ = portable.AdmitPoint
var _ = runtimeclient.ResolvePointValues[struct{}]

func TestLocalContractAdmission(t *testing.T) {
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "example.com/records", Composition: portable.Selection, Scope: extension.ScopeInstallation}, portable.MethodDescriptor{Name: "records/read", Kind: portable.Query, Target: extension.ScopeInstallation, Input: []byte("{\"type\":\"object\"}"), Output: []byte("{\"type\":\"string\"}")})
	if err != nil {
		t.Fatal(err)
	}
	method, found := contract.Method("records/read")
	if !found {
		t.Fatal("required method unavailable")
	}
	input, err := jsonvalue.Encode(struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateInput(input); err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateOutput([]byte("7")); err == nil {
		t.Fatal("invalid output admitted")
	}
}

func consume(ctx context.Context, runtime *flameruntime.Runtime) error {
	if _, err := runtime.Discover(ctx, protocol.CallOptions{}); err != nil {
		return err
	}
	_, events, err := runtime.StartRun(ctx, protocol.StartRunRequest{}, protocol.RunCommandOptions{})
	if err != nil {
		return err
	}
	for _, err := range events {
		if err != nil {
			return err
		}
	}
	return runtime.Close()
}

func TestPublicToolCollaboratorsHaveNoFeatureDependency(t *testing.T) {
 want := pluginsdk.ToolContext{CWD: t.TempDir(), EmbeddingSpace: "captured-space"}
 ctx, err := pluginsdk.WithToolContext(t.Context(), want)
 if err != nil { t.Fatal(err) }
 actual, err := pluginsdk.ToolContextFromContext(ctx)
 if err != nil || actual != want { t.Fatalf("public Tool context = %+v, %v", actual, err) }
 method, err := pluginsdk.TextEmbeddingMethod()
 if err != nil || method.Descriptor().Kind != portable.Mutation || method.Descriptor().Target != "call" { t.Fatalf("public embedding contract = %+v, %v", method, err) }
}

func TestPublicSQLiteReceiptsKeepTheBusinessOwner(t *testing.T) {
 schema := []string{pluginstorage.CommandsSchema, "CREATE TABLE IF NOT EXISTS records (value TEXT NOT NULL)"}
 db, err := sqlitebase.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), schema)
 if err != nil { t.Fatal(err) }
 defer db.Close()
 input, err := jsonvalue.Encode(struct{}{})
 if err != nil { t.Fatal(err) }
 calls := 0
 for range 2 {
  value, err := pluginstorage.Command(t.Context(), db, "records.update", "original", input, nil, func(ctx context.Context, _ struct{}) (bool, error) {
   calls++
   _, err := sqlitebase.Connection(ctx, db).ExecContext(ctx, "INSERT INTO records VALUES ('owned')")
   return err == nil, err
  })
  if err != nil || !value { t.Fatalf("owned command = %v, %v", value, err) }
 }
 receipt, err := pluginstorage.ReadReceipt(t.Context(), db, qimumcp.ReceiptRequest{ID: "original", Method: "records.update", InputDigest: input.Digest()})
 if err != nil || !receipt.Found { t.Fatalf("public owner read = %+v, %v", receipt, err) }
 value, err := qimumcp.DecodeReply[bool](receipt.Reply)
 if err != nil || !value { t.Fatalf("original owner value = %v, %v", value, err) }
 projected, err := pluginsdk.NewToolReceipt(chat.NewTextToolOutput("owned"), false)
 if err != nil || !projected.Found || projected.IsError { t.Fatalf("public Tool projection = %+v, %v", projected, err) }
 changed, err := jsonvalue.Encode(map[string]bool{"changed": true})
 if err != nil { t.Fatal(err) }
 _, err = pluginstorage.Command(t.Context(), db, "records.update", "original", changed, nil, func(context.Context, map[string]bool) (bool, error) {
   calls++
   return false, nil
 })
 if !errors.Is(err, portable.ErrInvocationConflict) { t.Fatalf("changed identity lacks public conflict category: %v", err) }
 var count int
 if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM records").Scan(&count); err != nil { t.Fatal(err) }
 if calls != 1 || count != 1 { t.Fatalf("receipt repeated business mutation: calls=%d records=%d", calls, count) }
}

`
	writeConsumerFile(t, filepath.Join(directory, "go.mod"), goMod)
	writeConsumerFile(t, filepath.Join(directory, "consumer_test.go"), source)
	checksums, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatalf("read Runtime dependency checksums: %v", err)
	}
	writeConsumerFile(t, filepath.Join(directory, "go.sum"), string(checksums))

	workspace := fmt.Sprintf("go 1.27.3\n\nuse (\n%q\n%q\n%q\n%q\n%q\n)\n", filepath.ToSlash(directory), filepath.ToSlash(root), filepath.ToSlash(dependencyModuleRoot(t, sdkModulePath)), filepath.ToSlash(dependencyModuleRoot(t, localRuntimeModulePath)), filepath.ToSlash(dependencyModuleRoot(t, qimuModulePath)))
	writeConsumerFile(t, filepath.Join(directory, "go.work"), workspace)
	dependencies := exec.CommandContext(t.Context(), "go", "list", "-deps", runtimeModulePath)
	dependencies.Dir = directory
	dependencies.Env = append(os.Environ(), "GOWORK="+filepath.Join(directory, "go.work"), "GOPROXY=off", "GONOPROXY=none", "GONOSUMDB=github.com/Tangerg/flame", "GOTOOLCHAIN=auto")
	output, err := dependencies.CombinedOutput()
	if err != nil {
		t.Fatalf("load external Runtime build closure: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if strings.HasPrefix(dependency, "github.com/Tangerg/flame/plugins/") {
			t.Fatalf("public Runtime builds a private feature implementation: %s", dependency)
		}
	}
	command := exec.CommandContext(t.Context(), "go", "test", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK="+filepath.Join(directory, "go.work"), "GOPROXY=off", "GONOPROXY=none", "GONOSUMDB=github.com/Tangerg/flame", "GOTOOLCHAIN=auto")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile external Runtime consumer: %v\n%s", err, output)
	}
}

func writeConsumerFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write external consumer fixture %s: %v", path, err)
	}
}

// dependencyModuleRoot follows the selected module graph for source-based contract
// checks. It works in both a workspace and an independently downloaded module.
func dependencyModuleRoot(t *testing.T, module string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "go", "list", "-m", "-f", "{{.Dir}}", module)
	command.Dir = moduleRoot(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve dependency module %s: %v\n%s", module, err, output)
	}
	directory := strings.TrimSpace(string(output))
	if directory == "" {
		t.Fatalf("dependency module %s has no resolved source directory", module)
	}
	return directory
}
