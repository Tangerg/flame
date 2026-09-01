package terminal

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

const (
	terminalMemoryPendingID = "mem_00000000000000000000000000000001"
	terminalMemoryUserID    = "mem_00000000000000000000000000000002"
	terminalMemoryAddedID   = "mem_00000000000000000000000000000003"
)

type agentMemoryServiceStub struct {
	mu      sync.Mutex
	project []agent.MemoryItem
	user    []agent.MemoryItem
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
	started  chan agent.MemoryPatch
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingAgentMemoryUpdateService) Update(
	ctx context.Context,
	patch agent.MemoryPatch,
) (agent.MemoryItem, error) {
	b.started <- patch
	select {
	case <-b.release:
		return b.AgentMemory.Update(ctx, patch)
	case <-ctx.Done():
		close(b.canceled)
		return agent.MemoryItem{}, context.Cause(ctx)
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
		project: []agent.MemoryItem{{
			ID: terminalMemoryPendingID, Scope: protocol.AgentMemoryScopeProject, Content: "confirm release steps",
			Origin: protocol.AgentMemoryOriginAuto, Status: protocol.AgentMemoryStatusPending, SessionID: "ses_origin",
			Day: "2026-08-12", CreatedAt: now, UpdatedAt: now,
		}},
		user: []agent.MemoryItem{{
			ID: terminalMemoryUserID, Scope: protocol.AgentMemoryScopeUser, Content: "prefer concise answers",
			Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive, Pinned: true,
			CreatedAt: now, UpdatedAt: now,
		}},
		added: make(chan string, 1), review: make(chan protocol.AgentMemoryReviewDecision, 1),
	}
}

func (a *agentMemoryServiceStub) Items(_ context.Context, target agent.MemoryTarget) ([]agent.MemoryItem, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if target.Scope == protocol.AgentMemoryScopeUser {
		return append([]agent.MemoryItem(nil), a.user...), nil
	}
	return append([]agent.MemoryItem(nil), a.project...), nil
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

func (a *agentMemoryServiceStub) Update(_ context.Context, patch agent.MemoryPatch) (agent.MemoryItem, error) {
	if err := patch.Validate(); err != nil {
		return agent.MemoryItem{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, items := range []*[]agent.MemoryItem{&a.project, &a.user} {
		for index := range *items {
			item := &(*items)[index]
			if item.ID != patch.ID {
				continue
			}
			if patch.Content != nil {
				item.Content = *patch.Content
			}
			if patch.Pinned != nil {
				item.Pinned = *patch.Pinned
			}
			item.UpdatedAt = time.Now()
			return *item, nil
		}
	}
	return agent.MemoryItem{}, errors.New("not found")
}

func (a *agentMemoryServiceStub) Delete(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, items := range []*[]agent.MemoryItem{&a.project, &a.user} {
		for index := range *items {
			if (*items)[index].ID == id {
				*items = append((*items)[:index], (*items)[index+1:]...)
				return nil
			}
		}
	}
	return errors.New("not found")
}

func (a *agentMemoryServiceStub) Add(_ context.Context, target agent.MemoryTarget, content string) (agent.MemoryItem, error) {
	if err := target.Validate(); err != nil {
		return agent.MemoryItem{}, err
	}
	now := time.Now()
	item := agent.MemoryItem{
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

type knowledgeServiceStub struct {
	mu        sync.Mutex
	content   map[protocol.KnowledgeScope]string
	revisions map[protocol.KnowledgeScope]string
	saved     chan string
	failed    chan struct{}
	failNext  bool
	blockNext <-chan struct{}
	started   chan string
}

func newKnowledgeServiceStub() *knowledgeServiceStub {
	return &knowledgeServiceStub{
		content: map[protocol.KnowledgeScope]string{
			protocol.KnowledgeScopeCWD:         "cwd guidance",
			protocol.KnowledgeScopeProjectRoot: "project rules",
			protocol.KnowledgeScopeHome:        "global preferences",
		},
		revisions: map[protocol.KnowledgeScope]string{
			protocol.KnowledgeScopeCWD: "rev-cwd", protocol.KnowledgeScopeProjectRoot: "rev-project", protocol.KnowledgeScopeHome: "rev-home",
		},
		saved: make(chan string, 1), failed: make(chan struct{}, 1),
	}
}

func (k *knowledgeServiceStub) Entries(context.Context, string) ([]workspace.KnowledgeEntry, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := time.Now()
	return []workspace.KnowledgeEntry{
		{Scope: protocol.KnowledgeScopeCWD, Content: k.content[protocol.KnowledgeScopeCWD], Revision: k.revisions[protocol.KnowledgeScopeCWD], UpdatedAt: &now},
		{Scope: protocol.KnowledgeScopeProjectRoot, Content: k.content[protocol.KnowledgeScopeProjectRoot], Revision: k.revisions[protocol.KnowledgeScopeProjectRoot], UpdatedAt: &now},
		{Scope: protocol.KnowledgeScopeHome, Content: k.content[protocol.KnowledgeScopeHome], Revision: k.revisions[protocol.KnowledgeScopeHome], UpdatedAt: &now},
	}, nil
}

func (k *knowledgeServiceStub) Document(_ context.Context, target workspace.KnowledgeTarget) (workspace.KnowledgeEntry, error) {
	if err := target.Validate(); err != nil {
		return workspace.KnowledgeEntry{}, err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return workspace.KnowledgeEntry{Scope: target.Scope, Content: k.content[target.Scope], Revision: k.revisions[target.Scope]}, nil
}

func (k *knowledgeServiceStub) Save(ctx context.Context, update workspace.KnowledgeUpdate) (workspace.KnowledgeEntry, error) {
	if err := update.Validate(); err != nil {
		return workspace.KnowledgeEntry{}, err
	}
	target, content := update.Target, update.Content
	k.mu.Lock()
	if k.revisions[target.Scope] != update.ExpectedRevision {
		k.mu.Unlock()
		return workspace.KnowledgeEntry{}, errors.New("revision conflict")
	}
	if k.failNext {
		k.failNext = false
		k.mu.Unlock()
		k.failed <- struct{}{}
		return workspace.KnowledgeEntry{}, errors.New("write refused")
	}
	block := k.blockNext
	k.blockNext = nil
	k.mu.Unlock()
	if block != nil {
		k.started <- content
		select {
		case <-block:
		case <-ctx.Done():
			return workspace.KnowledgeEntry{}, context.Cause(ctx)
		}
	}
	k.mu.Lock()
	k.content[target.Scope] = content
	k.revisions[target.Scope] += "+1"
	entry := workspace.KnowledgeEntry{Scope: target.Scope, Content: content, Revision: k.revisions[target.Scope]}
	k.mu.Unlock()
	k.saved <- content
	return entry, nil
}

func TestAgentMemoryAndKnowledgeReadersShowScopeAndProvenance(t *testing.T) {
	memory := newAgentMemoryServiceStub()
	knowledgeStore := newKnowledgeServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), AgentMemory: memory, Knowledge: knowledgeStore, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/memory project")
	host.Press(input.Enter)
	host.Shows(t, "Agent memory · project")
	host.Shows(t, "session  ses_origin")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/knowledge")
	host.Press(input.Enter)
	host.Shows(t, "FLAME.md knowledge")
	host.Shows(t, "global preferences")
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
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
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
		AgentMemory: base, started: make(chan agent.MemoryPatch, 1),
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

func TestKnowledgeReadUsesTheRequestedScope(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Knowledge: knowledgeStore, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/knowledge-read home")
	host.Press(input.Enter)
	host.Shows(t, "FLAME.md · home")
	host.Shows(t, "global preferences")
	stop()
}

func TestKnowledgeChangeConvergesTheExactOpenScope(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.KnowledgeChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Knowledge: knowledgeStore, Changes: source, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	subscription := awaitValue(t, source.subscription, "knowledge change subscription")
	if !slices.Equal(subscription.Topics, []changefeed.Topic{changefeed.KnowledgeChanged}) {
		t.Fatalf("knowledge subscription = %v", subscription.Topics)
	}
	host.Type("/knowledge-read home")
	host.Press(input.Enter)
	host.Shows(t, "global preferences")

	knowledgeStore.mu.Lock()
	knowledgeStore.content[protocol.KnowledgeScopeHome] = "preferences from runtime change"
	knowledgeStore.revisions[protocol.KnowledgeScopeHome] = "rev-home+external"
	knowledgeStore.mu.Unlock()
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.KnowledgeChanged), Sequence: 1}
	awaitValue(t, source.applied, "knowledge invalidation")
	host.Shows(t, "preferences from runtime change")
	host.Hides(t, "cwd guidance")
	stop()
}

func TestKnowledgeResyncConvergesTheExactOpenScope(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.KnowledgeChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Knowledge: knowledgeStore, Changes: source, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "knowledge resync subscription")
	host.Type("/knowledge-read home")
	host.Press(input.Enter)
	host.Shows(t, "global preferences")

	knowledgeStore.mu.Lock()
	knowledgeStore.content[protocol.KnowledgeScopeHome] = "preferences from scoped resync"
	knowledgeStore.revisions[protocol.KnowledgeScopeHome] = "rev-home+resync"
	knowledgeStore.mu.Unlock()
	source.events <- changefeed.Event{
		Type: changefeed.Resync, Sequence: 1, Topics: []changefeed.Topic{changefeed.KnowledgeChanged},
	}
	awaitValue(t, source.applied, "knowledge resync")
	host.Shows(t, "preferences from scoped resync")
	stop()
}

func TestKnowledgeEditorPreservesMultilineContentAcrossResize(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Knowledge: knowledgeStore, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit FLAME.md · projectRoot")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("knowledge editor did not survive a minimal viewport")
	}
	host.Shows(t, "Edit FLAME.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("line one")
	host.Press(input.Enter)
	host.Type("line two")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.saved, "knowledge save"); got != "line one\nline two" {
		t.Fatalf("saved content = %q", got)
	}
	host.Shows(t, "FLAME.md · projectRoot")
	host.Shows(t, "line one")
	stop()
}

func TestKnowledgeEditorRetainsDraftWhenRuntimeSaveFails(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	knowledgeStore.failNext = true
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Knowledge: knowledgeStore, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit FLAME.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("unsaved draft")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	awaitValue(t, knowledgeStore.failed, "failed knowledge save")
	host.Shows(t, "Edit FLAME.md · projectRoot")
	host.Shows(t, "write refused")
	host.Shows(t, "unsaved draft")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("failed knowledge editor did not survive a minimal viewport")
	}
	host.Shows(t, "unsaved draft")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.saved, "retried knowledge save"); got != "unsaved draft" {
		t.Fatalf("retried content = %q", got)
	}
	host.Shows(t, "FLAME.md · projectRoot")
	stop()
}

func TestKnowledgeEditorDoesNotLoseEditsMadeWhileSaving(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	release := make(chan struct{})
	knowledgeStore.blockNext = release
	knowledgeStore.started = make(chan string, 1)
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Knowledge: knowledgeStore, Workspace: "/workspace"})
	host.Shows(t, "Ask flame")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit FLAME.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("first draft")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.started, "knowledge save start"); got != "first draft" {
		t.Fatalf("first submitted content = %q", got)
	}
	host.Type(" with newer edits")
	host.Shows(t, "first draft with newer edits")
	close(release)
	if got := awaitValue(t, knowledgeStore.saved, "first knowledge save"); got != "first draft" {
		t.Fatalf("first saved content = %q", got)
	}
	host.Shows(t, "Saved. New edits remain unsaved.")
	host.Shows(t, "first draft with newer edits")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.saved, "second knowledge save"); got != "first draft with newer edits" {
		t.Fatalf("second saved content = %q", got)
	}
	host.Shows(t, "FLAME.md · projectRoot")
	stop()
}

func TestKnowledgeEditorSaveOutlivesSameSessionProjectionReplacement(t *testing.T) {
	knowledgeStore := newKnowledgeServiceStub()
	release := make(chan struct{})
	knowledgeStore.blockNext = release
	knowledgeStore.started = make(chan string, 1)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	backend := runtimefixture.New()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, Knowledge: knowledgeStore, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/knowledge-edit projectRoot")
	host.Press(input.Enter)
	host.Shows(t, "Edit FLAME.md · projectRoot")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("draft survives projection refresh")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if got := awaitValue(t, knowledgeStore.started, "blocked knowledge save"); got != "draft survives projection refresh" {
		t.Fatalf("blocked save content = %q", got)
	}
	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}

	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	host.Shows(t, "draft survives projection refresh")
	host.Hides(t, "Save interrupted")
	close(release)
	if got := awaitValue(t, knowledgeStore.saved, "knowledge save after projection replacement"); got != "draft survives projection refresh" {
		t.Fatalf("saved content = %q", got)
	}
	host.Shows(t, "FLAME.md · projectRoot")
	stop()
}
