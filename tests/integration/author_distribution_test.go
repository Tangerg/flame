//go:build author

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	guardrailsrpc "github.com/Tangerg/flame/plugins/guardrails/backend/rpc"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestStandalonePluginArchiveRetainsItsPrivateCommandOwner(t *testing.T) {
	root := t.TempDir()
	author := filepath.Join(root, "author")
	_, file, _, _ := runtime.Caller(0)
	repository := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	execute := func(directory string, env []string, command string, args ...string) {
		t.Helper()
		process := exec.CommandContext(t.Context(), command, args...)
		process.Dir, process.Env = directory, env
		if output, err := process.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", command, args, err, output)
		}
	}
	execute(repository, os.Environ(), "node", "plugins/scripts/build-author-kit.mjs", "--plugin", "guardrails", "--module-path", "example.com/flame-author/guardrails", "--output", author, "--offline")
	env := append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off", "GOFLAGS=-mod=vendor", "GOMODCACHE="+filepath.Join(root, "module-cache"))
	output, archives := filepath.Join(root, "package"), filepath.Join(root, "archives")
	execute(author, env, "node", "build.mjs", "--source", author, "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", output, "--archives", archives)
	files, err := filepath.Glob(filepath.Join(archives, "*.zip"))
	if err != nil || len(files) != 1 {
		t.Fatalf("release archive = %v, %v", files, err)
	}
	cfg := flameruntime.Config{DataDirectory: tempRuntimeDirectory(t), UserHomePath: tempRuntimeDirectory(t), ConfigDirectories: []string{tempRuntimeDirectory(t)}}
	host, err := flameruntime.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	installed, err := host.InstallPlugin(t.Context(), protocol.InstallPluginRequest{Source: files[0]}, protocol.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			servers, err := host.ListMCPServers(t.Context(), protocol.CallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, server := range servers.Data {
				if server.ID.Origin.InstallationID == installed.ID && server.Status.Type == protocol.MCPServerConnected {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("author backend did not connect")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait()
	contract := ""
	for _, contribution := range installed.Selected.Contributions {
		if contribution.ID == "guardrails-policy" {
			contract = contribution.Contract
		}
	}
	if contract == "" {
		t.Fatal("author did not publish its own contract")
	}
	contracts, err := guardrailsrpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "guardrails-policy", Contract: contract, Method: contracts.Replace.Name(), Input: `{"expectedRevision":1,"phrases":["private phrase"]}`, Target: capability.Target{Scope: "installation", Reference: installed.ID}}
	options := protocol.PluginCallOptions{IdempotencyKey: "standalone-policy"}
	original, err := host.CallPlugin(t.Context(), request, options)
	if err != nil || original.Failure != nil {
		t.Fatalf("author command = %+v, %v", original, err)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	host, err = flameruntime.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	wait()
	recovered, err := host.CallPlugin(t.Context(), request, options)
	if err != nil || recovered.Failure != nil || recovered.Output != original.Output {
		t.Fatalf("original command changed after restart: %+v, %v", recovered, err)
	}
}
