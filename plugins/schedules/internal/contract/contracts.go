package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/schedules/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

// CapabilityReference is the exact reference the Runtime binds this feature under;
// the background-command gate matches on it, so it has one owner.
const CapabilityReference = "schedules"

type Contracts struct {
	Capability *portable.Capability
	List       *qimumcp.Method[ListInput, Page]
	Create     *qimumcp.Method[schedule.Draft, schedule.Snapshot]
	Update     *qimumcp.Method[UpdateInput, schedule.Snapshot]
	Delete     *qimumcp.Method[ItemInput, Ack]
	RunNow     *qimumcp.Method[ItemInput, app.RunReference]
}

func NewContracts() (Contracts, error) {
	var contracts Contracts
	var err error
	contracts.List, err = qimumcp.NewMethod[ListInput, Page](qimumcp.MethodConfig{Name: List, Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List current schedule definitions and revisions."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Create, err = qimumcp.NewMethod[schedule.Draft, schedule.Snapshot](qimumcp.MethodConfig{Name: Create, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Create one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Update, err = qimumcp.NewMethod[UpdateInput, schedule.Snapshot](qimumcp.MethodConfig{Name: Update, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Change one schedule using its inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Delete, err = qimumcp.NewMethod[ItemInput, Ack](qimumcp.MethodConfig{Name: Delete, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Delete one schedule definition."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.RunNow, err = qimumcp.NewMethod[ItemInput, app.RunReference](qimumcp.MethodConfig{Name: RunNow, Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Capture and reconcile one manual Run through its original Runtime acceptance."})
	if err != nil {
		return Contracts{}, err
	}
	contracts.Capability, err = portable.NewCapability(portable.CapabilityConfig{Reference: CapabilityReference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, contracts.List.Descriptor(), contracts.Create.Descriptor(), contracts.Update.Descriptor(), contracts.Delete.Descriptor(), contracts.RunNow.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return contracts, nil
}
