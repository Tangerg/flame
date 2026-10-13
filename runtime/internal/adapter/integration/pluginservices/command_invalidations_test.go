package pluginservices

import (
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	schedulerpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
)

func TestCommandInvalidationsMatchConsumerContractInsteadOfReference(t *testing.T) {
	memory, err := memoryrpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	schedules, err := schedulerpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	var notices []invalidation.Notice
	observe, err := CommandInvalidations(func(notice invalidation.Notice) { notices = append(notices, notice) })
	if err != nil {
		t.Fatal(err)
	}
	observe(plugin.CommandBinding{Contract: memory.Capability.Identity()})
	observe(plugin.CommandBinding{Contract: schedules.Capability.Identity()})
	if len(notices) != 2 || notices[0].Resource != invalidation.AgentMemory || notices[1].Resource != invalidation.Schedules {
		t.Fatalf("consumer notices: %+v", notices)
	}
	changed := memory.Capability.Descriptor()
	changed.Methods[0].Target = "workspace"
	other, err := capability.FromDescriptor(changed)
	if err != nil {
		t.Fatal(err)
	}
	observe(plugin.CommandBinding{Contract: other.Identity()})
	if len(notices) != 2 {
		t.Fatal("a same-named different contract invalidated this consumer")
	}
}
