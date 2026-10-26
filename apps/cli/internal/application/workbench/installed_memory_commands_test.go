package workbench_test

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const memoryID = "01980000-0000-7000-8000-000000000001"

type memoryCommands struct {
	callerFunc
	preparations int
}

func (c *memoryCommands) prepare(method string, value any) (protocol.CallPluginRequest, error) {
	c.preparations++
	body, err := json.Marshal(value)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	const installation = "12345678-1234-1234-1234-123456789abc"
	return protocol.CallPluginRequest{InstallationID: installation, Digest: strings.Repeat("1", 64), Contribution: "memory-provider", Contract: memory.CapabilityJSON, Method: method, Target: extension.Target{Scope: "installation", Reference: installation}, Input: string(body)}, nil
}

func (c *memoryCommands) PrepareAdd(_ context.Context, input memory.AddV1Input) (protocol.CallPluginRequest, error) {
	return c.prepare(memory.AddV1Method, input)
}
func (*memoryCommands) DecodeAdd(reply *protocol.CallPluginResponse) (memory.AddV1Output, error) {
	var result memory.AddV1Output
	err := json.Unmarshal([]byte(reply.Output), &result, json.RejectUnknownMembers(true))
	return result, err
}
func (c *memoryCommands) PrepareUpdate(_ context.Context, input memory.UpdateV1Input) (protocol.CallPluginRequest, error) {
	return c.prepare(memory.UpdateV1Method, input)
}
func (*memoryCommands) DecodeUpdate(id string, _ *protocol.CallPluginResponse) (memory.Item, error) {
	return memory.Item{ID: id}, nil
}
func (c *memoryCommands) PrepareReview(_ context.Context, id string, decision memory.ReviewDecision) (protocol.CallPluginRequest, error) {
	return c.prepare(memory.ReviewV1Method, memory.ReviewV1Input{ID: id, Decision: string(decision)})
}
func (c *memoryCommands) PrepareDelete(_ context.Context, id string) (protocol.CallPluginRequest, error) {
	return c.prepare(memory.DeleteV1Method, memory.DeleteV1Input{ID: id})
}

func TestMemoryPendingAdmissionRunsBeforePreparationOrDispatch(t *testing.T) {
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	replayPolicy := policy(t, "store", &now)
	for _, operation := range []string{"addition", "update", "review"} {
		t.Run(operation, func(t *testing.T) {
			storage := &persistence{files: make(map[string][]byte)}
			store, err := workbench.Open(storage, workbench.Config{})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			caller := &memoryCommands{callerFunc: func(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error) {
				calls++
				return nil, protocol.ErrPluginUnavailable
			}}
			addition := memory.AddV1Input{Scope: "project", CWD: "/original", Content: "original addition"}
			update := memory.UpdateV1Input{ID: memoryID, Content: new("original patch"), Pinned: new(false)}
			execute := func() error {
				switch operation {
				case "addition":
					_, err := workbench.ExecuteMemoryAddition(t.Context(), caller, store, replayPolicy, addition)
					return err
				case "update":
					_, err := workbench.ExecuteMemoryUpdate(t.Context(), caller, store, replayPolicy, update)
					return err
				default:
					return workbench.ExecuteMemoryReview(t.Context(), caller, store, replayPolicy, memoryID, memory.ReviewApprove)
				}
			}
			if err := execute(); err == nil || calls != 1 || caller.preparations != 1 {
				t.Fatalf("unavailable owner lost its original intent: calls=%d preparations=%d error=%v", calls, caller.preparations, err)
			}
			original := store.PendingInstalledCommands()[0]
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = workbench.Open(storage, workbench.Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			var changes []func() error
			switch operation {
			case "addition":
				for _, changed := range []memory.AddV1Input{
					{Scope: "user", Content: addition.Content},
					{Scope: addition.Scope, CWD: "/replacement", Content: addition.Content},
					{Scope: addition.Scope, CWD: addition.CWD, Content: "changed"},
				} {
					changes = append(changes, func() error {
						_, err := workbench.ExecuteMemoryAddition(t.Context(), caller, store, replayPolicy, changed)
						return err
					})
				}
			case "update":
				for _, changed := range []memory.UpdateV1Input{
					{ID: memoryID, Content: new("changed"), Pinned: new(false)},
					{ID: memoryID, Content: new(*update.Content)},
					{ID: memoryID, Content: new(*update.Content), Pinned: new(true)},
				} {
					changes = append(changes, func() error {
						_, err := workbench.ExecuteMemoryUpdate(t.Context(), caller, store, replayPolicy, changed)
						return err
					})
				}
			case "review":
				changes = append(changes, func() error {
					return workbench.ExecuteMemoryReview(t.Context(), caller, store, replayPolicy, memoryID, memory.ReviewReject)
				})
			}
			for _, change := range changes {
				if err := change(); err == nil || strings.Contains(err.Error(), "/memory-") {
					t.Fatalf("changed intent was admitted or returned terminal instructions: %v", err)
				}
				if calls != 1 || caller.preparations != 1 || !reflect.DeepEqual(store.PendingInstalledCommands(), []workbench.PendingInstalledCommand{original}) {
					t.Fatal("changed intent prepared, dispatched or replaced its original journal")
				}
			}
			if err := execute(); err == nil || calls != 2 || caller.preparations != 1 {
				t.Fatalf("original intent did not resume its captured owner: %v", err)
			}
		})
	}
}

func TestPendingMemoryReadoutsKeepExactIdentityAndDecision(t *testing.T) {
	store, err := workbench.OpenMemory(workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	caller := &memoryCommands{callerFunc: func(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error) {
		return nil, protocol.ErrPluginUnavailable
	}}
	for _, id := range []string{memoryID, "01980000-0000-7000-8000-000000000002"} {
		if err := workbench.ExecuteMemoryReview(t.Context(), caller, store, policy(t, "store", &now), id, memory.ReviewApprove); err == nil {
			t.Fatal("unavailable owner became success")
		}
	}
	if _, _, err := store.PendingMemoryReview(memoryID[:len(memoryID)-1], memory.ReviewApprove); err == nil {
		t.Fatal("ambiguous prefix selected an original intent")
	}
	original, found, err := store.PendingMemoryReview(memoryID, memory.ReviewReject)
	if !found || err == nil || original.ID != memoryID || original.Decision != "approve" {
		t.Fatalf("original review decision was lost: %+v found=%v error=%v", original, found, err)
	}
}

func TestMemoryAdditionScopeMustMatchBeforeRetirement(t *testing.T) {
	store, err := workbench.OpenMemory(workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	caller := &memoryCommands{callerFunc: func(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error) {
		return &protocol.CallPluginResponse{Output: `{"item":{"scope":"user"}}`}, nil
	}}
	_, err = workbench.ExecuteMemoryAddition(t.Context(), caller, store, policy(t, "store", &now), memory.AddV1Input{Scope: "project", CWD: "/original", Content: "fact"})
	if err == nil || !errors.Is(err, authoring.ErrCommandOutcomeUnknown) || len(store.PendingInstalledCommands()) != 1 {
		t.Fatalf("changed scope retired the original addition: %v", err)
	}
}
