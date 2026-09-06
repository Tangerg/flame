package runtimebinding

import (
	"context"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type scheduleBindingStub struct {
	t            *testing.T
	now          time.Time
	actions      []string
	keys         map[string]struct{}
	created      protocol.CreateScheduleRequest
	updated      protocol.UpdateScheduleRequest
	createResult *protocol.Schedule
	updateResult *protocol.Schedule
	list         func(protocol.PageQuery) *protocol.Page[protocol.Schedule]
}

func (s *scheduleBindingStub) ListSchedules(_ context.Context, query protocol.PageQuery, options flameruntime.CallOptions) (*protocol.Page[protocol.Schedule], error) {
	s.assertMeta(options.RequestMeta)
	if query.Limit == nil || *query.Limit != schedulePageLimit {
		s.t.Fatalf("schedule page limit = %v", query.Limit)
	}
	if s.list != nil {
		return s.list(query), nil
	}
	first := wireSchedule(s.now.Add(time.Minute), "sch_2")
	if query.Cursor == "" {
		return protocol.NewPageWithCursor([]protocol.Schedule{first}, "next"), nil
	}
	second := wireSchedule(s.now, "sch_1")
	return protocol.NewPage([]protocol.Schedule{second}), nil
}

func (s *scheduleBindingStub) CreateSchedule(_ context.Context, request protocol.CreateScheduleRequest, options flameruntime.CommandOptions) (*protocol.Schedule, error) {
	s.assertCommand("create", options)
	s.created = request
	s.created.Workspace = cloneWorkspaceRef(request.Workspace)
	if s.createResult != nil {
		result := cloneSchedule(*s.createResult)
		return &result, nil
	}
	created := wireSchedule(s.now, "sch_created")
	created.Title, created.Instructions, created.Cron = request.Title, request.Instructions, request.Cron
	created.Provider, created.Model, created.ReasoningEffort = request.Provider, request.Model, request.ReasoningEffort
	created.Workspace = cloneWorkspaceRef(request.Workspace)
	return &created, nil
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

func (s *scheduleBindingStub) DeleteSchedule(_ context.Context, request protocol.DeleteScheduleRequest, options flameruntime.CommandOptions) error {
	s.assertCommand("delete:"+request.ID, options)
	return nil
}

func (s *scheduleBindingStub) RunScheduleNow(_ context.Context, request protocol.RunScheduleNowRequest, options flameruntime.CommandOptions) (*protocol.RunScheduleNowResponse, error) {
	s.assertCommand("run:"+request.ID, options)
	return &protocol.RunScheduleNowResponse{SessionID: "ses_headless", RunID: "run_headless"}, nil
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
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}

	listed, err := runtime.Schedules(t.Context())
	if err != nil || len(listed) != 2 || listed[0].ID != "sch_2" || listed[1].ID != "sch_1" {
		t.Fatalf("Schedules = (%+v, %v)", listed, err)
	}
	candidate := protocol.CreateScheduleRequest{
		Title: "Daily review", Instructions: "review everything", Workspace: &protocol.WorkspaceRef{Path: "/workspace"},
		Provider: "deepseek", Model: "deepseek-v4-flash", ReasoningEffort: "high", Cron: "0 9 * * *",
	}
	created, err := runtime.Create(t.Context(), candidate)
	if err != nil || created.ID != "sch_created" {
		t.Fatalf("Create = (%+v, %v)", created, err)
	}
	if stub.created.Workspace == nil || stub.created.Workspace.Path != "/workspace" ||
		stub.created.Model != candidate.Model || stub.created.ReasoningEffort != candidate.ReasoningEffort {
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
	if deleteErr := runtime.Delete(t.Context(), created.ID); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	handle, err := runtime.RunNow(t.Context(), "sch_1")
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

func TestScheduleAdapterRejectsPagesOutsideRuntimeOrder(t *testing.T) {
	t.Parallel()
	created := time.Unix(10, 0).UTC()
	for _, test := range []struct {
		name   string
		first  protocol.Schedule
		second protocol.Schedule
	}{
		{
			name:   "creation time ascends across pages",
			first:  wireSchedule(created, "sch_old"),
			second: wireSchedule(created.Add(time.Second), "sch_new"),
		},
		{
			name:   "equal-time identity ascends across pages",
			first:  wireSchedule(created, "sch_a"),
			second: wireSchedule(created, "sch_b"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &scheduleBindingStub{t: t, list: func(query protocol.PageQuery) *protocol.Page[protocol.Schedule] {
				if query.Cursor == "" {
					return protocol.NewPageWithCursor([]protocol.Schedule{test.first}, "next")
				}
				return protocol.NewPage([]protocol.Schedule{test.second})
			}}
			runtime := &Connection{schedules: stub, meta: requestMeta("test")}
			_, err := runtime.Schedules(t.Context())
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestScheduleAdapterProjectsWorkspaceChangeSemantics(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	stub := &scheduleBindingStub{t: t, now: now, keys: make(map[string]struct{})}
	runtime := &Connection{
		schedules: stub,
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

func TestScheduleAdapterRejectsMismatchedMutationAcknowledgements(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	newRuntime := func(stub *scheduleBindingStub) *Connection {
		return &Connection{
			schedules: stub,
			workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
				Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace",
				Availability: protocol.WorkspaceAvailable,
			}},
			meta: requestMeta("test"),
		}
	}

	t.Run("create", func(t *testing.T) {
		request := protocol.CreateScheduleRequest{
			Title: "Expected", Instructions: "review everything", Cron: "0 9 * * *",
		}
		result := wireSchedule(now, "sch_created")
		result.Title = "Other"
		result.Instructions, result.Cron = request.Instructions, request.Cron
		stub := &scheduleBindingStub{t: t, now: now, keys: make(map[string]struct{}), createResult: &result}
		_, err := newRuntime(stub).Create(t.Context(), request)
		requireRuntimeContractViolation(t, err)
	})

	t.Run("update", func(t *testing.T) {
		title := "Expected"
		request := protocol.UpdateScheduleRequest{ID: "sch_1", ExpectedRevision: 1, Title: &title}
		result := wireSchedule(now, request.ID)
		result.Revision = request.ExpectedRevision + 1
		result.Title = "Other"
		stub := &scheduleBindingStub{t: t, now: now, keys: make(map[string]struct{}), updateResult: &result}
		_, err := newRuntime(stub).Update(t.Context(), request)
		requireRuntimeContractViolation(t, err)
	})
}

func TestScheduleAdapterRejectsImpossibleScheduleProjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*protocol.Schedule)
	}{
		{name: "missing creation time", mutate: func(value *protocol.Schedule) { value.CreatedAt = time.Time{} }},
		{name: "disabled with next run", mutate: func(value *protocol.Schedule) { value.Enabled = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := wireSchedule(time.Unix(1, 0), "sch_1")
			test.mutate(&value)
			_, err := scheduleResult("update schedule", value.ID, &value, nil)
			requireRuntimeContractViolation(t, err)
		})
	}
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
