package bootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Tangerg/flame/plugins/guardrails/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestStandalonePluginArchiveRetainsItsPrivateCommandOwner(t *testing.T) {
	root := t.TempDir()
	author := filepath.Join(root, "author")
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate distribution author tool")
	}
	repository := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	run := func(directory string, env []string, command string, args ...string) {
		t.Helper()
		process := exec.CommandContext(t.Context(), command, args...)
		process.Dir, process.Env = directory, env
		if output, err := process.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", command, args, err, output)
		}
	}
	run(repository, os.Environ(), "node", "plugins/build-author-kit.mjs", "--plugin", "guardrails", "--output", author, "--offline")
	// An empty module cache and disabled workspace prove vendor is sufficient.
	env := append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOMODCACHE="+filepath.Join(root, "module-cache"))
	run(author, env, "go", "test", "./...")
	run(author, env, "go", "generate", "./...")
	packages, archives := filepath.Join(root, "packages"), filepath.Join(root, "archives")
	run(author, env, "node", "build.mjs", "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", packages, "--archives", archives)
	files, err := filepath.Glob(filepath.Join(archives, "*.zip"))
	if err != nil || len(files) != 1 {
		t.Fatalf("release archive = %v, %v", files, err)
	}
	cfg := poMCPConfig(t)
	host := poMCPOpen(t, cfg)
	installed := poMCPActivate(host, files[0])
	waitCallContributorState(t, host, installed.ID, protocol.MCPServerConnected)
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsoninput.Encode(rpc.ReplaceInput{ExpectedRevision: 1, Phrases: []string{"private phrase"}})
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "guardrails-policy", Contract: contracts.Capability.Identity().String(), Method: contracts.Replace.Name(), Input: input.String(), Target: capability.Target{Scope: "installation", Reference: installed.ID}}
	original := host.must(delivery.PluginsCall, request, "standalone-policy").(*protocol.CallPluginResponse)
	if original.Failure != nil {
		t.Fatal(original.Failure)
	}
	host.close()
	host = poMCPOpen(t, cfg)
	waitCallContributorState(t, host, installed.ID, protocol.MCPServerConnected)
	recovered := host.must(delivery.PluginsCall, request, "standalone-policy").(*protocol.CallPluginResponse)
	if recovered.Failure != nil || recovered.Output != original.Output {
		t.Fatalf("original command changed after restart: %+v", recovered)
	}
}
