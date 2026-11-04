package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"
	"github.com/Tangerg/flame/plugins/filewatch/internal/wire"
)

func main() {
	contract, err := define()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(contract.Descriptor(), json.Deterministic(true))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		log.Fatal(err)
	}
}

func define() (*plugincontract.Capability, error) {
	getMethod, err := pluginmcp.NewMethod[struct{}, filewatch.Snapshot](portable.MethodConfig{Name: "flame.filewatch/get.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read this installation's single persistent file trigger and configuration revision."})
	if err != nil {
		return nil, err
	}
	configureMethod, err := pluginmcp.NewMethod[wire.ConfigureInput, filewatch.Snapshot](portable.MethodConfig{Name: "flame.filewatch/configure.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Replace or rearm the trigger at its current revision; omitting trigger disables observation. Configuration acceptance does not claim a live watch. Captured work remains immutable."})
	if err != nil {
		return nil, err
	}
	statusMethod, err := pluginmcp.NewMethod[struct{}, filewatch.Status](portable.MethodConfig{Name: "flame.filewatch/status.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect the actual background observation revision and failures. A failed watch requires explicit configuration rearm; accepted dispatch intents continue reconciliation."})
	if err != nil {
		return nil, err
	}
	eventsMethod, err := pluginmcp.NewMethod[struct{}, []filewatch.Header](portable.MethodConfig{Name: "flame.filewatch/events.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read at most 256 retained observation headers, newest first. Accepted means core confirmed the captured Run opening, not that execution completed. Pending intents are never pruned."})
	if err != nil {
		return nil, err
	}
	eventMethod, err := pluginmcp.NewMethod[wire.EventInput, filewatch.Event](portable.MethodConfig{Name: "flame.filewatch/event.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read one retained observation and its complete original dispatch capture."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.filewatch.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, getMethod.Descriptor(), configureMethod.Descriptor(), statusMethod.Descriptor(), eventsMethod.Descriptor(), eventMethod.Descriptor())
}
