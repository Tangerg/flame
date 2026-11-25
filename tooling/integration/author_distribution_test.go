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

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	flameruntime "github.com/Tangerg/flame/runtime"

	framecall "github.com/Tangerg/frame/call"
)

// standaloneAuthor is a first-party plugin re-exported as an independent
// author project and driven through one private command.
type standaloneAuthor struct {
	name string
	// typescript authors package with npm under the manifest's release name;
	// Go authors relocate their module and build with the Go kit.
	typescript bool
	// contribution carries the author's contract and receives the command.
	contribution string
	// publishedContract, when set, is the host contract the author must keep.
	publishedContract string
	method, input     string
	// memoryProvider authors become the selected Memory provider.
	memoryProvider bool
}

var standaloneAuthors = []standaloneAuthor{
	{
		name: "guardrails", contribution: "guardrails-policy",
		method: GuardrailsReplacePolicyV1Method, input: `{"expectedRevision":1,"phrases":["private phrase"]}`,
	},
	{
		name: "memory", contribution: "memory-provider", publishedContract: memorycontract.CapabilityJSON,
		method: memorycontract.AddV3Method, input: `{"scope":"user","cwd":"","content":"independent author fact"}`,
		memoryProvider: true,
	},
	{
		name: "usagereports", typescript: true, contribution: "usage-reports", publishedContract: UsageReportsCapabilityJSON,
		method: UsageReportsCaptureV1Method, input: `{"period":"daily"}`,
	},
}

func TestStandalonePluginArchiveRetainsItsContractAndPrivateCommandOwner(t *testing.T) {
	for _, author := range standaloneAuthors {
		t.Run(author.name, func(t *testing.T) {
			verifyStandalonePlugin(t, author)
		})
	}
}

func verifyStandalonePlugin(t *testing.T, author standaloneAuthor) {
	name := author.name
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_MODEL", "claude-opus-5")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
	root := t.TempDir()
	project := filepath.Join(root, "author project 作者")
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
	export := []string{"plugins/scripts/build-author-kit.mjs", "--plugin", name, "--output", project, "--offline"}
	if !author.typescript {
		export = append(export, "--module-path", "example.com/flame-author/"+name)
	}
	execute(repository, os.Environ(), "node", export...)
	env := append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off", "GOFLAGS=-mod=vendor", "GOMODCACHE="+filepath.Join(root, "module-cache"))
	output, archives := filepath.Join(root, "package"), filepath.Join(root, "archives")
	if author.typescript {
		execute(project, env, "npm", "run", "package", "--", "--output", output, "--archives", archives)
	} else {
		execute(project, env, "node", "build.mjs", "--source", project, "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", output, "--archives", archives)
	}
	files, err := filepath.Glob(filepath.Join(archives, "*.zip"))
	if err != nil || len(files) != 1 {
		t.Fatalf("release archive = %v, %v", files, err)
	}
	if author.typescript {
		manifest, err := os.ReadFile(filepath.Join(repository, "plugins", name, "package", "plugin.json"))
		if err != nil {
			t.Fatal(err)
		}
		var identity struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(manifest, &identity); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(files[0]) != "plugin-"+identity.Name+"-"+identity.Version+".zip" {
			t.Fatal("portable author artifact differs from its manifest identity")
		}
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
	waitForAuthorBackend(t, host, installed.ID)
	contract := ""
	for _, contribution := range installed.Selected.Contributions {
		if contribution.ID == author.contribution {
			contract = contribution.Contract
		}
	}
	if contract == "" {
		t.Fatal("author did not publish its own contract")
	}
	if author.publishedContract != "" {
		published, err := plugincontract.ParseCapability([]byte(author.publishedContract))
		if err != nil {
			t.Fatal(err)
		}
		if contract != published.Identity().String() {
			t.Fatal("standalone author changed the contract its host publishes")
		}
	}
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: author.contribution, Contract: contract, Method: author.method, Input: author.input, Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID}}
	if author.memoryProvider {
		if len(installed.Selected.Components) == 0 {
			t.Fatal("author release lost its component")
		}
		selected, err := host.SelectPluginProvider(t.Context(), protocol.SelectPluginProviderRequest{Role: "memory", InstallationID: installed.ID, Contribution: "memory-provider"}, protocol.CommandOptions{})
		if err != nil || selected.InstallationID != installed.ID {
			t.Fatalf("select independent Memory: %+v, %v", selected, err)
		}
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
	waitForAuthorBackend(t, host, installed.ID)
	recovered, err := host.CallPlugin(t.Context(), request, options)
	if err != nil || recovered.Failure != nil || recovered.Output != original.Output {
		t.Fatalf("original command changed after restart: %+v, %v", recovered, err)
	}
	if author.memoryProvider {
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
		request.Method = memorycontract.ListV3Method
		request.Input = `{"scope":"user","cwd":"","cursor":""}`
		reply, err := host.CallPlugin(t.Context(), request, protocol.PluginCallOptions{})
		if err != nil || reply.Failure != nil {
			t.Fatalf("read independent Memory: %+v, %v", reply, err)
		}
		var page memorycontract.Page
		if err := json.Unmarshal([]byte(reply.Output), &page, json.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		if len(page.Rows) != 1 || page.Rows[0].Content != "independent author fact" {
			t.Fatalf("recovery changed the private business result: %+v", page)
		}
		var added memorycontract.AddResult
		if err := json.Unmarshal([]byte(original.Output), &added, json.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		if added.Item == nil || string(page.Rows[0].Id) != string(added.Item.Id) {
			t.Fatal("original command created another item")
		}
	}
}

// waitForAuthorBackend polls until the installation's MCP backend connects, and
// logs every server status of the installation if it never does.
func waitForAuthorBackend(t *testing.T, host *flameruntime.Runtime, installationID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		servers, err := host.ListMCPServers(t.Context(), protocol.CallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, server := range servers.Data {
			if server.ID.Origin.InstallationID == installationID && server.Status.Type == protocol.MCPServerConnected {
				return
			}
		}
		if time.Now().After(deadline) {
			for _, server := range servers.Data {
				if server.ID.Origin.InstallationID != installationID {
					continue
				}
				status, err := json.Marshal(server.Status)
				if err != nil {
					t.Fatalf("encode author connection status: %v", err)
				}
				t.Logf("author connection status: %s", status)
			}
			t.Fatalf("author backend did not connect for installation %s", installationID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
