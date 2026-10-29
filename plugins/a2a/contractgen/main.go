package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

func main() {
	contract, err := define()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contract.Descriptor())
	}
	if err != nil {
		log.Fatal(err)
	}
}

func define() (*portable.Capability, error) {
	agents, err := qimumcp.NewMethod[struct{}, wire.Agents](qimumcp.MethodConfig{Name: "flame.a2a/agents.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List configured remote agents without contacting them."})
	if err != nil {
		return nil, err
	}
	send, err := qimumcp.NewMethod[wire.SendRequest, wire.Result](qimumcp.MethodConfig{Name: "flame.a2a/send.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Send one message up to 64 KiB to a configured remote agent. Return the remote task or message and the original requestId. A response is bounded to 2 MiB. Preserve the command identity: an unconfirmed send must never be resubmitted."})
	if err != nil {
		return nil, err
	}
	status, err := qimumcp.NewMethod[wire.TaskRequest, wire.Result](qimumcp.MethodConfig{Name: "flame.a2a/status.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read the remote task bound to the original requestId, using its captured endpoint and task identity. This does not replay a send or advance local task state."})
	if err != nil {
		return nil, err
	}
	cancel, err := qimumcp.NewMethod[wire.TaskRequest, wire.Result](qimumcp.MethodConfig{Name: "flame.a2a/cancel.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Request cancellation of the remote task bound to the original requestId. Only the returned remote state confirms cancellation; local interruption or transport failure does not. An unconfirmed cancel must never be resubmitted."})
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.a2a.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, agents.Descriptor(), send.Descriptor(), status.Descriptor(), cancel.Descriptor())
}
