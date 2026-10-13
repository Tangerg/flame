package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/schedulescontracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../../../../plugins/schedules/io.github.tangerg.flame/contracts --ts ../../../../../plugins/schedules/io.github.tangerg.flame/contracts --go-out ../../../../../cli/internal/adapter/runtimebinding/schedules_contract.generated.go --go-package runtimebinding --go-prefix Schedules"

import (
	"github.com/Tangerg/flame/runtime/capability"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type Contracts struct {
	Capability *capability.Contract
	List       *pluginsdk.Method[ListInput, Page]
	Create     *pluginsdk.Method[schedule.Draft, schedule.Snapshot]
	Update     *pluginsdk.Method[UpdateInput, schedule.Snapshot]
	Delete     *pluginsdk.Method[ItemInput, Ack]
	RunNow     *pluginsdk.Method[ItemInput, app.RunReference]
}

func NewContracts() (Contracts, error) {
	var contracts Contracts
	var err error
	contracts.List, err = pluginsdk.NewMethod[ListInput, Page](pluginsdk.MethodConfig{Name: List, Kind: capability.Query, Target: "installation", Description: "List current schedule definitions and revisions."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Create, err = pluginsdk.NewMethod[schedule.Draft, schedule.Snapshot](pluginsdk.MethodConfig{Name: Create, Kind: capability.Mutation, Target: "installation", Description: "Create one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = pluginsdk.NewMethod[UpdateInput, schedule.Snapshot](pluginsdk.MethodConfig{Name: Update, Kind: capability.Mutation, Target: "installation", Description: "Change one schedule using its inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = pluginsdk.NewMethod[ItemInput, Ack](pluginsdk.MethodConfig{Name: Delete, Kind: capability.Mutation, Target: "installation", Description: "Delete one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.RunNow, err = pluginsdk.NewMethod[ItemInput, app.RunReference](pluginsdk.MethodConfig{Name: RunNow, Kind: capability.Mutation, Target: "installation", Description: "Capture and reconcile one manual Run through its original Runtime acceptance."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Capability, err = capability.New(capability.Config{Reference: "schedules", Composition: capability.Selection, Scope: "installation"}, contracts.List.Descriptor(), contracts.Create.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.RunNow.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
