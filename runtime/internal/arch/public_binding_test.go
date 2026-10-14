package arch

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPublicPackageSetIsExact(t *testing.T) {
	root := moduleRoot(t)
	want := []string{".", "capability", "idempotency", "jsoninput", "localruntime", "model", "plugin", "point", "protocol", "storage"}
	var got []string
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read Runtime module root: %v", err)
	}
	rootPackageFound := false
	for _, entry := range entries {
		if !entry.IsDir() {
			if !rootPackageFound && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
				got = append(got, ".")
				rootPackageFound = true
			}
			continue
		}
		if entry.Name() == "cmd" || entry.Name() == "internal" {
			continue
		}
		hasProductionGo, err := directoryHasProductionGo(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatalf("inspect %s: %v", entry.Name(), err)
		}
		if hasProductionGo {
			got = append(got, entry.Name())
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("public Runtime packages = %v, want %v; change the public Go baseline deliberately", got, want)
	}
}

func directoryHasProductionGo(directory string) (bool, error) {
	errFound := fmt.Errorf("production Go file found")
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != directory && (entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			return errFound
		}
		return nil
	})
	if err == errFound {
		return true, nil
	}
	return false, err
}

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

go 1.27.0

require (
	github.com/Tangerg/flame/runtime v0.0.0-20261008063147-97d0c5c7e56e
	github.com/Tangerg/flame/runtime/localruntime v0.0.0-20261008063147-97d0c5c7e56e
)

`
	source := `package runtimeconsumer

import (
"context"
"errors"
"testing"
"path/filepath"

flameruntime "github.com/Tangerg/flame/runtime"
"github.com/Tangerg/flame/runtime/capability"
"github.com/Tangerg/flame/runtime/jsoninput"
"github.com/Tangerg/flame/runtime/idempotency"
"github.com/Tangerg/flame/runtime/localruntime"
modelchoice "github.com/Tangerg/flame/runtime/model"
modelcatalog "github.com/Tangerg/flame/runtime/model/catalog"
pluginsdk "github.com/Tangerg/flame/runtime/plugin"
pluginstorage "github.com/Tangerg/flame/runtime/plugin/sqlite"
sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
"github.com/Tangerg/flame/runtime/point"
"github.com/Tangerg/flame/runtime/protocol"
)

var _ = flameruntime.Open
var _ = flameruntime.ConnectCallback
var _ = flameruntime.BindPluginStreamingMethod[struct{}, uint64, bool]
var _ flameruntime.PluginStreamer = (*flameruntime.Runtime)(nil)
var _ flameruntime.PluginStreamer = (*flameruntime.Client)(nil)
var _ flameruntime.PluginStreamer = (*flameruntime.PluginCallbackCaller)(nil)
var _ *flameruntime.Runtime
var _ = localruntime.ReadToken
var _ = modelchoice.NewWithReasoningEffort
var _ = (modelcatalog.Selections{}).AdmitSelection
var _ protocol.RunEvent
var _ = protocol.DecodeRequest
var _ = pluginsdk.RegisterTools
var _ = pluginsdk.RegisterClient
var _ = pluginsdk.CallCommand[string, string]
var _ pluginsdk.Command[string]
var _ = pluginsdk.NewMethod[struct{}, string]
var _ = pluginsdk.NewStreamingMethod[struct{}, uint64, bool]
var _ capability.MethodDescriptor
var _ = pluginsdk.NewBinding
var _ = pluginsdk.Resolve
var _ *pluginsdk.Binding
var _ = jsoninput.Encode[struct{}]
var _ = pluginsdk.RequestInput
var _ = pluginsdk.NewCallbacks
var _ pluginsdk.CallbackRef
var _ = point.Admit
var _ = flameruntime.ResolvePointValues[struct{}]

func TestLocalContractAdmission(t *testing.T) {
	contract, err := capability.New(capability.Config{Reference: "example.com/records", Composition: capability.Selection, Scope: "installation"}, capability.MethodDescriptor{Name: "records/read", Kind: capability.Query, Target: "installation", Input: []byte("{\"type\":\"object\"}"), Output: []byte("{\"type\":\"string\"}")})
	if err != nil {
		t.Fatal(err)
	}
	method, found := contract.Method("records/read")
	if !found {
		t.Fatal("required method unavailable")
	}
	input, err := jsoninput.Encode(struct{}{})
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
	if _, err := runtime.Discover(ctx, flameruntime.CallOptions{}); err != nil {
		return err
	}
	_, events, err := runtime.StartRun(ctx, protocol.StartRunRequest{}, flameruntime.RunCommandOptions{})
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

func TestPublicSQLiteReceiptsKeepTheBusinessOwner(t *testing.T) {
 schema := []string{pluginstorage.CommandsSchema, "CREATE TABLE IF NOT EXISTS records (value TEXT NOT NULL)"}
 db, err := sqlitebase.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), schema)
 if err != nil { t.Fatal(err) }
 defer db.Close()
 input, err := jsoninput.Encode(struct{}{})
 if err != nil { t.Fatal(err) }
 calls := 0
 for range 2 {
  value, err := pluginstorage.Command(t.Context(), db, "records.update", "original", struct{}{}, input, nil, func(ctx context.Context, _ struct{}) (bool, error) {
   calls++
   _, err := sqlitebase.Connection(ctx, db).ExecContext(ctx, "INSERT INTO records VALUES ('owned')")
   return err == nil, err
  })
  if err != nil || !value { t.Fatalf("owned command = %v, %v", value, err) }
 }
 changed, err := jsoninput.Encode(map[string]bool{"changed": true})
 if err != nil { t.Fatal(err) }
 _, err = pluginstorage.Command(t.Context(), db, "records.update", "original", struct{}{}, changed, nil, func(context.Context, struct{}) (bool, error) {
   calls++
   return false, nil
 })
 if !errors.Is(err, idempotency.ErrKeyConflict) { t.Fatalf("changed identity lacks public conflict category: %v", err) }
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

	workspace := fmt.Sprintf("go 1.27.0\n\nuse (\n%s\n%s\n%s\n)\n", filepath.ToSlash(directory), filepath.ToSlash(root), filepath.ToSlash(filepath.Join(root, "localruntime")))
	writeConsumerFile(t, filepath.Join(directory, "go.work"), workspace)
	command := exec.CommandContext(t.Context(), "go", "test", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK="+filepath.Join(directory, "go.work"), "GOPROXY=off")
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
