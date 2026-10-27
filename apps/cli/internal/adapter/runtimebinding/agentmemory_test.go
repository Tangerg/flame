package runtimebinding

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type memoryFixtureRecord = memory.Method_go_5fZ2l0aHViLmNvbS9UYW5nZXJnL2ZsYW1lL3BsdWdpbnMvbWVtb3J5L2ludGVybmFsL2RvbWFpbg_5fItemSnapshot

const (
	adapterMemoryIDOne   = "mem_00000000000000000000000000000001"
	adapterMemoryIDTwo   = "mem_00000000000000000000000000000002"
	adapterMemoryIDOther = "mem_000000000000000000000000000000ff"
)

type agentMemoryBindingStub struct {
	pluginBinding
	providerReads    int
	provider         string
	protocolOverride string
	calls            []protocol.CallPluginRequest
	t                *testing.T
	actions          []string
	now              time.Time
	listed           *protocol.Page[memory.Item]
	nilList          bool
	updateResult     *memory.Item
	addResult        *memory.Item
	listPage         func(memory.ListV2Input) (*protocol.Page[memory.Item], error)
	pageCapacity     func(memory.ListV2Input) int64
	pluginCall       func(protocol.CallPluginRequest, protocol.PluginCallOptions) (*protocol.CallPluginResponse, error)
}

func (a *agentMemoryBindingStub) list(request memory.ListV2Input, options protocol.CallOptions) (*protocol.Page[memory.Item], error) {
	a.assertMeta(options.RequestMeta)
	switch request.Scope {
	case string(memory.ScopeProject):
		if request.CWD != "/workspace" {
			a.t.Fatalf("project list request = %+v", request)
		}
	case string(memory.ScopeUser):
		if request.CWD != "" {
			a.t.Fatalf("user list request leaked workspace: %+v", request)
		}
	default:
		a.t.Fatalf("list request = %+v", request)
	}
	if a.listPage != nil {
		return a.listPage(request)
	}
	if a.nilList {
		return nil, nil
	}
	if a.listed != nil {
		return a.listed, nil
	}
	return &protocol.Page[memory.Item]{Data: []memory.Item{{
		Revision: 1, ID: adapterMemoryIDOne, Scope: memory.Scope(request.Scope), Content: "durable fact", Origin: memory.OriginAuto,
		Status: memory.StatusPending, CreatedAt: a.now, UpdatedAt: a.now,
	}}}, nil
}

func TestAgentMemoryAdapterTraversesWithoutPartialSuccess(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	for _, scenario := range []string{"complete", "duplicate", "later failure"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			item := memory.Item{Revision: 1, ID: adapterMemoryIDOne, Scope: memory.ScopeUser, Content: "fact", Origin: memory.OriginUser, Status: memory.StatusActive, CreatedAt: now, UpdatedAt: now}
			stub := &agentMemoryBindingStub{t: t, listPage: func(request memory.ListV2Input) (*protocol.Page[memory.Item], error) {
				calls++
				if calls == 1 {
					if request.Cursor != "" {
						t.Fatal("first cursor")
					}
					return &protocol.Page[memory.Item]{Data: []memory.Item{item}, NextCursor: "next"}, nil
				}
				if request.Cursor != "next" {
					t.Fatal("continuation was lost")
				}
				if scenario == "later failure" {
					return nil, errors.New("storage unavailable")
				}
				if scenario == "complete" {
					item.ID = adapterMemoryIDTwo
				}
				return &protocol.Page[memory.Item]{Data: []memory.Item{item}}, nil
			}}
			adapter := &AgentMemory{runtime: &Connection{plugins: stub, meta: requestMeta("test")}}
			target, err := conversation.NewMemoryTarget(memory.ScopeUser, "")
			if err != nil {
				t.Fatal(err)
			}
			items, err := adapter.Items(t.Context(), target)
			if calls != 2 {
				t.Fatalf("calls = %d", calls)
			}
			if scenario == "complete" {
				if err != nil || len(items) != 2 {
					t.Fatalf("complete = %+v, %v", items, err)
				}
			} else if err == nil || items != nil {
				t.Fatalf("partial success = %+v, %v", items, err)
			}
		})
	}
}

func TestAgentMemoryAdapterRejectsBrokenRuntimeProjections(t *testing.T) {
	now := time.Now()
	valid := memory.Item{
		Revision: 1, ID: adapterMemoryIDOne, Scope: memory.ScopeProject, Content: "fact",
		Origin: memory.OriginAuto, Status: memory.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	for _, test := range []struct {
		name    string
		listed  *protocol.Page[memory.Item]
		nilList bool
	}{
		{name: "nil list", nilList: true},
		{name: "wrong scope", listed: &protocol.Page[memory.Item]{Data: []memory.Item{{
			Revision: 1, ID: adapterMemoryIDOne, Scope: memory.ScopeUser, Content: "fact",
			Origin: memory.OriginUser, Status: memory.StatusActive,
			CreatedAt: now, UpdatedAt: now,
		}}}},
		{name: "duplicate identity", listed: &protocol.Page[memory.Item]{Data: []memory.Item{valid, valid}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &agentMemoryBindingStub{t: t, now: now, listed: test.listed, nilList: test.nilList}
			adapter := &AgentMemory{runtime: &Connection{
				plugins: stub,
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: "/workspace"},
					ProjectRoot:  "/workspace",
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
			}}
			target, err := conversation.NewMemoryTarget(memory.ScopeProject, "/workspace")
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

func TestMemoryTraversalRejectsCapacityDriftWithoutPartialSuccess(t *testing.T) {
	stub := &agentMemoryBindingStub{t: t, now: time.Now()}
	stub.listPage = func(input memory.ListV2Input) (*protocol.Page[memory.Item], error) {
		if input.Limit != nil {
			t.Fatal("collector replaced the owner's default page width")
		}
		if input.Cursor == "" {
			return &protocol.Page[memory.Item]{Data: []memory.Item{*stub.item(adapterMemoryIDOne, memory.ScopeUser, "fact", false)}, NextCursor: "next"}, nil
		}
		return &protocol.Page[memory.Item]{Data: []memory.Item{}}, nil
	}
	stub.pageCapacity = func(input memory.ListV2Input) int64 {
		if input.Cursor == "" {
			return 512
		}
		return 513
	}
	adapter := &AgentMemory{runtime: &Connection{plugins: stub, meta: requestMeta("test")}}
	items, err := adapter.Items(t.Context(), conversation.MemoryTarget{Scope: memory.ScopeUser})
	if err == nil || items != nil {
		t.Fatalf("capacity drift returned partial success: %+v, %v", items, err)
	}
}

func (a *agentMemoryBindingStub) item(id string, scope memory.Scope, content string, pinned bool) *memory.Item {
	return &memory.Item{
		Revision: 1, ID: id, Scope: scope, Content: content, Origin: memory.OriginUser,
		Status: memory.StatusActive, Pinned: pinned, CreatedAt: a.now, UpdatedAt: a.now,
	}
}

func (a *agentMemoryBindingStub) assertMeta(meta protocol.RequestMeta) {
	a.t.Helper()
	if meta.ProtocolVersion != protocol.Version {
		a.t.Fatalf("request meta = %+v", meta)
	}
}

func (a *agentMemoryBindingStub) assertCommand(options protocol.CommandOptions) {
	a.t.Helper()
	a.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		a.t.Fatal("command has no idempotency key")
	}
}

func TestAgentMemoryAdditionPreservesCapturedWorkspace(t *testing.T) {
	stub := &agentMemoryBindingStub{t: t}
	adapter := &AgentMemory{runtime: &Connection{plugins: stub, meta: requestMeta("test")}}
	input := memory.AddV2Input{Scope: "project", CWD: "/workspace/subdirectory", Content: "authored"}
	request, err := adapter.PrepareAdd(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var captured memory.AddV2Input
	if err := json.Unmarshal([]byte(request.Input), &captured); err != nil {
		t.Fatal(err)
	}
	if captured != input {
		t.Fatalf("captured addition = %+v, want %+v", captured, input)
	}
}

func TestAgentMemoryAdapterPreservesTargetReviewAndMutationSemantics(t *testing.T) {
	stub := &agentMemoryBindingStub{t: t, now: time.Now()}
	adapter := &AgentMemory{runtime: &Connection{
		plugins: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}}
	project, err := conversation.NewMemoryTarget(memory.ScopeProject, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	items, err := adapter.Items(t.Context(), project)
	if err != nil || len(items) != 1 || items[0].Status != memory.StatusPending {
		t.Fatalf("Items = (%+v, %v)", items, err)
	}
	user, err := conversation.NewMemoryTarget(memory.ScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, itemsErr := adapter.Items(t.Context(), user); itemsErr != nil {
		t.Fatal(itemsErr)
	}
	review, err := adapter.PrepareReview(t.Context(), adapterMemoryIDOne, 1, memory.ReviewApprove)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CallPlugin(t.Context(), review, mutation.NewCommandID()); err != nil {
		t.Fatal(err)
	}
	content, pinned := "edited", true
	update, err := adapter.PrepareUpdate(t.Context(), memory.UpdateV2Input{ExpectedRevision: 1, ID: adapterMemoryIDOne, Content: &content, Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := adapter.CallPlugin(t.Context(), update, mutation.NewCommandID())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := adapter.DecodeUpdate(adapterMemoryIDOne, reply)
	if err != nil || updated.Content != content || !updated.Pinned {
		t.Fatalf("Update = (%+v, %v)", updated, err)
	}
	addition, err := adapter.PrepareAdd(t.Context(), memory.AddV2Input{Scope: string(user.Scope), Content: "authored"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CallPlugin(t.Context(), addition, mutation.NewCommandID()); err != nil {
		t.Fatal(err)
	}
	request, err := adapter.PrepareDelete(t.Context(), adapterMemoryIDOne, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CallPlugin(t.Context(), request, mutation.NewCommandID()); err != nil {
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
	result, err := jsonvalue.Encode(memory.UpdateV2Output{ID: adapterMemoryIDOther, Scope: "project", Content: "edited", Origin: "user", Status: "active", CreatedAt: now, UpdatedAt: now, Embedding: []float64{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&AgentMemory{}).DecodeUpdate(adapterMemoryIDOne, &protocol.CallPluginResponse{Output: string(result.JSON())})
	requireRuntimeContractViolation(t, err)
}

func TestAgentMemoryAdapterRejectsAnUnidentifiedAddition(t *testing.T) {
	t.Parallel()
	now := time.Now()
	unidentifiedAdd := memory.Item{
		Scope: memory.ScopeUser, Content: "authored",
		Origin: memory.OriginUser, Status: memory.StatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	tests := []struct {
		name   string
		stub   *agentMemoryBindingStub
		invoke func(*AgentMemory) error
	}{
		{
			// add mints the item ID, so the presence of an identity is the
			// whole contract.
			name: "add identity",
			stub: &agentMemoryBindingStub{addResult: &unidentifiedAdd},
			invoke: func(adapter *AgentMemory) error {
				target, err := conversation.NewMemoryTarget(memory.ScopeUser, "")
				if err != nil {
					return err
				}
				request, err := adapter.PrepareAdd(t.Context(), memory.AddV2Input{Scope: string(target.Scope), CWD: target.Workspace, Content: "authored"})
				if err != nil {
					return err
				}
				reply, err := adapter.CallPlugin(t.Context(), request, mutation.NewCommandID())
				if err != nil {
					return err
				}
				_, err = adapter.DecodeAdd(reply)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.stub.t, test.stub.now = t, now
			adapter := &AgentMemory{runtime: &Connection{plugins: test.stub, meta: requestMeta("test")}}
			requireRuntimeContractViolation(t, test.invoke(adapter))
		})
	}
}

const memoryInstallation = "12345678-1234-1234-1234-123456789abc"

func (a *agentMemoryBindingStub) ListPluginProviders(_ context.Context, _ protocol.CallOptions) (*protocol.Page[protocol.PluginProviderSelection], error) {
	a.providerReads++
	id := memoryInstallation
	if a.provider != "" {
		id = a.provider
	}
	return protocol.NewPage([]protocol.PluginProviderSelection{{Role: "memory", InstallationID: id, Contribution: "memory-provider"}}), nil
}
func (a *agentMemoryBindingStub) ListPlugins(_ context.Context, _ protocol.CallOptions) (*protocol.Page[protocol.PluginInstallation], error) {
	contract, err := extension.ParseCapability([]byte(memory.CapabilityJSON))
	if err != nil {
		a.t.Fatal(err)
	}
	descriptor := contract.Descriptor()
	if a.protocolOverride != "" {
		descriptor.Protocol = a.protocolOverride
	}
	encoded, err := json.Marshal(descriptor, json.Deterministic(true))
	if err != nil {
		a.t.Fatal(err)
	}
	return protocol.NewPage([]protocol.PluginInstallation{{ID: memoryInstallation, Enabled: true, Selected: protocol.PluginRelease{Digest: "1111111111111111111111111111111111111111111111111111111111111111", Contributions: []protocol.PluginContribution{{ID: "memory-provider", Contract: string(encoded)}}}}}), nil
}
func (a *agentMemoryBindingStub) CallPlugin(_ context.Context, request protocol.CallPluginRequest, options protocol.PluginCallOptions) (*protocol.CallPluginResponse, error) {
	if request.InstallationID != memoryInstallation || request.Target.Reference != memoryInstallation || request.Contract != memory.CapabilityJSON {
		a.t.Fatalf("unbound memory operation: %+v", request)
	}
	a.calls = append(a.calls, request)
	if a.pluginCall != nil {
		return a.pluginCall(request, options)
	}
	if request.Method == memory.AddV2Method {
		a.assertCommand(protocol.CommandOptions{RequestMeta: options.RequestMeta, IdempotencyKey: options.IdempotencyKey})
		var input memory.AddV2Input
		if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
			a.t.Fatal(err)
		}
		a.actions = append(a.actions, "add:"+input.Scope)
		result := a.addResult
		if result == nil {
			result = a.item(adapterMemoryIDTwo, memory.Scope(input.Scope), input.Content, false)
		}
		out := memory.AddV2Output{Item: memoryFixtureRecord{Revision: result.Revision, ID: memory.Method_go_5fZ2l0aHViLmNvbS9UYW5nZXJnL2ZsYW1lL3BsdWdpbnMvbWVtb3J5L2ludGVybmFsL2RvbWFpbg_5fItemID(result.ID), Scope: string(result.Scope), Content: result.Content, Origin: string(result.Origin), Status: string(result.Status), Pinned: result.Pinned, SessionID: result.SessionID, Day: result.Day, CreatedAt: result.CreatedAt, UpdatedAt: result.UpdatedAt, Embedding: []float64{}}}
		encoded, err := jsonvalue.Encode(out)
		return &protocol.CallPluginResponse{Output: string(encoded.JSON())}, err
	}
	if request.Method == memory.UpdateV2Method {
		a.assertCommand(protocol.CommandOptions{RequestMeta: options.RequestMeta, IdempotencyKey: options.IdempotencyKey})
		var input memory.UpdateV2Input
		if err := json.Unmarshal([]byte(request.Input), &input, json.RejectUnknownMembers(true)); err != nil {
			a.t.Fatal(err)
		}
		a.actions = append(a.actions, "update:"+input.ID)
		result := a.updateResult
		if result == nil {
			result = a.item(input.ID, memory.ScopeProject, "edited", true)
		}
		output := memory.UpdateV2Output{Revision: result.Revision, ID: memory.Method_go_5fZ2l0aHViLmNvbS9UYW5nZXJnL2ZsYW1lL3BsdWdpbnMvbWVtb3J5L2ludGVybmFsL2RvbWFpbg_5fItemID(result.ID), Scope: string(result.Scope), Content: result.Content, Origin: string(result.Origin), Status: string(result.Status), Pinned: result.Pinned, SessionID: result.SessionID, Day: result.Day, CreatedAt: result.CreatedAt, UpdatedAt: result.UpdatedAt, Embedding: []float64{}}
		encoded, err := jsonvalue.Encode(output)
		return &protocol.CallPluginResponse{Output: string(encoded.JSON())}, err
	}
	if request.Method == memory.ReviewV2Method {
		a.assertCommand(protocol.CommandOptions{RequestMeta: options.RequestMeta, IdempotencyKey: options.IdempotencyKey})
		var input memory.ReviewV2Input
		if err := json.Unmarshal([]byte(request.Input), &input, json.RejectUnknownMembers(true)); err != nil {
			a.t.Fatal(err)
		}
		a.actions = append(a.actions, "review:"+input.ID+":"+input.Decision)
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	}
	if request.Method == memory.DeleteV2Method {
		a.assertCommand(protocol.CommandOptions{RequestMeta: options.RequestMeta, IdempotencyKey: options.IdempotencyKey})
		var input memory.DeleteV2Input
		if err := json.Unmarshal([]byte(request.Input), &input, json.RejectUnknownMembers(true)); err != nil {
			a.t.Fatal(err)
		}
		a.actions = append(a.actions, "delete:"+input.ID)
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	}
	if request.Method != memory.ListV2Method || options.IdempotencyKey != "" {
		a.t.Fatalf("invalid memory query: %+v", request)
	}
	var input memory.ListV2Input
	if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
		a.t.Fatal(err)
	}
	page, err := a.list(input, protocol.CallOptions{RequestMeta: options.RequestMeta})
	if err != nil || page == nil {
		return nil, err
	}
	output := memory.ListV2Output{Capacity: 512, NextCursor: page.NextCursor, Rows: make([]memoryFixtureRecord, 0, len(page.Data))}
	if a.pageCapacity != nil {
		output.Capacity = a.pageCapacity(input)
	}
	for _, row := range page.Data {
		output.Rows = append(output.Rows, memoryFixtureRecord{Revision: row.Revision, ID: memory.Method_go_5fZ2l0aHViLmNvbS9UYW5nZXJnL2ZsYW1lL3BsdWdpbnMvbWVtb3J5L2ludGVybmFsL2RvbWFpbg_5fItemID(row.ID), Scope: string(row.Scope), Content: row.Content, Origin: string(row.Origin), Status: string(row.Status), Pinned: row.Pinned, SessionID: row.SessionID, Day: row.Day, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Embedding: []float64{}})
	}
	encoded, err := jsonvalue.Encode(output)
	if err != nil {
		a.t.Fatal(err)
	}
	return &protocol.CallPluginResponse{Output: string(encoded.JSON())}, nil
}

func TestMemoryCommandsResumeOriginalBindingAfterSelectionChanges(t *testing.T) {
	for _, method := range []string{memory.DeleteV2Method, memory.ReviewV2Method, memory.UpdateV2Method, memory.AddV2Method} {
		t.Run(method, func(t *testing.T) {

			directory := t.TempDir()
			store, err := openTestWorkbench(directory)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			limits, err := authoring.NewReplayCapability("original-store", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			policy, err := mutation.PolicyFromProfile(runtimefixture.ReplayProfile{Capability: limits}, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			stub := &agentMemoryBindingStub{t: t}
			adapter := &AgentMemory{runtime: &Connection{plugins: stub, meta: requestMeta("test")}}
			intent := "memory-command:" + method + ":" + adapterMemoryIDOne
			ctx, cancel := context.WithCancel(t.Context())
			stub.pluginCall = func(request protocol.CallPluginRequest, options protocol.PluginCallOptions) (*protocol.CallPluginResponse, error) {
				pending, staged := store.PendingInstalledCommand(intent)
				if !staged || string(pending.CommandID) != options.IdempotencyKey || !(request.Input == pending.Request.Input) {
					t.Fatal("delete dispatched before retaining its original identity and input")
				}
				cancel()
				return nil, conversation.ErrDisconnected
			}
			execute := func(ctx context.Context) error {
				_, err := workbench.ExecuteInstalledCommand(ctx, adapter, store, intent, policy, func(ctx context.Context) (protocol.CallPluginRequest, error) {
					if method == memory.AddV2Method {
						return adapter.PrepareAdd(ctx, memory.AddV2Input{Scope: "user", Content: "original addition"})
					}
					if method == memory.UpdateV2Method {
						return adapter.PrepareUpdate(ctx, memory.UpdateV2Input{ExpectedRevision: 1, ID: adapterMemoryIDOne, Content: new("original patch"), Pinned: new(false)})
					}
					if method == memory.ReviewV2Method {
						return adapter.PrepareReview(ctx, adapterMemoryIDOne, 1, memory.ReviewApprove)
					}
					return adapter.PrepareDelete(ctx, adapterMemoryIDOne, 1)
				}, func(reply *protocol.CallPluginResponse) (struct{}, error) {
					if method == memory.AddV2Method {
						_, err := adapter.DecodeAdd(reply)
						return struct{}{}, err
					}
					if method == memory.UpdateV2Method {
						_, err := adapter.DecodeUpdate(adapterMemoryIDOne, reply)
						return struct{}{}, err
					}
					return struct{}{}, nil
				})
				return err
			}
			if err := execute(ctx); !errors.Is(err, authoring.ErrCommandOutcomeUnknown) {
				t.Fatalf("lost delete acknowledgement: %v", err)
			}
			pending, retained := store.PendingInstalledCommand(intent)
			if !retained {
				t.Fatal("unknown delete lost its original request")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = openTestWorkbench(directory)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			stub.provider = "replacement-installation"
			stub.pluginCall = func(protocol.CallPluginRequest, protocol.PluginCallOptions) (*protocol.CallPluginResponse, error) {
				return &protocol.CallPluginResponse{Output: `{"extra":true}`}, nil
			}
			if err := execute(t.Context()); !errors.Is(err, authoring.ErrCommandOutcomeUnknown) {
				t.Fatalf("invalid acknowledgement confirmed a delete: %v", err)
			}
			if _, retained := store.PendingInstalledCommand(intent); !retained {
				t.Fatal("invalid acknowledgement retired the original command")
			}
			stub.pluginCall = func(request protocol.CallPluginRequest, options protocol.PluginCallOptions) (*protocol.CallPluginResponse, error) {
				if request.InstallationID != pending.Request.InstallationID || request.Digest != pending.Request.Digest || request.Contribution != pending.Request.Contribution || request.Contract != pending.Request.Contract || request.Method != pending.Request.Method || request.Target != pending.Request.Target || !(request.Input == pending.Request.Input) || options.IdempotencyKey != string(pending.CommandID) {
					t.Fatal("delete recovery changed its original command or binding")
				}
				if method == memory.AddV2Method {
					output, err := jsonvalue.Encode(memory.AddV2Output{Changed: true, Item: memoryFixtureRecord{Revision: 1, ID: adapterMemoryIDOne, Scope: "user", Content: "original addition", Origin: "user", Status: "active", CreatedAt: now, UpdatedAt: now, Embedding: []float64{}}})
					return &protocol.CallPluginResponse{Output: string(output.JSON())}, err
				}
				if method == memory.UpdateV2Method {
					output, err := jsonvalue.Encode(memory.UpdateV2Output{Revision: 1, ID: adapterMemoryIDOne, Scope: "user", Content: "original patch", Origin: "user", Status: "active", CreatedAt: now, UpdatedAt: now, Embedding: []float64{}})
					return &protocol.CallPluginResponse{Output: string(output.JSON())}, err
				}
				return &protocol.CallPluginResponse{Output: `{}`}, nil
			}
			if err := execute(t.Context()); err != nil || stub.providerReads != 1 || len(stub.calls) != 3 {
				t.Fatalf("delete recovery: providers=%d calls=%d error=%v", stub.providerReads, len(stub.calls), err)
			}
			if _, retained := store.PendingInstalledCommand(intent); retained {
				t.Fatal("confirmed delete remains pending")
			}

		})
	}
}

func TestMemoryReadRetainsOriginalProviderAcrossPages(t *testing.T) {
	now := time.Now()
	stub := &agentMemoryBindingStub{t: t}
	stub.listPage = func(input memory.ListV2Input) (*protocol.Page[memory.Item], error) {
		row := memory.Item{Revision: 1, ID: adapterMemoryIDOne, Scope: memory.ScopeUser, Content: "fact", Origin: memory.OriginUser, Status: memory.StatusActive, CreatedAt: now, UpdatedAt: now}
		if input.Cursor == "" {
			stub.provider = "replacement-installation"
			return protocol.NewPageWithCursor([]memory.Item{row}, "next"), nil
		}
		row.ID = adapterMemoryIDTwo
		return protocol.NewPage([]memory.Item{row}), nil
	}
	adapter := &AgentMemory{runtime: &Connection{plugins: stub, meta: requestMeta("test")}}
	target, err := conversation.NewMemoryTarget(memory.ScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	items, err := adapter.Items(t.Context(), target)
	if err != nil || len(items) != 2 || stub.providerReads != 1 || len(stub.calls) != 2 {
		t.Fatalf("original Memory traversal: items=%v, providers=%d, calls=%d, error=%v", items, stub.providerReads, len(stub.calls), err)
	}
}

func TestMemoryReadRefusesAnotherPublishedProtocolBeforeDispatch(t *testing.T) {
	stub := &agentMemoryBindingStub{t: t, protocolOverride: "another-protocol"}
	adapter := &AgentMemory{runtime: &Connection{plugins: stub, meta: requestMeta("test")}}
	target, err := conversation.NewMemoryTarget(memory.ScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Items(t.Context(), target); !errors.Is(err, protocol.ErrPluginUnavailable) || len(stub.calls) != 0 {
		t.Fatalf("mismatched publication dispatched: %v, calls=%d", err, len(stub.calls))
	}
}
