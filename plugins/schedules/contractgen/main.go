package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"
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
	listMethod, err := pluginmcp.NewMethod[wire.ListInput, wire.Page](portable.MethodConfig{Name: wire.List, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List current schedule definitions and revisions."})
	if err != nil {
		return nil, err
	}
	createMethod, err := pluginmcp.NewMethod[schedule.Draft, schedule.Snapshot](portable.MethodConfig{Name: wire.Create, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Create one schedule definition."})
	if err != nil {
		return nil, err
	}
	updateMethod, err := pluginmcp.NewMethod[wire.UpdateInput, schedule.Snapshot](portable.MethodConfig{Name: wire.Update, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Change one schedule using its inspected revision."})
	if err != nil {
		return nil, err
	}
	deleteMethod, err := pluginmcp.NewMethod[wire.ItemInput, wire.Ack](portable.MethodConfig{Name: wire.Delete, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Delete one schedule definition."})
	if err != nil {
		return nil, err
	}
	runNowMethod, err := pluginmcp.NewMethod[wire.ItemInput, app.RunReference](portable.MethodConfig{Name: wire.RunNow, Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Capture and reconcile one manual Run through its original Runtime acceptance."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: wire.CapabilityReference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, listMethod.Descriptor(), createMethod.Descriptor(), updateMethod.Descriptor(), deleteMethod.Descriptor(), runNowMethod.Descriptor())
}
