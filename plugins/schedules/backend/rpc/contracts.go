package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/schedules/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	app "github.com/Tangerg/flame/plugins/schedules/backend/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
)

// CapabilityReference is the exact reference the Runtime binds this feature under;
// the background-command gate matches on it, so it has one owner.
const CapabilityReference = "schedules"

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
	contracts.List, err = pluginsdk.NewMethod[ListInput, Page](pluginsdk.MethodConfig{Name: List, Kind: capability.Query, Target: capability.ScopeInstallation, Description: "List current schedule definitions and revisions."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Create, err = pluginsdk.NewMethod[schedule.Draft, schedule.Snapshot](pluginsdk.MethodConfig{Name: Create, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Create one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = pluginsdk.NewMethod[UpdateInput, schedule.Snapshot](pluginsdk.MethodConfig{Name: Update, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Change one schedule using its inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = pluginsdk.NewMethod[ItemInput, Ack](pluginsdk.MethodConfig{Name: Delete, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Delete one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.RunNow, err = pluginsdk.NewMethod[ItemInput, app.RunReference](pluginsdk.MethodConfig{Name: RunNow, Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Capture and reconcile one manual Run through its original Runtime acceptance."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Capability, err = capability.New(capability.Config{Reference: CapabilityReference, Composition: capability.Selection, Scope: capability.ScopeInstallation}, contracts.List.Descriptor(), contracts.Create.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.RunNow.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
