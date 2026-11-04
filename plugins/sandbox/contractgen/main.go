package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	"github.com/Tangerg/flame/plugins/sandbox/internal/wire"
)

func main() {
	contract, err := define()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(contract.Descriptor(), json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		log.Fatal(err)
	}
}

func define() (*plugincontract.Capability, error) {
	execute, err := pluginmcp.NewMethod[wire.ExecuteInput, sandbox.Result](portable.MethodConfig{Name: "flame.sandbox/execute.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Execute a bounded /bin/sh script in a new private native sandbox workspace. Writes stay in that workspace, the configured home is hidden, network is denied and child environment is scrubbed. No CPU or memory quota is promised. Replaying the original command returns its saved outcome; unknown work is never re-executed."})
	if err != nil {
		return nil, err
	}
	limits, err := pluginmcp.NewMethod[struct{}, wire.Limits](portable.MethodConfig{Name: "flame.sandbox/limits.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect native backend availability and the actual script, timeout and retained-output limits. Availability does not substitute for successful confinement at execution."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.sandbox.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, execute.Descriptor(), limits.Descriptor())
}
