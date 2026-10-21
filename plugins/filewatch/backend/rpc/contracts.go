package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/filewatch/backend/cmd/contracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
)

type ConfigureInput struct {
	ExpectedRevision int64            `json:"expectedRevision" jsonschema:"minimum=1"`
	Trigger          *filewatch.Draft `json:"trigger,omitzero"`
}

type EventInput struct {
	ID string `json:"id" jsonschema:"minLength=1,maxLength=256"`
}

type Contracts struct {
	Capability *capability.Contract
	Get        *pluginsdk.Method[struct{}, filewatch.Snapshot]
	Configure  *pluginsdk.Method[ConfigureInput, filewatch.Snapshot]
	Status     *pluginsdk.Method[struct{}, filewatch.Status]
	Events     *pluginsdk.Method[struct{}, []filewatch.Header]
	Event      *pluginsdk.Method[EventInput, filewatch.Event]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Get, err = pluginsdk.NewMethod[struct{}, filewatch.Snapshot](pluginsdk.MethodConfig{Name: "flame.filewatch/get.v1", Kind: capability.Query, Target: "installation", Description: "Read this installation's single persistent file trigger and configuration revision."})
	if err != nil {
		return Contracts{}, err
	}
	c.Configure, err = pluginsdk.NewMethod[ConfigureInput, filewatch.Snapshot](pluginsdk.MethodConfig{Name: "flame.filewatch/configure.v1", Kind: capability.Mutation, Target: "installation", Description: "Replace or rearm the trigger at its current revision; omitting trigger disables observation. Configuration acceptance does not claim a live watch. Captured work remains immutable."})
	if err != nil {
		return Contracts{}, err
	}
	c.Status, err = pluginsdk.NewMethod[struct{}, filewatch.Status](pluginsdk.MethodConfig{Name: "flame.filewatch/status.v1", Kind: capability.Query, Target: "installation", Description: "Inspect the actual background observation revision and failures. A failed watch requires explicit configuration rearm; accepted dispatch intents continue reconciliation."})
	if err != nil {
		return Contracts{}, err
	}
	c.Events, err = pluginsdk.NewMethod[struct{}, []filewatch.Header](pluginsdk.MethodConfig{Name: "flame.filewatch/events.v1", Kind: capability.Query, Target: "installation", Description: "Read at most 256 retained observation headers, newest first. Accepted means core confirmed the captured Run opening, not that execution completed. Pending intents are never pruned."})
	if err != nil {
		return Contracts{}, err
	}
	c.Event, err = pluginsdk.NewMethod[EventInput, filewatch.Event](pluginsdk.MethodConfig{Name: "flame.filewatch/event.v1", Kind: capability.Query, Target: "installation", Description: "Read one retained observation and its complete original dispatch capture."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.filewatch.v1", Composition: capability.Collection, Scope: "installation"}, c.Get.Descriptor(), c.Configure.Descriptor(), c.Status.Descriptor(), c.Events.Descriptor(), c.Event.Descriptor())
	return c, err
}

func Classify(err error) string {
	switch {
	case errors.Is(err, filewatch.ErrInvalid):
		return "invalid_trigger"
	case errors.Is(err, filewatch.ErrRevision):
		return "revision_conflict"
	case errors.Is(err, filewatch.ErrNotFound):
		return "observation_not_found"
	case errors.Is(err, idempotency.ErrKeyConflict):
		return "command_conflict"
	default:
		return ""
	}
}
