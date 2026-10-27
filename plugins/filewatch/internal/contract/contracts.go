package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/filewatch/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type ConfigureInput struct {
	ExpectedRevision int64            `json:"expectedRevision" jsonschema:"minimum=1"`
	Trigger          *filewatch.Draft `json:"trigger,omitzero"`
}

type EventInput struct {
	ID string `json:"id" jsonschema:"minLength=1,maxLength=256"`
}

type Contracts struct {
	Capability *portable.Capability
	Get        *qimumcp.Method[struct{}, filewatch.Snapshot]
	Configure  *qimumcp.Method[ConfigureInput, filewatch.Snapshot]
	Status     *qimumcp.Method[struct{}, filewatch.Status]
	Events     *qimumcp.Method[struct{}, []filewatch.Header]
	Event      *qimumcp.Method[EventInput, filewatch.Event]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = qimumcp.NewMethod[struct{}, filewatch.Snapshot](qimumcp.MethodConfig{Name: "flame.filewatch/get.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read this installation's single persistent file trigger and configuration revision."})
	if err != nil {
		return Contracts{}, err
	}
	c.Configure, err = qimumcp.NewMethod[ConfigureInput, filewatch.Snapshot](qimumcp.MethodConfig{Name: "flame.filewatch/configure.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Replace or rearm the trigger at its current revision; omitting trigger disables observation. Configuration acceptance does not claim a live watch. Captured work remains immutable."})
	if err != nil {
		return Contracts{}, err
	}
	c.Status, err = qimumcp.NewMethod[struct{}, filewatch.Status](qimumcp.MethodConfig{Name: "flame.filewatch/status.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Inspect the actual background observation revision and failures. A failed watch requires explicit configuration rearm; accepted dispatch intents continue reconciliation."})
	if err != nil {
		return Contracts{}, err
	}
	c.Events, err = qimumcp.NewMethod[struct{}, []filewatch.Header](qimumcp.MethodConfig{Name: "flame.filewatch/events.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read at most 256 retained observation headers, newest first. Accepted means core confirmed the captured Run opening, not that execution completed. Pending intents are never pruned."})
	if err != nil {
		return Contracts{}, err
	}
	c.Event, err = qimumcp.NewMethod[EventInput, filewatch.Event](qimumcp.MethodConfig{Name: "flame.filewatch/event.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read one retained observation and its complete original dispatch capture."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = portable.NewCapability(portable.CapabilityConfig{Reference: "flame.filewatch.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, c.Get.Descriptor(), c.Configure.Descriptor(), c.Status.Descriptor(), c.Events.Descriptor(), c.Event.Descriptor())
	return c, err
}
