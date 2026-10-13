package runtimebinding

import (
	"context"
	json "encoding/json/v2"
	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/schedule"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type scheduleBindingStub struct {
	pluginBinding
	queries          []protocol.CallPluginRequest
	protocolOverride string
	providerReads    int
	provider         string
	t                *testing.T
	now              time.Time
	actions          []string
	keys             map[string]struct{}
	created          schedule.CreateV1Input
	updated          protocol.UpdateScheduleRequest
	createResult     *protocol.Schedule
	updateResult     *protocol.Schedule
	list             func(protocol.PageQuery) *protocol.Page[protocol.Schedule]
}

const scheduleInstallation = "12345678-1234-1234-1234-123456789abc"

func (s *scheduleBindingStub) ListPluginProviders(_ context.Context, options flameruntime.CallOptions) (*protocol.Page[protocol.PluginProviderSelection], error) {
	s.assertMeta(options.RequestMeta)
	s.providerReads++
	installation := scheduleInstallation
	if s.provider != "" {
		installation = s.provider
	}
	return protocol.NewPage([]protocol.PluginProviderSelection{{Role: "schedules", InstallationID: installation, Contribution: "schedules-provider"}}), nil
}
func (s *scheduleBindingStub) ListPlugins(_ context.Context, options flameruntime.CallOptions) (*protocol.Page[protocol.PluginInstallation], error) {
	s.assertMeta(options.RequestMeta)
	contracts, err := capability.Parse([]byte(schedule.CapabilityJSON))
	if err != nil {
		s.t.Fatal(err)
	}
	descriptor := contracts.Descriptor()
	if s.protocolOverride != "" {
		descriptor.Protocol = s.protocolOverride
	}
	return protocol.NewPage([]protocol.PluginInstallation{{ID: scheduleInstallation, Enabled: true, Selected: protocol.PluginRelease{Digest: "1111111111111111111111111111111111111111111111111111111111111111", Contributions: []protocol.PluginContribution{{ID: "schedules-provider", Contract: descriptor}}}}}), nil
}
func (s *scheduleBindingStub) CallPlugin(_ context.Context, request protocol.CallPluginRequest, options flameruntime.PluginCallOptions) (*protocol.CallPluginResponse, error) {
	s.assertMeta(options.RequestMeta)
	if request.InstallationID != scheduleInstallation || request.Target.Reference != scheduleInstallation || request.Target.Scope != "installation" || request.Contract != schedule.CapabilityJSON {
		s.t.Fatalf("unbound schedule query: %+v", request)
	}
	if request.Method == schedule.CreateV1Method {
		var input schedule.CreateV1Input
		if err := json.Unmarshal(request.Input, &input); err != nil {
			s.t.Fatal(err)
		}
		s.assertCommand("create", flameruntime.CommandOptions{RequestMeta: options.RequestMeta, IdempotencyKey: options.IdempotencyKey})
		s.created = input
		row := wireSchedule(s.now, "sch_created")
		row.Title, row.Instructions, row.Cron = input.Title, input.Instructions, input.Cron
		row.Provider, row.Model, row.ReasoningEffort = input.ModelSelection["provider"], input.ModelSelection["model"], input.ModelSelection["reasoningEffort"]
		if input.CWD != "" {
			row.Workspace = &protocol.WorkspaceRef{Path: input.CWD}
		}
		if s.createResult != nil {
			row = cloneSchedule(*s.createResult)
		}
		output, err := jsoninput.Encode(schedule.CreateV1Output{ID: row.ID, Title: row.Title, Instructions: row.Instructions, Cron: row.Cron, CWD: input.CWD, ModelSelection: input.ModelSelection, CreatedAt: row.CreatedAt, NextRunAt: *row.NextRunAt, Revision: row.Revision})
		if err != nil {
			s.t.Fatal(err)
		}
		return &protocol.CallPluginResponse{Output: output.JSON()}, nil
	}
	if request.Method == schedule.RunNowV1Method {
		var input schedule.RunNowV1Input
		if err := json.Unmarshal(request.Input, &input); err != nil {
			s.t.Fatal(err)
		}
		s.assertCommand("run:"+input.ID, flameruntime.CommandOptions{RequestMeta: options.RequestMeta, IdempotencyKey: options.IdempotencyKey})
		output, err := jsoninput.Encode(schedule.RunNowV1Output{SessionID: "ses_headless", RunID: "run_headless"})
		if err != nil {
			s.t.Fatal(err)
		}
		return &protocol.CallPluginResponse{Output: output.JSON()}, nil
	}
	if request.Method == schedule.DeleteV1Method {
		var input schedule.DeleteV1Input
		if err := json.Unmarshal(request.Input, &input); err != nil {
			s.t.Fatal(err)
		}
		s.assertCommand("delete:"+input.ID, flameruntime.CommandOptions{RequestMeta: options.RequestMeta, IdempotencyKey: options.IdempotencyKey})
		return &protocol.CallPluginResponse{Output: []byte(`{}`)}, nil
	}
	if request.Method != schedule.ListV1Method || options.IdempotencyKey != "" {
		s.t.Fatal("invalid schedule query")
	}
	s.queries = append(s.queries, request)
	var input schedule.ListV1Input
	if err := json.Unmarshal(request.Input, &input); err != nil {
		s.t.Fatal(err)
	}
	if input.Limit == nil || *input.Limit != schedulePageLimit {
		s.t.Fatalf("schedule page limit: %v", input.Limit)
	}
	query := protocol.PageQuery{Cursor: input.Cursor, Limit: new(int(*input.Limit))}
	var page *protocol.Page[protocol.Schedule]
	if s.list != nil {
		page = s.list(query)
	} else {
		first := wireSchedule(s.now.Add(time.Minute), "sch_2")
		if query.Cursor == "" {
			page = protocol.NewPageWithCursor([]protocol.Schedule{first}, "next")
		} else {
			page = protocol.NewPage([]protocol.Schedule{wireSchedule(s.now, "sch_1")})
		}
	}
	if page == nil {
		return nil, nil
	}
	value := schedule.ListV1Output{NextCursor: page.NextCursor}
	for _, row := range page.Data {
		snapshot := schedule.CreateV1Output{ID: row.ID, Title: row.Title, Instructions: row.Instructions, Cron: row.Cron, CreatedAt: row.CreatedAt, Revision: row.Revision, ModelSelection: map[string]string{"provider": row.Provider, "model": row.Model, "reasoningEffort": row.ReasoningEffort}}
		if row.Workspace != nil {
			snapshot.CWD = row.Workspace.Path
		}
		if row.NextRunAt != nil {
			snapshot.NextRunAt = *row.NextRunAt
		}
		if row.LastRunAt != nil {
			snapshot.LastRunAt = *row.LastRunAt
		}
		value.Rows = append(value.Rows, snapshot)
	}
	if value.Rows == nil {
		value.Rows = make([]schedule.CreateV1Output, 0)
	}
	encoded, err := jsoninput.Encode(value)
	if err != nil {
		s.t.Fatal(err)
	}
	return &protocol.CallPluginResponse{Output: encoded.JSON()}, nil
}

func (s *scheduleBindingStub) UpdateSchedule(_ context.Context, request protocol.UpdateScheduleRequest, options flameruntime.CommandOptions) (*protocol.Schedule, error) {
	s.assertCommand("update", options)
	s.updated = request
	s.updated.Title = clonePointer(request.Title)
	s.updated.Instructions = clonePointer(request.Instructions)
	s.updated.Workspace = cloneWorkspaceRef(request.Workspace)
	s.updated.Provider = clonePointer(request.Provider)
	s.updated.Model = clonePointer(request.Model)
	s.updated.ReasoningEffort = clonePointer(request.ReasoningEffort)
	s.updated.Cron = clonePointer(request.Cron)
	s.updated.Enabled = clonePointer(request.Enabled)
	if s.updateResult != nil {
		result := cloneSchedule(*s.updateResult)
		return &result, nil
	}
	updated := wireSchedule(s.now, request.ID)
	updated.Revision = request.ExpectedRevision + 1
	if request.Title != nil {
		updated.Title = *request.Title
	}
	if request.Instructions != nil {
		updated.Instructions = *request.Instructions
	}
	if request.Workspace != nil {
		updated.Workspace = cloneWorkspaceRef(request.Workspace)
	} else if request.WorkspaceMode == protocol.ScheduleWorkspaceDefault {
		updated.Workspace = nil
	}
	if request.Provider != nil {
		updated.Provider = *request.Provider
		updated.Model = *request.Model
	}
	if request.ReasoningEffort != nil {
		updated.ReasoningEffort = *request.ReasoningEffort
	}
	if request.Cron != nil {
		updated.Cron = *request.Cron
	}
	if request.Enabled != nil {
		updated.Enabled = *request.Enabled
		if !updated.Enabled {
			updated.NextRunAt = nil
		}
	}
	return &updated, nil
}

func (s *scheduleBindingStub) assertMeta(meta protocol.RequestMeta) {
	s.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		s.t.Fatalf("schedule request meta = %+v", meta)
	}
}

func (s *scheduleBindingStub) assertCommand(action string, options flameruntime.CommandOptions) {
	s.t.Helper()
	s.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		s.t.Fatal("schedule command has no idempotency key")
	}
	if _, duplicate := s.keys[options.IdempotencyKey]; duplicate {
		s.t.Fatalf("schedule command reused idempotency key %q", options.IdempotencyKey)
	}
	s.keys[options.IdempotencyKey] = struct{}{}
	s.actions = append(s.actions, action)
}

func wireSchedule(now time.Time, id string) protocol.Schedule {
	next := now.Add(time.Hour)
	return protocol.Schedule{
		ID: id, Title: "Review", Instructions: "review the repository", Cron: "0 * * * *",
		Enabled: true, NextRunAt: &next, CreatedAt: now, Revision: 1,
	}
}

func TestScheduleAdapterConsumesEveryOperationAndPaginates(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	stub := &scheduleBindingStub{t: t, now: now, keys: make(map[string]struct{})}
	runtime := &Connection{
		schedules: stub,
		plugins:   stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}

	listed, err := runtime.Schedules(t.Context())
	if err != nil || len(listed) != 2 || listed[0].ID != "sch_2" || listed[1].ID != "sch_1" {
		t.Fatalf("public Schedules = (%+v, %v)", listed, err)
	}
	candidate := schedule.CreateV1Input{
		Title: "Daily review", Instructions: "review everything", CWD: "/workspace", Enabled: true,
		ModelSelection: map[string]string{"provider": "deepseek", "model": "deepseek-v4-flash", "reasoningEffort": "high"}, Cron: "0 9 * * *",
	}
	creation, err := runtime.PrepareCreate(t.Context(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	createdReply, err := runtime.CallPlugin(t.Context(), creation, mutation.NewCommandID())
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.DecodeCreate(createdReply)
	if err != nil || created.ID != "sch_created" {
		t.Fatalf("Create = (%+v, %v)", created, err)
	}
	if stub.created.CWD != "/workspace" || stub.created.ModelSelection["model"] != candidate.ModelSelection["model"] || stub.created.ModelSelection["reasoningEffort"] != candidate.ModelSelection["reasoningEffort"] {
		t.Fatalf("create request = %+v", stub.created)
	}
	title := "Updated"
	updated, err := runtime.Update(t.Context(), protocol.UpdateScheduleRequest{ID: created.ID, ExpectedRevision: created.Revision, Title: &title})
	if err != nil || updated.Title != title || updated.Revision != created.Revision+1 {
		t.Fatalf("Update = (%+v, %v)", updated, err)
	}
	if stub.updated.ExpectedRevision != created.Revision || stub.updated.Title == nil || *stub.updated.Title != title {
		t.Fatalf("update request = %+v", stub.updated)
	}
	deletion, err := runtime.PrepareDelete(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CallPlugin(t.Context(), deletion, mutation.NewCommandID()); err != nil {
		t.Fatal(err)
	}
	runRequest, err := runtime.PrepareRunNow(t.Context(), "sch_1")
	if err != nil {
		t.Fatal(err)
	}
	runReply, err := runtime.CallPlugin(t.Context(), runRequest, mutation.NewCommandID())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := runtime.DecodeRunNow(runReply)
	if err != nil || handle.SessionID != "ses_headless" || handle.RunID != "run_headless" {
		t.Fatalf("RunNow = (%+v, %v)", handle, err)
	}
	want := []string{"create", "update", "delete:sch_created", "run:sch_1"}
	if len(stub.actions) != len(want) {
		t.Fatalf("actions = %v, want %v", stub.actions, want)
	}
	for index := range want {
		if stub.actions[index] != want[index] {
			t.Fatalf("actions = %v, want %v", stub.actions, want)
		}
	}
}

func TestScheduleAdapterProjectsWorkspaceChangeSemantics(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	stub := &scheduleBindingStub{t: t, now: now, keys: make(map[string]struct{})}
	runtime := &Connection{
		schedules: stub,
		plugins:   stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}

	request := protocol.UpdateScheduleRequest{
		ID: "sch_1", ExpectedRevision: 1, Workspace: &protocol.WorkspaceRef{Path: "/workspace/alias"},
	}
	_, err := runtime.Update(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if request.Workspace.Path != "/workspace/alias" {
		t.Fatalf("workspace resolution mutated the caller's request to %q", request.Workspace.Path)
	}
	if stub.updated.Workspace == nil || stub.updated.Workspace.Path != "/workspace" || stub.updated.WorkspaceMode != "" {
		t.Fatalf("bound workspace request = %+v", stub.updated)
	}

	_, err = runtime.Update(t.Context(), protocol.UpdateScheduleRequest{
		ID: "sch_1", ExpectedRevision: 2, WorkspaceMode: protocol.ScheduleWorkspaceDefault,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stub.updated.Workspace != nil || stub.updated.WorkspaceMode != protocol.ScheduleWorkspaceDefault {
		t.Fatalf("default workspace request = %+v", stub.updated)
	}
}

func TestScheduleAdapterRejectsAMutationForAnotherSchedule(t *testing.T) {
	t.Parallel()
	value := wireSchedule(time.Unix(1, 0), "sch_other")
	_, err := scheduleResult("update schedule", "sch_expected", &value, nil)
	requireRuntimeContractViolation(t, err)
}

func cloneSchedule(value protocol.Schedule) protocol.Schedule {
	value.Workspace = cloneWorkspaceRef(value.Workspace)
	value.LastRunAt = clonePointer(value.LastRunAt)
	value.NextRunAt = clonePointer(value.NextRunAt)
	return value
}

func cloneWorkspaceRef(value *protocol.WorkspaceRef) *protocol.WorkspaceRef {
	if value == nil {
		return nil
	}
	return &protocol.WorkspaceRef{Path: value.Path}
}

func TestScheduleReadKeepsOriginalProviderAcrossPages(t *testing.T) {
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	stub := &scheduleBindingStub{t: t, now: now, keys: map[string]struct{}{}}
	stub.list = func(query protocol.PageQuery) *protocol.Page[protocol.Schedule] {
		if query.Cursor == "" {
			stub.provider = "other"
			return protocol.NewPageWithCursor([]protocol.Schedule{wireSchedule(now, "sch_1")}, "next")
		}
		return protocol.NewPage([]protocol.Schedule{wireSchedule(now, "sch_2")})
	}
	caller := &Connection{plugins: stub, meta: requestMeta("test")}
	rows, err := caller.Schedules(t.Context())
	if err != nil || len(rows) != 2 || stub.providerReads != 1 || len(stub.queries) != 2 {
		t.Fatalf("frozen provider read: rows=%v, error=%v, selections=%d", rows, err, stub.providerReads)
	}
	for _, request := range stub.queries {
		if request.InstallationID != scheduleInstallation {
			t.Fatal("read retargeted a captured provider")
		}
	}
}

func TestScheduleReadRejectsDuplicateRowsAcrossPublicPages(t *testing.T) {
	stub := &scheduleBindingStub{t: t, now: time.Now(), keys: map[string]struct{}{}}
	stub.list = func(query protocol.PageQuery) *protocol.Page[protocol.Schedule] {
		if query.Cursor == "" {
			return protocol.NewPageWithCursor([]protocol.Schedule{wireSchedule(stub.now, "sch_1")}, "next")
		}
		return protocol.NewPage([]protocol.Schedule{wireSchedule(stub.now, "sch_1")})
	}
	caller := &Connection{plugins: stub, meta: requestMeta("test")}
	_, err := caller.Schedules(t.Context())
	requireRuntimeContractViolation(t, err)
}

func TestScheduleReadRefusesAnotherPublishedProtocol(t *testing.T) {
	stub := &scheduleBindingStub{t: t, protocolOverride: "retired"}
	caller := &Connection{plugins: stub, meta: requestMeta("test")}
	if _, err := caller.Schedules(t.Context()); err == nil || len(stub.queries) != 0 {
		t.Fatal("metadata protocol was rewritten before query admission")
	}
}

func TestScheduleRunNowRequiresACompleteCoreReference(t *testing.T) {
	r := &Connection{}
	for _, output := range []string{
		`{}`, `{"SessionID":"ses_original"}`, `{"SessionID":"bad session","RunID":"run_original"}`,
		`{"SessionID":"ses_original","RunID":"bad run"}`, `{"SessionID":"ses_original","RunID":"run_original","extra":true}`,
	} {
		if _, err := r.DecodeRunNow(&protocol.CallPluginResponse{Output: []byte(output)}); err == nil {
			t.Fatalf("invalid accepted reference became success: %s", output)
		}
	}
	if _, err := r.DecodeRunNow(nil); err == nil {
		t.Fatal("absent acknowledgement became success")
	}
	value, err := r.DecodeRunNow(&protocol.CallPluginResponse{Output: []byte(`{"SessionID":"ses_original","RunID":"run_original"}`)})
	if err != nil || value.SessionID != "ses_original" || value.RunID != "run_original" {
		t.Fatalf("accepted Core reference: %+v, %v", value, err)
	}
}

func TestScheduleCreateCapturesPublishedInputWithoutBorrowingCallerState(t *testing.T) {
	stub := &scheduleBindingStub{t: t}
	r := &Connection{plugins: stub, meta: requestMeta("test"), workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: "/resolved"}, ProjectRoot: "/resolved", Availability: protocol.WorkspaceAvailable,
	}}}
	input := schedule.CreateV1Input{Title: "original", Instructions: "inspect", CWD: "/authored", Cron: "@daily", Enabled: true, ModelSelection: map[string]string{"provider": "deepseek", "model": "original"}}
	request, err := r.PrepareCreate(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if input.CWD != "/authored" {
		t.Fatal("workspace resolution mutated the authored draft")
	}
	input.Title, input.CWD = "replacement", "/replacement"
	input.ModelSelection["model"] = "replacement"
	stub.provider = "replacement-installation"
	var captured schedule.CreateV1Input
	if err := json.Unmarshal(request.Input, &captured); err != nil {
		t.Fatal(err)
	}
	if captured.Title != "original" || captured.CWD != "/resolved" || captured.ModelSelection["model"] != "original" || !captured.Enabled || request.InstallationID != scheduleInstallation || request.Method != schedule.CreateV1Method || request.Contract != schedule.CapabilityJSON {
		t.Fatalf("creation changed its captured intent or owner: %+v, %+v", captured, request)
	}
}
