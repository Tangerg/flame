package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"

	"github.com/Tangerg/frame"
)

func main() { plugincontract.Publish(define) }

func define() (*plugincontract.Capability, error) {
	listMethod, err := pluginmcp.NewMethod[wire.ListInput, wire.Page](frame.MethodConfig{Name: wire.List, Kind: frame.Query, Target: extension.ScopeInstallation, Description: "List current schedule definitions and revisions."})
	if err != nil {
		return nil, err
	}
	createMethod, err := pluginmcp.NewMethod[schedule.Draft, schedule.Snapshot](frame.MethodConfig{Name: wire.Create, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Create one schedule definition."})
	if err != nil {
		return nil, err
	}
	updateMethod, err := pluginmcp.NewMethod[wire.UpdateInput, schedule.Snapshot](frame.MethodConfig{Name: wire.Update, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Change one schedule using its inspected revision."})
	if err != nil {
		return nil, err
	}
	deleteMethod, err := pluginmcp.NewMethod[wire.ItemInput, wire.Ack](frame.MethodConfig{Name: wire.Delete, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Delete one schedule definition."})
	if err != nil {
		return nil, err
	}
	runNowMethod, err := pluginmcp.NewMethod[wire.ItemInput, app.RunReference](frame.MethodConfig{Name: wire.RunNow, Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Capture and reconcile one manual Run through its original Runtime acceptance."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: wire.CapabilityReference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, listMethod.Descriptor(), createMethod.Descriptor(), updateMethod.Descriptor(), deleteMethod.Descriptor(), runNowMethod.Descriptor())
}
