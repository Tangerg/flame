package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/memory"
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
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	request, err := boundMemoryRequest(memory.ReviewV1Method, memory.ReviewV1Input{ID: terminalMemoryPendingID, Decision: string(memory.ReviewApprove)})
	if err != nil {
		t.Fatal(err)
	}
	original := workbench.PendingInstalledCommand{Intent: memoryReviewIntent(terminalMemoryPendingID), CommandID: mutation.NewCommandID(), Request: request, Replay: guard}
	if err := store.StageInstalledCommand(original); err != nil {
		t.Fatal(err)
	}
	a := &app{workbench: store, agentMemory: &agentMemoryServiceStub{}}
	if err := a.PrepareAgentMemoryReview("project "+terminalMemoryPendingID[:len(terminalMemoryPendingID)-1], false); err == nil || !strings.Contains(err.Error(), "unconfirmed approve") {
		t.Fatalf("opposite review did not fence its original decision before current-target work: %v", err)
	}
	pending, found := store.PendingInstalledCommand(original.Intent)
	if !found || pending.CommandID != original.CommandID || !bytes.Equal(pending.Request.Input, original.Request.Input) {
		t.Fatal("opposite review changed original identity or input")
	}
	if _, err := retainedMemoryReview(pending.Request, terminalMemoryPendingID, memory.ReviewApprove); err != nil {
		t.Fatal(err)
	}

	otherRequest, err := boundMemoryRequest(memory.ReviewV1Method, memory.ReviewV1Input{ID: terminalMemoryUserID, Decision: string(memory.ReviewReject)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StageInstalledCommand(workbench.PendingInstalledCommand{Intent: memoryReviewIntent(terminalMemoryUserID), CommandID: mutation.NewCommandID(), Request: otherRequest, Replay: guard}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.pendingMemoryItemCommand(memory.ReviewV1Method, memoryReviewIntent(""), terminalMemoryPendingID[:len(terminalMemoryPendingID)-1]); err == nil {
		t.Fatal("ambiguous pending prefix selected an original command")
	}
	exact, found, err := a.pendingMemoryItemCommand(memory.ReviewV1Method, memoryReviewIntent(""), terminalMemoryPendingID)
	if err != nil || !found || exact.CommandID != original.CommandID {
		t.Fatalf("exact pending identity lost precedence: found=%v error=%v", found, err)
	}
}

func TestPendingMemoryUpdateKeepsOriginalPatchAndOmission(t *testing.T) {
	content := "original patch"
	original := memory.UpdateV1Input{ID: terminalMemoryPendingID, Content: &content, Pinned: new(false)}
	request, err := boundMemoryRequest(memory.UpdateV1Method, original)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), request.Input...)
	if err := retainedMemoryUpdate(request, memory.UpdateV1Input{ID: original.ID, Content: new(content), Pinned: new(false)}); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []memory.UpdateV1Input{
		{ID: original.ID, Content: new("changed patch"), Pinned: new(false)},
		{ID: original.ID, Content: &content},
		{ID: original.ID, Content: &content, Pinned: new(true)},
	} {
		if err := retainedMemoryUpdate(request, changed); err == nil {
			t.Fatal("changed patch replaced an unconfirmed original update")
		}
	}
	if !bytes.Equal(before, request.Input) {
		t.Fatal("checking a pending update changed original bytes")
	}
}
