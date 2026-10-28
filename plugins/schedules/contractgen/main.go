package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
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

func define() (*portable.Capability, error) {
	listMethod, err := qimumcp.NewMethod[wire.ListInput, wire.Page](qimumcp.MethodConfig{Name: wire.List, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List current schedule definitions and revisions."})
	if err != nil {
		return nil, err
	}
	createMethod, err := qimumcp.NewMethod[schedule.Draft, schedule.Snapshot](qimumcp.MethodConfig{Name: wire.Create, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Create one schedule definition."})
	if err != nil {
		return nil, err
	}
	updateMethod, err := qimumcp.NewMethod[wire.UpdateInput, schedule.Snapshot](qimumcp.MethodConfig{Name: wire.Update, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Change one schedule using its inspected revision."})
	if err != nil {
		return nil, err
	}
	deleteMethod, err := qimumcp.NewMethod[wire.ItemInput, wire.Ack](qimumcp.MethodConfig{Name: wire.Delete, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Delete one schedule definition."})
	if err != nil {
		return nil, err
	}
	runNowMethod, err := qimumcp.NewMethod[wire.ItemInput, app.RunReference](qimumcp.MethodConfig{Name: wire.RunNow, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Capture and reconcile one manual Run through its original Runtime acceptance."})
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: wire.CapabilityReference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, listMethod.Descriptor(), createMethod.Descriptor(), updateMethod.Descriptor(), deleteMethod.Descriptor(), runNowMethod.Descriptor())
}
