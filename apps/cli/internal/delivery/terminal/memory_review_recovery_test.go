package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
)

func TestPendingMemoryReviewKeepsItsDecisionBeforeTargetReads(t *testing.T) {
	store, err := workbench.OpenMemory(workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	limits, err := replay.NewCapability("original-store", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := mutation.NewReplayPolicy(limits, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	lost := &unconfirmedMemoryReview{agentMemoryServiceStub: &agentMemoryServiceStub{}, cancel: cancel}
	if err := workbench.ExecuteMemoryReview(ctx, lost, store, policy, terminalMemoryPendingID, memory.ReviewApprove); err == nil {
		t.Fatal("lost acknowledgement became success")
	}
	original := store.PendingInstalledCommands()[0]
	a := &app{workbench: store, agentMemory: &agentMemoryServiceStub{}}
	if err := a.PrepareAgentMemoryReview("project "+terminalMemoryPendingID[:len(terminalMemoryPendingID)-1], false); err == nil || !strings.Contains(err.Error(), "unconfirmed approve") {
		t.Fatalf("opposite review did not fence its original decision before current-target work: %v", err)
	}
	pending, found := store.PendingInstalledCommand(original.Intent)
	if !found || pending.CommandID != original.CommandID || !(pending.Request.Input == original.Request.Input) {
		t.Fatal("opposite review changed original identity or input")
	}
	if _, _, err := store.PendingMemoryReview(terminalMemoryPendingID, memory.ReviewApprove); err != nil {
		t.Fatal(err)
	}

}

type unconfirmedMemoryReview struct {
	*agentMemoryServiceStub
	cancel context.CancelFunc
}

func (c *unconfirmedMemoryReview) CallPlugin(context.Context, protocol.CallPluginRequest, replay.CommandID) (*protocol.CallPluginResponse, error) {
	c.cancel()
	return nil, conversation.ErrDisconnected
}
