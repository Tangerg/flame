package terminal

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	memorycontract "github.com/Tangerg/flame/cli/internal/domain/memory"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"
)

const (
	terminalMemoryPendingID = "mem_00000000000000000000000000000001"
	terminalMemoryUserID    = "mem_00000000000000000000000000000002"
	terminalMemoryAddedID   = "mem_00000000000000000000000000000003"
)

type agentMemoryServiceStub struct {
	mu      sync.Mutex
	project []protocol.AgentMemoryItem
	user    []protocol.AgentMemoryItem
	added   chan string
	review  chan memorycontract.ReviewDecision
}

type blockingAgentMemoryReviewService struct {
	AgentMemory
	started  chan memorycontract.ReviewDecision
	release  chan struct{}
	canceled chan struct{}
}

type blockingAgentMemoryUpdateService struct {
	AgentMemory
	started  chan memorycontract.UpdateV1Input
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingAgentMemoryUpdateService) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID replay.CommandID) (*protocol.CallPluginResponse, error) {
	if request.Method != memorycontract.UpdateV1Method {
		return b.AgentMemory.CallPlugin(ctx, request, commandID)
	}
	var input memorycontract.UpdateV1Input
	if err := json.Unmarshal(request.Input, &input); err != nil {
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

func (b *blockingAgentMemoryReviewService) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID replay.CommandID) (*protocol.CallPluginResponse, error) {
	if request.Method != memorycontract.ReviewV1Method {
		return b.AgentMemory.CallPlugin(ctx, request, commandID)
	}
	var input memorycontract.ReviewV1Input
	if err := json.Unmarshal(request.Input, &input); err != nil {
		return nil, err
	}
	b.started <- memorycontract.ReviewDecision(input.Decision)
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
		project: []protocol.AgentMemoryItem{{
			ID: terminalMemoryPendingID, Scope: protocol.AgentMemoryScopeProject, Content: "confirm release steps",
			Origin: protocol.AgentMemoryOriginAuto, Status: protocol.AgentMemoryStatusPending, SessionID: "ses_origin",
			Day: "2026-08-12", CreatedAt: now, UpdatedAt: now,
		}},
		user: []protocol.AgentMemoryItem{{
			ID: terminalMemoryUserID, Scope: protocol.AgentMemoryScopeUser, Content: "prefer concise answers",
			Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive, Pinned: true,
			CreatedAt: now, UpdatedAt: now,
		}},
		added: make(chan string, 1), review: make(chan memorycontract.ReviewDecision, 1),
	}
}

func (a *agentMemoryServiceStub) Items(_ context.Context, target conversation.MemoryTarget) ([]protocol.AgentMemoryItem, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if target.Scope == protocol.AgentMemoryScopeUser {
		return append([]protocol.AgentMemoryItem(nil), a.user...), nil
	}
	return append([]protocol.AgentMemoryItem(nil), a.project...), nil
}

func (a *agentMemoryServiceStub) reviewItem(_ context.Context, id string, decision memorycontract.ReviewDecision) error {
	if decision != memorycontract.ReviewApprove && decision != memorycontract.ReviewReject {
		return errors.New("invalid review decision")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for index := range a.project {
		if a.project[index].ID != id {
			continue
		}
		if a.project[index].Status != protocol.AgentMemoryStatusPending {
			return errors.New("not pending")
		}
		if decision == memorycontract.ReviewReject {
			a.project = append(a.project[:index], a.project[index+1:]...)
		} else {
			a.project[index].Status = protocol.AgentMemoryStatusActive
			a.project[index].UpdatedAt = time.Now()
		}
		a.review <- decision
		return nil
	}
	return errors.New("not found")
}

func (a *agentMemoryServiceStub) updateItem(_ context.Context, request memorycontract.UpdateV1Input) (protocol.AgentMemoryItem, error) {
	if request.Content == nil && request.Pinned == nil {
		return protocol.AgentMemoryItem{}, errors.New("update requires a change")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, items := range []*[]protocol.AgentMemoryItem{&a.project, &a.user} {
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
	return protocol.AgentMemoryItem{}, errors.New("not found")
}

func (*agentMemoryServiceStub) PrepareUpdate(_ context.Context, input memorycontract.UpdateV1Input) (protocol.CallPluginRequest, error) {
	return boundMemoryRequest(memorycontract.UpdateV1Method, input)
}
func (*agentMemoryServiceStub) DecodeUpdate(id string, reply *protocol.CallPluginResponse) (protocol.AgentMemoryItem, error) {
	var row memorycontract.UpdateV1Output
	if err := json.Unmarshal(reply.Output, &row); err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	if row.ID != id {
		return protocol.AgentMemoryItem{}, errors.New("wrong update item")
	}
	return protocol.AgentMemoryItem{ID: row.ID, Scope: protocol.AgentMemoryScope(row.Scope), Content: row.Content, Origin: protocol.AgentMemoryOrigin(row.Origin), Status: protocol.AgentMemoryStatus(row.Status), Pinned: row.Pinned, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
func (*agentMemoryServiceStub) PrepareReview(_ context.Context, id string, decision memorycontract.ReviewDecision) (protocol.CallPluginRequest, error) {
	return boundMemoryRequest(memorycontract.ReviewV1Method, memorycontract.ReviewV1Input{ID: id, Decision: string(decision)})
}
func (*agentMemoryServiceStub) PrepareDelete(_ context.Context, id string) (protocol.CallPluginRequest, error) {
	return boundMemoryRequest(memorycontract.DeleteV1Method, memorycontract.DeleteV1Input{ID: id})
}
func boundMemoryRequest(method string, value any) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(value)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	const installation = "12345678-1234-1234-1234-123456789abc"
	return protocol.CallPluginRequest{InstallationID: installation, Digest: strings.Repeat("1", 64), Contribution: "memory-provider", Contract: memorycontract.CapabilityJSON, Method: method, Target: capability.Target{Scope: "installation", Reference: installation}, Input: input.JSON()}, nil
}

func (a *agentMemoryServiceStub) CallPlugin(_ context.Context, request protocol.CallPluginRequest, commandID replay.CommandID) (*protocol.CallPluginResponse, error) {
	if (request.Method != memorycontract.DeleteV1Method && request.Method != memorycontract.ReviewV1Method && request.Method != memorycontract.UpdateV1Method) || commandID == "" {
		return nil, errors.New("invalid memory command")
	}
	if request.Method == memorycontract.UpdateV1Method {
		var input memorycontract.UpdateV1Input
		if err := json.Unmarshal(request.Input, &input, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		saved, err := a.updateItem(context.Background(), input)
		if err != nil {
			return nil, err
		}
		output := memorycontract.UpdateV1Output{ID: saved.ID, Scope: string(saved.Scope), Content: saved.Content, Origin: string(saved.Origin), Status: string(saved.Status), Pinned: saved.Pinned, CreatedAt: saved.CreatedAt, UpdatedAt: saved.UpdatedAt, Embedding: []float64{}}
		encoded, err := jsoninput.Encode(output)
		return &protocol.CallPluginResponse{Output: encoded.JSON()}, err
	}
	if request.Method == memorycontract.ReviewV1Method {
		var input memorycontract.ReviewV1Input
		if err := json.Unmarshal(request.Input, &input, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		if err := a.reviewItem(context.Background(), input.ID, memorycontract.ReviewDecision(input.Decision)); err != nil {
			return nil, err
		}
		return &protocol.CallPluginResponse{Output: []byte(`{}`)}, nil
	}
	var input memorycontract.DeleteV1Input
	if err := json.Unmarshal(request.Input, &input, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if err := a.delete(input.ID); err != nil {
		return nil, err
	}
	return &protocol.CallPluginResponse{Output: []byte(`{}`)}, nil
}

func (a *agentMemoryServiceStub) delete(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, items := range []*[]protocol.AgentMemoryItem{&a.project, &a.user} {
		for index := range *items {
			if (*items)[index].ID == id {
				*items = append((*items)[:index], (*items)[index+1:]...)
				return nil
			}
		}
	}
	return errors.New("not found")
}

func (a *agentMemoryServiceStub) Add(_ context.Context, target conversation.MemoryTarget, content string) (protocol.AgentMemoryItem, error) {
	if err := target.Validate(); err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	now := time.Now()
	item := protocol.AgentMemoryItem{
		ID: terminalMemoryAddedID, Scope: target.Scope, Content: content, Origin: protocol.AgentMemoryOriginUser,
		Status: protocol.AgentMemoryStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	a.mu.Lock()
	if target.Scope == protocol.AgentMemoryScopeUser {
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
	if got := awaitValue(t, memory.review, "agent memory review"); got != memorycontract.ReviewApprove {
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
		AgentMemory: base, started: make(chan memorycontract.ReviewDecision, 1),
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
	if decision := awaitValue(t, memory.started, "agent memory review"); decision != memorycontract.ReviewApprove {
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
	if decision := awaitValue(t, base.review, "committed agent memory review"); decision != memorycontract.ReviewApprove {
		t.Fatalf("committed review decision = %q", decision)
	}
	items, err := base.Items(t.Context(), conversation.MemoryTarget{Scope: protocol.AgentMemoryScopeProject, Workspace: "/tmp/demo/store"})
	if err != nil || len(items) != 1 || items[0].Status != protocol.AgentMemoryStatusActive {
		t.Fatalf("project memory after review = (%+v, %v)", items, err)
	}
	stop()
}

func TestAgentMemoryUpdateDoesNotInstallAReaderAfterSessionSwitch(t *testing.T) {
	base := newAgentMemoryServiceStub()
	memory := &blockingAgentMemoryUpdateService{
		AgentMemory: base, started: make(chan memorycontract.UpdateV1Input, 1),
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
	items, err := base.Items(t.Context(), conversation.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
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
	items, err := memory.Items(t.Context(), conversation.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
	if err != nil || len(items) != 1 || items[0].Content != "prefer explicit answers\nwith evidence" {
		t.Fatalf("edited user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/memory-unpin user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · user")
	items, err = memory.Items(t.Context(), conversation.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
	if err != nil || len(items) != 1 || items[0].Pinned {
		t.Fatalf("unpinned user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/memory-pin user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "pinned")
	items, err = memory.Items(t.Context(), conversation.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
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
	items, err = memory.Items(t.Context(), conversation.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted user memory = (%+v, %v)", items, err)
	}
	stop()
}
