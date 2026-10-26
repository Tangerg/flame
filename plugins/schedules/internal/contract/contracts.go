package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/schedules/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

// CapabilityReference is the exact reference the Runtime binds this feature under;
// the background-command gate matches on it, so it has one owner.
const CapabilityReference = "schedules"

type Contracts struct {
	Capability *extension.Capability
	List       *pluginsdk.Method[ListInput, Page]
	Create     *pluginsdk.Method[schedule.Draft, schedule.Snapshot]
	Update     *pluginsdk.Method[UpdateInput, schedule.Snapshot]
	Delete     *pluginsdk.Method[ItemInput, Ack]
	RunNow     *pluginsdk.Method[ItemInput, app.RunReference]
}

func NewContracts() (Contracts, error) {
	var contracts Contracts
	var err error
	contracts.List, err = pluginsdk.NewMethod[ListInput, Page](pluginsdk.MethodConfig{Name: List, Kind: extension.Query, Target: extension.ScopeInstallation, Description: "List current schedule definitions and revisions."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Create, err = pluginsdk.NewMethod[schedule.Draft, schedule.Snapshot](pluginsdk.MethodConfig{Name: Create, Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Create one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = pluginsdk.NewMethod[UpdateInput, schedule.Snapshot](pluginsdk.MethodConfig{Name: Update, Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Change one schedule using its inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = pluginsdk.NewMethod[ItemInput, Ack](pluginsdk.MethodConfig{Name: Delete, Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Delete one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.RunNow, err = pluginsdk.NewMethod[ItemInput, app.RunReference](pluginsdk.MethodConfig{Name: RunNow, Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Capture and reconcile one manual Run through its original Runtime acceptance."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: CapabilityReference, Composition: extension.Selection, Scope: extension.ScopeInstallation}, contracts.List.Descriptor(), contracts.Create.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.RunNow.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
