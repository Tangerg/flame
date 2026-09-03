package runtimebinding

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	adapterMemoryIDOne   = "mem_00000000000000000000000000000001"
	adapterMemoryIDTwo   = "mem_00000000000000000000000000000002"
	adapterMemoryIDOther = "mem_000000000000000000000000000000ff"
)

type agentMemoryBindingStub struct {
	t            *testing.T
	actions      []string
	now          time.Time
	listed       *protocol.AgentMemoryList
	nilList      bool
	updateResult *protocol.AgentMemoryItem
	addResult    *protocol.AgentMemoryItem
}

func (a *agentMemoryBindingStub) ListAgentMemory(_ context.Context, request protocol.AgentMemoryListRequest, options flameruntime.CallOptions) (*protocol.AgentMemoryList, error) {
	a.assertMeta(options.RequestMeta)
	switch request.Scope {
	case protocol.AgentMemoryScopeProject:
		if request.Workspace == nil || request.Workspace.Path != "/workspace" {
			a.t.Fatalf("project list request = %+v", request)
		}
	case protocol.AgentMemoryScopeUser:
		if request.Workspace != nil {
			a.t.Fatalf("user list request leaked workspace: %+v", request)
		}
	default:
		a.t.Fatalf("list request = %+v", request)
	}
	if a.nilList {
		return nil, nil
	}
	if a.listed != nil {
		return a.listed, nil
	}
	return &protocol.AgentMemoryList{Items: []protocol.AgentMemoryItem{{
		ID: adapterMemoryIDOne, Scope: request.Scope, Content: "durable fact", Origin: protocol.AgentMemoryOriginAuto,
		Status: protocol.AgentMemoryStatusPending, CreatedAt: a.now, UpdatedAt: a.now,
	}}}, nil
}

func TestAgentMemoryAdapterRejectsBrokenRuntimeProjections(t *testing.T) {
	now := time.Now()
	valid := protocol.AgentMemoryItem{
		ID: adapterMemoryIDOne, Scope: protocol.AgentMemoryScopeProject, Content: "fact",
		Origin: protocol.AgentMemoryOriginAuto, Status: protocol.AgentMemoryStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	for _, test := range []struct {
		name    string
		listed  *protocol.AgentMemoryList
		nilList bool
	}{
		{name: "nil list", nilList: true},
		{name: "wrong scope", listed: &protocol.AgentMemoryList{Items: []protocol.AgentMemoryItem{{
			ID: adapterMemoryIDOne, Scope: protocol.AgentMemoryScopeUser, Content: "fact",
			Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive,
			CreatedAt: now, UpdatedAt: now,
		}}}},
		{name: "duplicate identity", listed: &protocol.AgentMemoryList{Items: []protocol.AgentMemoryItem{valid, valid}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &agentMemoryBindingStub{t: t, now: now, listed: test.listed, nilList: test.nilList}
			adapter := &AgentMemory{runtime: &Connection{
				agentMemory: stub,
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: "/workspace"},
					ProjectRoot:  "/workspace",
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
			}}
			target, err := agent.NewMemoryTarget(protocol.AgentMemoryScopeProject, "/workspace")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Items(t.Context(), target); err == nil {
				t.Fatal("broken projection was accepted")
			} else {
				requireRuntimeContractViolation(t, err)
			}
		})
	}
}

func TestAgentMemoryAdapterRejectsCatalogOrderViolations(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	item := func(id string, status protocol.AgentMemoryStatus, pinned bool, updated time.Time) protocol.AgentMemoryItem {
		origin := protocol.AgentMemoryOriginAuto
		if status == protocol.AgentMemoryStatusActive {
			origin = protocol.AgentMemoryOriginUser
		}
		return protocol.AgentMemoryItem{
			ID: id, Scope: protocol.AgentMemoryScopeProject, Content: "fact",
			Origin: origin, Status: status, Pinned: pinned,
			CreatedAt: created, UpdatedAt: updated,
		}
	}
	for _, test := range []struct {
		name  string
		items []protocol.AgentMemoryItem
	}{
		{
			name: "pending follows active",
			items: []protocol.AgentMemoryItem{
				item(adapterMemoryIDOne, protocol.AgentMemoryStatusActive, false, created),
				item(adapterMemoryIDTwo, protocol.AgentMemoryStatusPending, false, created),
			},
		},
		{
			name: "pinned follows unpinned",
			items: []protocol.AgentMemoryItem{
				item(adapterMemoryIDOne, protocol.AgentMemoryStatusPending, false, created),
				item(adapterMemoryIDTwo, protocol.AgentMemoryStatusPending, true, created),
			},
		},
		{
			name: "update time ascends",
			items: []protocol.AgentMemoryItem{
				item(adapterMemoryIDOne, protocol.AgentMemoryStatusPending, true, created),
				item(adapterMemoryIDTwo, protocol.AgentMemoryStatusPending, true, created.Add(time.Second)),
			},
		},
		{
			name: "equal-time identity ascends",
			items: []protocol.AgentMemoryItem{
				item(adapterMemoryIDOne, protocol.AgentMemoryStatusPending, true, created),
				item(adapterMemoryIDTwo, protocol.AgentMemoryStatusPending, true, created),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &agentMemoryBindingStub{
				t: t, now: created, listed: &protocol.AgentMemoryList{Items: test.items},
			}
			adapter := &AgentMemory{runtime: &Connection{
				agentMemory: stub,
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: "/workspace"},
					ProjectRoot:  "/workspace",
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
			}}
			target, err := agent.NewMemoryTarget(protocol.AgentMemoryScopeProject, "/workspace")
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Items(t.Context(), target)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func (a *agentMemoryBindingStub) ReviewAgentMemory(_ context.Context, request protocol.AgentMemoryReviewRequest, options flameruntime.CommandOptions) error {
	a.assertCommand(options)
	a.actions = append(a.actions, "review:"+request.ID+":"+string(request.Decision))
	return nil
}

func (a *agentMemoryBindingStub) UpdateAgentMemory(_ context.Context, request protocol.AgentMemoryUpdateRequest, options flameruntime.CommandOptions) (*protocol.AgentMemoryItem, error) {
	a.assertCommand(options)
	if request.Content == nil || *request.Content != "edited" || request.Pinned == nil || !*request.Pinned {
		a.t.Fatalf("update request = %+v", request)
	}
	a.actions = append(a.actions, "update:"+request.ID)
	if a.updateResult != nil {
		return a.updateResult, nil
	}
	return a.item(request.ID, protocol.AgentMemoryScopeProject, "edited", true), nil
}

func (a *agentMemoryBindingStub) DeleteAgentMemory(_ context.Context, request protocol.AgentMemoryItemRequest, options flameruntime.CommandOptions) error {
	a.assertCommand(options)
	a.actions = append(a.actions, "delete:"+request.ID)
	return nil
}

func (a *agentMemoryBindingStub) AddAgentMemory(_ context.Context, request protocol.AgentMemoryAddRequest, options flameruntime.CommandOptions) (*protocol.AgentMemoryItem, error) {
	a.assertCommand(options)
	if request.Scope != protocol.AgentMemoryScopeUser || request.Workspace != nil || request.Content != "authored" {
		a.t.Fatalf("add request = %+v", request)
	}
	a.actions = append(a.actions, "add:user")
	if a.addResult != nil {
		return a.addResult, nil
	}
	return a.item(adapterMemoryIDTwo, request.Scope, request.Content, false), nil
}

func (a *agentMemoryBindingStub) item(id string, scope protocol.AgentMemoryScope, content string, pinned bool) *protocol.AgentMemoryItem {
	return &protocol.AgentMemoryItem{
		ID: id, Scope: scope, Content: content, Origin: protocol.AgentMemoryOriginUser,
		Status: protocol.AgentMemoryStatusActive, Pinned: pinned, CreatedAt: a.now, UpdatedAt: a.now,
	}
}

func (a *agentMemoryBindingStub) assertMeta(meta protocol.RequestMeta) {
	a.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		a.t.Fatalf("request meta = %+v", meta)
	}
}

func (a *agentMemoryBindingStub) assertCommand(options flameruntime.CommandOptions) {
	a.t.Helper()
	a.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		a.t.Fatal("command has no idempotency key")
	}
}

func TestAgentMemoryAdapterPreservesTargetReviewAndMutationSemantics(t *testing.T) {
	stub := &agentMemoryBindingStub{t: t, now: time.Now()}
	adapter := &AgentMemory{runtime: &Connection{
		agentMemory: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}}
	project, err := agent.NewMemoryTarget(protocol.AgentMemoryScopeProject, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	items, err := adapter.Items(t.Context(), project)
	if err != nil || len(items) != 1 || items[0].Status != protocol.AgentMemoryStatusPending {
		t.Fatalf("Items = (%+v, %v)", items, err)
	}
	user, err := agent.NewMemoryTarget(protocol.AgentMemoryScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, itemsErr := adapter.Items(t.Context(), user); itemsErr != nil {
		t.Fatal(itemsErr)
	}
	if reviewErr := adapter.Review(t.Context(), adapterMemoryIDOne, protocol.AgentMemoryReviewApprove); reviewErr != nil {
		t.Fatal(reviewErr)
	}
	content, pinned := "edited", true
	updated, err := adapter.Update(t.Context(), protocol.AgentMemoryUpdateRequest{ID: adapterMemoryIDOne, Content: &content, Pinned: &pinned})
	if err != nil || updated.Content != content || !updated.Pinned {
		t.Fatalf("Update = (%+v, %v)", updated, err)
	}
	if _, err := adapter.Add(t.Context(), user, "authored"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Delete(t.Context(), adapterMemoryIDOne); err != nil {
		t.Fatal(err)
	}
	want := []string{"review:" + adapterMemoryIDOne + ":approve", "update:" + adapterMemoryIDOne, "add:user", "delete:" + adapterMemoryIDOne}
	if len(stub.actions) != len(want) {
		t.Fatalf("actions = %v, want %v", stub.actions, want)
	}
	for index := range want {
		if stub.actions[index] != want[index] {
			t.Fatalf("actions = %v, want %v", stub.actions, want)
		}
	}
}

func TestAgentMemoryMutationRejectsIdentityDrift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	result := protocol.AgentMemoryItem{
		ID: adapterMemoryIDOther, Scope: protocol.AgentMemoryScopeProject, Content: "edited",
		Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err := agentMemoryResult("update agent memory", adapterMemoryIDOne, "", &result, nil)
	requireRuntimeContractViolation(t, err)
}

func TestNormalizeAgentMemoryUpdateUsesRuntimeContractAndOwnsPointers(t *testing.T) {
	t.Parallel()

	content, pinned := " edited ", true
	request := protocol.AgentMemoryUpdateRequest{ID: adapterMemoryIDOne, Content: &content, Pinned: &pinned}
	normalized, err := normalizeAgentMemoryUpdate(request)
	if err != nil || normalized.Content == nil || *normalized.Content != "edited" || normalized.Pinned == nil || !*normalized.Pinned {
		t.Fatalf("normalizeAgentMemoryUpdate = (%+v, %v)", normalized, err)
	}
	pinned = false
	if !*normalized.Pinned {
		t.Fatal("normalized request aliases caller pinned storage")
	}
	blank := "  "
	for _, invalid := range []protocol.AgentMemoryUpdateRequest{
		{ID: adapterMemoryIDOne},
		{ID: adapterMemoryIDOne, Content: &blank},
	} {
		if _, err := normalizeAgentMemoryUpdate(invalid); err == nil {
			t.Fatalf("accepted invalid update %+v", invalid)
		}
	}
}

func TestAgentMemoryAdapterRejectsMutationAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	wrongContent := protocol.AgentMemoryItem{
		ID: adapterMemoryIDOne, Scope: protocol.AgentMemoryScopeProject, Content: "ignored",
		Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive, Pinned: true,
		CreatedAt: now, UpdatedAt: now,
	}
	wrongPinned := wrongContent
	wrongPinned.Content, wrongPinned.Pinned = "edited", false
	wrongAdd := protocol.AgentMemoryItem{
		ID: adapterMemoryIDTwo, Scope: protocol.AgentMemoryScopeUser, Content: "ignored",
		Origin: protocol.AgentMemoryOriginUser, Status: protocol.AgentMemoryStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	tests := []struct {
		name   string
		stub   *agentMemoryBindingStub
		invoke func(*AgentMemory) error
	}{
		{
			name: "update content",
			stub: &agentMemoryBindingStub{updateResult: &wrongContent},
			invoke: func(adapter *AgentMemory) error {
				content, pinned := "edited", true
				_, err := adapter.Update(t.Context(), protocol.AgentMemoryUpdateRequest{ID: adapterMemoryIDOne, Content: &content, Pinned: &pinned})
				return err
			},
		},
		{
			name: "update pinned",
			stub: &agentMemoryBindingStub{updateResult: &wrongPinned},
			invoke: func(adapter *AgentMemory) error {
				content, pinned := "edited", true
				_, err := adapter.Update(t.Context(), protocol.AgentMemoryUpdateRequest{ID: adapterMemoryIDOne, Content: &content, Pinned: &pinned})
				return err
			},
		},
		{
			name: "add content",
			stub: &agentMemoryBindingStub{addResult: &wrongAdd},
			invoke: func(adapter *AgentMemory) error {
				target, err := agent.NewMemoryTarget(protocol.AgentMemoryScopeUser, "")
				if err != nil {
					return err
				}
				_, err = adapter.Add(t.Context(), target, "authored")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.stub.t, test.stub.now = t, now
			adapter := &AgentMemory{runtime: &Connection{agentMemory: test.stub, meta: requestMeta("test")}}
			requireRuntimeContractViolation(t, test.invoke(adapter))
		})
	}
}
