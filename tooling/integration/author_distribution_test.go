//go:build author

package integration

import (
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	flameruntime "github.com/Tangerg/flame/runtime"
)

func TestStandalonePluginArchiveRetainsItsContractAndPrivateCommandOwner(t *testing.T) {
	for _, name := range []string{"guardrails", "memory", "usagereports"} {
		t.Run(name, func(t *testing.T) {
			verifyStandalonePlugin(t, name)
		})
	}
}

func verifyStandalonePlugin(t *testing.T, name string) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
	root := t.TempDir()
	author := filepath.Join(root, "author project 作者")
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
	export := []string{"plugins/scripts/build-author-kit.mjs", "--plugin", name, "--output", author, "--offline"}
	if name != "usagereports" {
		export = append(export, "--module-path", "example.com/flame-author/"+name)
	}
	execute(repository, os.Environ(), "node", export...)
	env := append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off", "GOFLAGS=-mod=vendor", "GOMODCACHE="+filepath.Join(root, "module-cache"))
	output, archives := filepath.Join(root, "package"), filepath.Join(root, "archives")
	if name == "usagereports" {
		execute(author, env, "npm", "run", "package", "--", "--output", output, "--archives", archives)
	} else {
		execute(author, env, "node", "build.mjs", "--source", author, "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", output, "--archives", archives)
	}
	files, err := filepath.Glob(filepath.Join(archives, "*.zip"))
	if err != nil || len(files) != 1 {
		t.Fatalf("release archive = %v, %v", files, err)
	}
	if name == "usagereports" && filepath.Base(files[0]) != "plugin-flame.usagereports-1.0.0.zip" {
		t.Fatal("portable author artifact acquired a platform identity")
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
		if contribution.ID == name+"-policy" || contribution.ID == name+"-provider" || name == "usagereports" && contribution.ID == "usage-reports" {
			contract = contribution.Contract
		}
	}
	if contract == "" {
		t.Fatal("author did not publish its own contract")
	}
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "guardrails-policy", Contract: contract, Method: GuardrailsReplacePolicyV1Method, Input: `{"expectedRevision":1,"phrases":["private phrase"]}`, Target: extension.Target{Scope: "installation", Reference: installed.ID}}
	if name == "memory" {
		published, err := extension.ParseCapability([]byte(memorycontract.CapabilityJSON))
		if err != nil {
			t.Fatal(err)
		}
		exported, err := extension.ParseCapability([]byte(contract))
		if err != nil || exported.Identity() != published.Identity() {
			t.Fatalf("module relocation changed the host's Memory contract: %v", err)
		}
		if len(installed.Selected.Components) == 0 {
			t.Fatal("author release lost its component")
		}
		selected, err := host.SelectPluginProvider(t.Context(), protocol.SelectPluginProviderRequest{Role: "memory", InstallationID: installed.ID, Contribution: "memory-provider"}, protocol.CommandOptions{})
		if err != nil || selected.InstallationID != installed.ID {
			t.Fatalf("select independent Memory: %+v, %v", selected, err)
		}
		request.Contract = memorycontract.CapabilityJSON
		request.Contribution = "memory-provider"
		request.Method = memorycontract.AddV1Method
		request.Input = `{"Scope":"user","CWD":"","Content":"independent author fact"}`
	}
	if name == "usagereports" {
		published, err := extension.ParseCapability([]byte(UsageReportsCapabilityJSON))
		if err != nil {
			t.Fatal(err)
		}
		if published.Identity().String() != contract {
			t.Fatal("independent TypeScript author changed the original usage contract")
		}
		request.Contribution, request.Method, request.Input = "usage-reports", UsageReportsCaptureV1Method, `{"period":"daily"}`
	}
	options := protocol.PluginCallOptions{IdempotencyKey: "standalone-command"}
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
	if name == "memory" {
		providers, err := host.ListPluginProviders(t.Context(), protocol.CallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		selected := false
		for _, provider := range providers.Data {
			if provider.Role == "memory" {
				selected = provider.InstallationID == installed.ID && provider.Contribution == "memory-provider"
			}
		}
		if !selected {
			t.Fatal("restart replaced the selected independent provider")
		}
		request.Method = memorycontract.ListV1Method
		request.Input = `{"Scope":"user","CWD":"","Cursor":""}`
		reply, err := host.CallPlugin(t.Context(), request, protocol.PluginCallOptions{})
		if err != nil || reply.Failure != nil {
			t.Fatalf("read independent Memory: %+v, %v", reply, err)
		}
		var page memorycontract.ListV1Output
		if err := json.Unmarshal([]byte(reply.Output), &page, json.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		if len(page.Rows) != 1 || page.Rows[0].Content != "independent author fact" {
			t.Fatalf("recovery changed the private business result: %+v", page)
		}
		var added memorycontract.AddV1Output
		if err := json.Unmarshal([]byte(original.Output), &added, json.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		if page.Rows[0].ID != added.Item.ID {
			t.Fatal("original command created another item")
		}
	}
}
