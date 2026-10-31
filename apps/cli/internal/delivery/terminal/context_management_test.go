package terminal

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/oolong/core/input"
)

const (
	terminalMemoryPendingID = "mem_00000000000000000000000000000001"
	terminalMemoryUserID    = "mem_00000000000000000000000000000002"
	terminalMemoryAddedID   = "mem_00000000000000000000000000000003"
)

type agentMemoryServiceStub struct {
	mu      sync.Mutex
	project []memory.Item
	user    []memory.Item
	added   chan string
	review  chan memorycontract.ReviewInputDecision
}

type blockingAgentMemoryReviewService struct {
	AgentMemory
	started  chan memorycontract.ReviewInputDecision
	release  chan struct{}
	canceled chan struct{}
}

type blockingAgentMemoryUpdateService struct {
	AgentMemory
	started  chan memorycontract.UpdateInput
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingAgentMemoryUpdateService) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID authoring.CommandID) (*protocol.CallPluginResponse, error) {
	if request.Method != memorycontract.UpdateV2Method {
		return b.AgentMemory.CallPlugin(ctx, request, commandID)
	}
	var input memorycontract.UpdateInput
	if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
		return nil, err
	}
	b.started <- input
	select {
	case <-b.release:
		return b.AgentMemory.CallPlugin(ctx, request, commandID)
	case <-ctx.Done():
		close(b.canceled)
		return nil, context.Cause(ctx)
	}
}

func (b *blockingAgentMemoryReviewService) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID authoring.CommandID) (*protocol.CallPluginResponse, error) {
	if request.Method != memorycontract.ReviewV2Method {
		return b.AgentMemory.CallPlugin(ctx, request, commandID)
	}
	var input memorycontract.ReviewInput
	if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
		return nil, err
	}
	b.started <- input.Decision
	select {
	case <-b.release:
		return b.AgentMemory.CallPlugin(ctx, request, commandID)
	case <-ctx.Done():
		close(b.canceled)
		return nil, context.Cause(ctx)
	}
}

func newAgentMemoryServiceStub() *agentMemoryServiceStub {
	now := time.Now()
	return &agentMemoryServiceStub{
		project: []memory.Item{{
			Revision: 1, ID: terminalMemoryPendingID, Scope: memorycontract.ItemSnapshotScopeProject, Content: "confirm release steps",
			Origin: memorycontract.ItemSnapshotOriginAuto, Status: memorycontract.ItemSnapshotStatusPending, SessionID: "ses_origin",
			Day: "2026-08-12", CreatedAt: now, UpdatedAt: now,
		}},
		user: []memory.Item{{
			Revision: 1, ID: terminalMemoryUserID, Scope: memorycontract.ItemSnapshotScopeUser, Content: "prefer concise answers",
			Origin: memorycontract.ItemSnapshotOriginUser, Status: memorycontract.ItemSnapshotStatusActive, Pinned: true,
			CreatedAt: now, UpdatedAt: now,
		}},
		added: make(chan string, 1), review: make(chan memorycontract.ReviewInputDecision, 1),
	}
}

func (a *agentMemoryServiceStub) Items(_ context.Context, target conversation.MemoryTarget) ([]memory.Item, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if target.Scope == memorycontract.ItemSnapshotScopeUser {
		return append([]memory.Item(nil), a.user...), nil
	}
	return append([]memory.Item(nil), a.project...), nil
}

func (a *agentMemoryServiceStub) reviewItem(_ context.Context, id string, expectedRevision uint64, decision memorycontract.ReviewInputDecision) error {
	if decision != memorycontract.ReviewInputDecisionApprove && decision != memorycontract.ReviewInputDecisionReject {
		return errors.New("invalid review decision")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for index := range a.project {
		if a.project[index].ID != id {
			continue
		}
		if a.project[index].Status != memorycontract.ItemSnapshotStatusPending {
			return errors.New("not pending")
		}
		if decision == memorycontract.ReviewInputDecisionReject {
			a.project = append(a.project[:index], a.project[index+1:]...)
		} else {
			a.project[index].Status = memorycontract.ItemSnapshotStatusActive
			a.project[index].UpdatedAt = time.Now()
		}
		a.review <- decision
		return nil
	}
	return errors.New("not found")
}

func (a *agentMemoryServiceStub) updateItem(_ context.Context, request memorycontract.UpdateInput) (memory.Item, error) {
	if request.Content == nil && request.Pinned == nil {
		return memory.Item{}, errors.New("update requires a change")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, items := range []*[]memory.Item{&a.project, &a.user} {
		for index := range *items {
			item := &(*items)[index]
			if item.ID != request.ID {
				continue
			}
			if request.Content != nil {
				item.Content = *request.Content
			}
			if request.Pinned != nil {
				item.Pinned = *request.Pinned
			}
			item.UpdatedAt = time.Now()
			return *item, nil
		}
	}
	return memory.Item{}, errors.New("not found")
}

func (*agentMemoryServiceStub) PrepareAdd(_ context.Context, input memorycontract.AddInput) (protocol.CallPluginRequest, error) {
	return boundMemoryRequest(memorycontract.AddV2Method, input)
}
func (*agentMemoryServiceStub) DecodeAdd(reply *protocol.CallPluginResponse) (memorycontract.AddResult, error) {
	var result memorycontract.AddResult
	err := json.Unmarshal([]byte(reply.Output), &result)
	return result, err
}
func (*agentMemoryServiceStub) PrepareUpdate(_ context.Context, input memorycontract.UpdateInput) (protocol.CallPluginRequest, error) {
	return boundMemoryRequest(memorycontract.UpdateV2Method, input)
}
func (*agentMemoryServiceStub) DecodeUpdate(id string, reply *protocol.CallPluginResponse) (memory.Item, error) {
	var row memorycontract.ItemSnapshot
	if err := json.Unmarshal([]byte(reply.Output), &row); err != nil {
		return memory.Item{}, err
	}
	if string(row.ID) != id {
		return memory.Item{}, errors.New("wrong update item")
	}
	return memory.Item{ID: string(row.ID), Revision: row.Revision, Scope: row.Scope, Content: row.Content, Origin: row.Origin, Status: row.Status, Pinned: row.Pinned, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
func (*agentMemoryServiceStub) PrepareReview(_ context.Context, id string, expectedRevision uint64, decision memorycontract.ReviewInputDecision) (protocol.CallPluginRequest, error) {
	return boundMemoryRequest(memorycontract.ReviewV2Method, memorycontract.ReviewInput{ID: id, ExpectedRevision: expectedRevision, Decision: decision})
}
func (*agentMemoryServiceStub) PrepareDelete(_ context.Context, id string, expectedRevision uint64) (protocol.CallPluginRequest, error) {
	return boundMemoryRequest(memorycontract.DeleteV2Method, memorycontract.ItemInput{ID: id, ExpectedRevision: expectedRevision})
}
func boundMemoryRequest(method string, value any) (protocol.CallPluginRequest, error) {
	input, err := jsonvalue.Encode(value)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	const installation = "12345678-1234-1234-1234-123456789abc"
	return protocol.CallPluginRequest{InstallationID: installation, Digest: strings.Repeat("1", 64), Contribution: "memory-provider", Contract: memorycontract.CapabilityJSON, Method: method, Target: portable.Target{Scope: "installation", Reference: installation}, Input: string(input.JSON())}, nil
}

func (a *agentMemoryServiceStub) CallPlugin(_ context.Context, request protocol.CallPluginRequest, commandID authoring.CommandID) (*protocol.CallPluginResponse, error) {
	if (request.Method != memorycontract.DeleteV2Method && request.Method != memorycontract.ReviewV2Method && request.Method != memorycontract.UpdateV2Method && request.Method != memorycontract.AddV2Method) || commandID == "" {
		return nil, errors.New("invalid memory command")
	}
	if request.Method == memorycontract.AddV2Method {
		var input memorycontract.AddInput
		if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
			return nil, err
		}
		target, err := conversation.NewMemoryTarget(memorycontract.ItemSnapshotScope(input.Scope), input.CWD)
		if err != nil {
			return nil, err
		}
		saved, err := a.addItem(context.Background(), target, input.Content)
		if err != nil {
			return nil, err
		}
		output := memorycontract.AddResult{Item: memorycontract.ItemSnapshot{Revision: saved.Revision, ID: memorycontract.ItemID(saved.ID), Scope: saved.Scope, Content: saved.Content, Origin: saved.Origin, Status: saved.Status, Pinned: saved.Pinned, CreatedAt: saved.CreatedAt, UpdatedAt: saved.UpdatedAt, Embedding: []float64{}}}
		encoded, err := jsonvalue.Encode(output)
		return &protocol.CallPluginResponse{Output: string(encoded.JSON())}, err
	}
	if request.Method == memorycontract.UpdateV2Method {
		var input memorycontract.UpdateInput
		if err := json.Unmarshal([]byte(request.Input), &input, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		saved, err := a.updateItem(context.Background(), input)
		if err != nil {
			return nil, err
		}
		output := memorycontract.ItemSnapshot{Revision: saved.Revision, ID: memorycontract.ItemID(saved.ID), Scope: saved.Scope, Content: saved.Content, Origin: saved.Origin, Status: saved.Status, Pinned: saved.Pinned, CreatedAt: saved.CreatedAt, UpdatedAt: saved.UpdatedAt, Embedding: []float64{}}
		encoded, err := jsonvalue.Encode(output)
		return &protocol.CallPluginResponse{Output: string(encoded.JSON())}, err
	}
	if request.Method == memorycontract.ReviewV2Method {
		var input memorycontract.ReviewInput
		if err := json.Unmarshal([]byte(request.Input), &input, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		if err := a.reviewItem(context.Background(), input.ID, input.ExpectedRevision, input.Decision); err != nil {
			return nil, err
		}
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	}
	var input memorycontract.ItemInput
	if err := json.Unmarshal([]byte(request.Input), &input, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if err := a.delete(input.ID); err != nil {
		return nil, err
	}
	return &protocol.CallPluginResponse{Output: `{}`}, nil
}

func (a *agentMemoryServiceStub) delete(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, items := range []*[]memory.Item{&a.project, &a.user} {
		for index := range *items {
			if (*items)[index].ID == id {
				*items = append((*items)[:index], (*items)[index+1:]...)
				return nil
			}
		}
	}
	return errors.New("not found")
}

func (a *agentMemoryServiceStub) addItem(_ context.Context, target conversation.MemoryTarget, content string) (memory.Item, error) {
	if err := target.Validate(); err != nil {
		return memory.Item{}, err
	}
	now := time.Now()
	item := memory.Item{
		ID: terminalMemoryAddedID, Scope: target.Scope, Content: content, Origin: memorycontract.ItemSnapshotOriginUser,
		Status: memorycontract.ItemSnapshotStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	a.mu.Lock()
	if target.Scope == memorycontract.ItemSnapshotScopeUser {
		a.user = append(a.user, item)
	} else {
		a.project = append(a.project, item)
	}
	a.mu.Unlock()
	a.added <- content
	return item, nil
}

func TestAgentMemoryReaderShowsScopeAndProvenance(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/memory project")
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · project")
	host.Shows(t, "session  ses_origin")
	stop()
}

func TestAgentMemoryMultilineAddSurvivesResize(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/memory-add user")
	host.Press(input.Enter)
	host.Shows(t, "Add user memory")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory editor did not survive a minimal viewport")
	}
	host.Shows(t, "Add user memory")
	host.Type("first line")
	host.Press(input.Enter)
	host.Type("second line")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, memory.added, "agent memory add"); got != "first line\nsecond line" {
		t.Fatalf("added content = %q", got)
	}
	host.Shows(t, "Agent memory · user")
	host.Shows(t, "first line")
	stop()
}

func TestAgentMemoryEditorKeepsTheDraftAcrossARejectedSave(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/memory-add user")
	host.Press(input.Enter)
	host.Shows(t, "Add user memory")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "memory content is empty")

	host.Type("durable fact")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, memory.added, "agent memory add"); got != "durable fact" {
		t.Fatalf("added content = %q", got)
	}
	host.Hides(t, "memory content is empty")
	stop()
}

func TestPendingAgentMemoryReviewRequiresResizeSafeConfirmation(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/memory-approve project " + terminalMemoryPendingID)
	host.Press(input.Enter)
	host.Shows(t, "Approve agent memory")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory confirmation did not survive a minimal viewport")
	}
	host.Shows(t, "Approve agent memory")
	host.Press(input.Down)
	host.Press(input.Enter)
	if got := awaitValue(t, memory.review, "agent memory review"); got != memorycontract.ReviewInputDecisionApprove {
		t.Fatalf("review = %q", got)
	}
	host.Shows(t, "Agent memory · project")
	host.Shows(t, "active")
	stop()
}

func TestAgentMemoryReviewOutlivesSameSessionProjectionReplacement(t *testing.T) {
	backend := runtimefixture.New()
	base := newAgentMemoryServiceStub()
	memory := &blockingAgentMemoryReviewService{
		AgentMemory: base, started: make(chan memorycontract.ReviewInputDecision, 1),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(memory.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: backend, AgentMemory: memory, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/memory-approve project " + terminalMemoryPendingID)
	host.Press(input.Enter)
	host.Shows(t, "Approve agent memory")
	host.Press(input.Down)
	host.Press(input.Enter)
	if decision := awaitValue(t, memory.started, "agent memory review"); decision != memorycontract.ReviewInputDecisionApprove {
		t.Fatalf("review decision = %q", decision)
	}
	if _, err := backend.RollbackSession(t.Context(), conversation.RollbackSession{
		SessionID: "ses_demo_1", Scope: protocol.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: protocol.RuntimeSessionsChanged, Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	select {
	case <-memory.canceled:
		t.Fatal("session projection replacement canceled the agent memory review")
	default:
	}
	release()
	if decision := awaitValue(t, base.review, "committed agent memory review"); decision != memorycontract.ReviewInputDecisionApprove {
		t.Fatalf("committed review decision = %q", decision)
	}
	items, err := base.Items(t.Context(), conversation.MemoryTarget{Scope: memorycontract.ItemSnapshotScopeProject, Workspace: "/tmp/demo/store"})
	if err != nil || len(items) != 1 || items[0].Status != memorycontract.ItemSnapshotStatusActive {
		t.Fatalf("project memory after review = (%+v, %v)", items, err)
	}
	stop()
}

func TestAgentMemoryUpdateDoesNotInstallAReaderAfterSessionSwitch(t *testing.T) {
	base := newAgentMemoryServiceStub()
	memory := &blockingAgentMemoryUpdateService{
		AgentMemory: base, started: make(chan memorycontract.UpdateInput, 1),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(memory.release) })
	t.Cleanup(release)
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), AgentMemory: memory, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	host.Type("/memory-unpin user " + terminalMemoryUserID)
	host.Press(input.Enter)
	patch := awaitValue(t, memory.started, "agent memory update")
	if patch.ID != terminalMemoryUserID || patch.Pinned == nil || *patch.Pinned {
		t.Fatalf("agent memory patch = %+v", patch)
	}
	host.Type("/new")
	host.Press(input.Enter)
	host.Shows(t, "session · Untitled session")
	select {
	case <-memory.canceled:
		t.Fatal("session switch canceled the agent memory update")
	default:
	}
	release()
	host.Shows(t, "agent memory updated · "+terminalMemoryUserID)
	host.Hides(t, "Agent memory · user")
	items, err := base.Items(t.Context(), conversation.MemoryTarget{Scope: memorycontract.ItemSnapshotScopeUser})
	if err != nil || len(items) != 1 || items[0].Pinned {
		t.Fatalf("user memory after update = (%+v, %v)", items, err)
	}
	stop()
}

func TestAgentMemoryEditPinAndDeleteRoundTripThroughAuthoritativeReads(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")

	host.Type("/memory-edit user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "Edit agent memory · "+terminalMemoryUserID)
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("prefer explicit answers")
	host.Press(input.Enter)
	host.Type("with evidence")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory editor did not survive a minimal viewport")
	}
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "Agent memory · user")
	host.Shows(t, "prefer explicit answers")
	items, err := memory.Items(t.Context(), conversation.MemoryTarget{Scope: memorycontract.ItemSnapshotScopeUser})
	if err != nil || len(items) != 1 || items[0].Content != "prefer explicit answers\nwith evidence" {
		t.Fatalf("edited user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/memory-unpin user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · user")
	items, err = memory.Items(t.Context(), conversation.MemoryTarget{Scope: memorycontract.ItemSnapshotScopeUser})
	if err != nil || len(items) != 1 || items[0].Pinned {
		t.Fatalf("unpinned user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/memory-pin user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "pinned")
	items, err = memory.Items(t.Context(), conversation.MemoryTarget{Scope: memorycontract.ItemSnapshotScopeUser})
	if err != nil || len(items) != 1 || !items[0].Pinned {
		t.Fatalf("pinned user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/memory-delete user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "Delete agent memory")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("agent memory deletion did not survive a minimal viewport")
	}
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "No active or pending memory")
	items, err = memory.Items(t.Context(), conversation.MemoryTarget{Scope: memorycontract.ItemSnapshotScopeUser})
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted user memory = (%+v, %v)", items, err)
	}
	stop()
}
