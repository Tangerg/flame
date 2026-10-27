package pluginpackage

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sdkplugin "github.com/Tangerg/flame/packages/go-sdk/plugin"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestMain(tests *testing.M) { os.Exit(testsupport.RunWithNode(tests)) }

func managedFiles(managed string) map[string]string {
	return map[string]string{
		"plugin.json":                          `{"$schema":"` + manifestSchema + `","name":"test.managed","extensions":{"` + plugin.Namespace + `":{"declaration":"` + plugin.Namespace + `/declaration.json"}}}`,
		plugin.Namespace + "/declaration.json": `{"protocol":"` + sdkplugin.DeclarationProtocol + `","managedBackends":` + managed + `}`,
		"backend/start.mjs":                    `const value = await import("./lazy.mjs"); process.stdout.write(value.default);`,
		"backend/lazy.mjs":                     `export default "original";`,
	}
}

func TestManagedBackendAdmissionHasOneStartupOwner(t *testing.T) {
	for _, declaration := range []string{
		`[{"id":"backend","runtime":"node","entry":"backend/start.mjs"},{"id":"backend","runtime":"node","entry":"backend/start.mjs"}]`,
		`[{"id":"backend","runtime":"bun","entry":"backend/start.mjs"}]`,
		`[{"id":"backend","runtime":"node","entry":"../start.mjs"}]`,
		`[{"id":"backend","runtime":"node","entry":"backend/missing.mjs"}]`,
		`[{"id":"backend","runtime":"node","entry":"backend/start.mjs","command":"node"}]`,
	} {
		if _, err := publishPackage(t.Context(), testReleases(t), writePackage(t, managedFiles(declaration))); !errors.Is(err, plugin.ErrInvalid) {
			t.Fatalf("invalid managed startup admitted: %s: %v", declaration, err)
		}
	}
	files := managedFiles(`[{"id":"backend","runtime":"node","entry":"backend/start.mjs"}]`)
	files["mcp.json"] = `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"node"}}}`
	if _, err := publishPackage(t.Context(), testReleases(t), writePackage(t, files)); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("two startup sources selected a winner: %v", err)
	}
	files["mcp.json"] = `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"invalid-native-startup"}}}`
	if _, err := publishPackage(t.Context(), testReleases(t), writePackage(t, files)); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("managed startup replaced a rejected native declaration: %v", err)
	}
}

func TestManagedBackendLeasesCompleteGraphAndIgnoresAmbientNode(t *testing.T) {
	r := testReleases(t)
	source := writePackage(t, managedFiles(`[{"id":"backend","runtime":"node","entry":"backend/start.mjs"}]`))
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NODE_OPTIONS", "--import=/missing/ambient.mjs")
	t.Setenv("NODE_PATH", "/missing/modules")
	connection, found, err := r.Connection(t.Context(), installation, release, testsupport.ServerName("backend"))
	if err != nil || !found {
		t.Fatalf("managed connection: %v, %v", found, err)
	}
	defer connection.Retire.Close()
	if err := os.WriteFile(filepath.Join(source, "backend/lazy.mjs"), []byte(`export default "changed";`), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), connection.Stdio.Command, connection.Stdio.Args...)
	command.Dir = connection.Stdio.Dir
	for key, value := range connection.Stdio.Env {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "original" {
		t.Fatalf("leased managed graph: %s, %v", output, err)
	}
	if connection.Server.ExecutionEnvironment == "" {
		t.Fatal("activation lost its execution environment identity")
	}
}

func TestMissingManagedCompanionKeepsNativeActivationAndManagementUsable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	node, err := localruntime.NodeForExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(node.Command(), node.Command()+".unavailable"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Rename(node.Command()+".unavailable", node.Command()); err != nil {
			t.Error(err)
		}
	})
	files := managedFiles(`[{"id":"backend","runtime":"node","entry":"backend/start.mjs"}]`)
	files["mcp.json"] = `{"$schema":"` + MCPSchema + `","mcpServers":{"native":{"type":"stdio","command":"./bin/native"}}}`
	files["bin/native"] = "fixture"
	r := testReleases(t)
	source := writePackage(t, files)
	release, err := publishPackage(t.Context(), r, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Connection(t.Context(), installation, release, testsupport.ServerName("backend")); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("missing companion was not isolated as unavailable: %v", err)
	}
	native, found, err := r.Connection(t.Context(), installation, release, testsupport.ServerName("native"))
	if err != nil || !found {
		t.Fatalf("missing Node withdrew native activation: %v, %v", found, err)
	}
	if err := native.Retire.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := r.Realize(t.Context(), installation, release)
	if err != nil || len(state.UnavailableBackends) != 1 || state.UnavailableBackends[0] != testsupport.ServerName("backend") {
		t.Fatalf("management lost independent realization: %+v: %v", state, err)
	}
}
