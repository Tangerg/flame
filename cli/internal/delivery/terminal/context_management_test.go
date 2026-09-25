package terminal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
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
	review  chan protocol.AgentMemoryReviewDecision
}

type blockingAgentMemoryReviewService struct {
	AgentMemory
	started  chan protocol.AgentMemoryReviewDecision
	release  chan struct{}
	canceled chan struct{}
}

type blockingAgentMemoryUpdateService struct {
	AgentMemory
	started  chan protocol.AgentMemoryUpdateRequest
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingAgentMemoryUpdateService) Update(
	ctx context.Context,
	request protocol.AgentMemoryUpdateRequest,
) (protocol.AgentMemoryItem, error) {
	b.started <- request
	select {
	case <-b.release:
		return b.AgentMemory.Update(ctx, request)
	case <-ctx.Done():
		close(b.canceled)
		return protocol.AgentMemoryItem{}, context.Cause(ctx)
	}
}

func (b *blockingAgentMemoryReviewService) Review(
	ctx context.Context,
	id string,
	decision protocol.AgentMemoryReviewDecision,
) error {
	b.started <- decision
	select {
	case <-b.release:
		return b.AgentMemory.Review(ctx, id, decision)
	case <-ctx.Done():
		close(b.canceled)
		return context.Cause(ctx)
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
		added: make(chan string, 1), review: make(chan protocol.AgentMemoryReviewDecision, 1),
	}
}

func (a *agentMemoryServiceStub) Items(_ context.Context, target agent.MemoryTarget) ([]protocol.AgentMemoryItem, error) {
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

func (a *agentMemoryServiceStub) Review(_ context.Context, id string, decision protocol.AgentMemoryReviewDecision) error {
	if err := (protocol.AgentMemoryReviewRequest{ID: id, Decision: decision}).ValidateWire(); err != nil {
		return err
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
		if decision == protocol.AgentMemoryReviewReject {
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

func (a *agentMemoryServiceStub) Update(_ context.Context, request protocol.AgentMemoryUpdateRequest) (protocol.AgentMemoryItem, error) {
	if err := request.ValidateWire(); err != nil {
		return protocol.AgentMemoryItem{}, err
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

func (a *agentMemoryServiceStub) Delete(_ context.Context, id string) error {
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

func (a *agentMemoryServiceStub) Add(_ context.Context, target agent.MemoryTarget, content string) (protocol.AgentMemoryItem, error) {
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
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/memory project")
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · project")
	host.Shows(t, "session  ses_origin")
	stop()
}

func TestAgentMemoryMultilineAddSurvivesResize(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
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

func TestPendingAgentMemoryReviewRequiresResizeSafeConfirmation(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
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
	if got := awaitValue(t, memory.review, "agent memory review"); got != protocol.AgentMemoryReviewApprove {
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
		AgentMemory: base, started: make(chan protocol.AgentMemoryReviewDecision, 1),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(memory.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, AgentMemory: memory, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/memory-approve project " + terminalMemoryPendingID)
	host.Press(input.Enter)
	host.Shows(t, "Approve agent memory")
	host.Press(input.Down)
	host.Press(input.Enter)
	if decision := awaitValue(t, memory.started, "agent memory review"); decision != protocol.AgentMemoryReviewApprove {
		t.Fatalf("review decision = %q", decision)
	}
	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
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
	if decision := awaitValue(t, base.review, "committed agent memory review"); decision != protocol.AgentMemoryReviewApprove {
		t.Fatalf("committed review decision = %q", decision)
	}
	items, err := base.Items(t.Context(), agent.MemoryTarget{Scope: protocol.AgentMemoryScopeProject, Workspace: "/tmp/demo/store"})
	if err != nil || len(items) != 1 || items[0].Status != protocol.AgentMemoryStatusActive {
		t.Fatalf("project memory after review = (%+v, %v)", items, err)
	}
	stop()
}

func TestAgentMemoryUpdateDoesNotInstallAReaderAfterSessionSwitch(t *testing.T) {
	base := newAgentMemoryServiceStub()
	memory := &blockingAgentMemoryUpdateService{
		AgentMemory: base, started: make(chan protocol.AgentMemoryUpdateRequest, 1),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(memory.release) })
	t.Cleanup(release)
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), AgentMemory: memory, SessionID: "ses_demo_1"})
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
	items, err := base.Items(t.Context(), agent.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
	if err != nil || len(items) != 1 || items[0].Pinned {
		t.Fatalf("user memory after update = (%+v, %v)", items, err)
	}
	stop()
}

func TestAgentMemoryEditPinAndDeleteRoundTripThroughAuthoritativeReads(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), AgentMemory: memory, Workspace: "/workspace"})
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
	items, err := memory.Items(t.Context(), agent.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
	if err != nil || len(items) != 1 || items[0].Content != "prefer explicit answers\nwith evidence" {
		t.Fatalf("edited user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/memory-unpin user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · user")
	items, err = memory.Items(t.Context(), agent.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
	if err != nil || len(items) != 1 || items[0].Pinned {
		t.Fatalf("unpinned user memory = (%+v, %v)", items, err)
	}

	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/memory-pin user " + terminalMemoryUserID)
	host.Press(input.Enter)
	host.Shows(t, "pinned")
	items, err = memory.Items(t.Context(), agent.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
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
	items, err = memory.Items(t.Context(), agent.MemoryTarget{Scope: protocol.AgentMemoryScopeUser})
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted user memory = (%+v, %v)", items, err)
	}
	stop()
}
