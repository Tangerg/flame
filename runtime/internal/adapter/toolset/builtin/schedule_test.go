package builtin

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"

	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	scheduleapp "github.com/Tangerg/flame/runtime/internal/application/schedules"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	scheduledomain "github.com/Tangerg/flame/runtime/internal/domain/schedule"
)

func TestSchedulesCreateListDelete(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := BuildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byName := scheduleByName(tools)

	body, err := callTextTool(t.Context(), byName["create_schedule"], `{"title":"daily","instructions":"summarize","cron":"0 9 * * *"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created scheduleResponse
	if unmarshalErr := json.Unmarshal([]byte(body), &created); unmarshalErr != nil {
		t.Fatalf("unmarshal create: %v", unmarshalErr)
	}
	if created.Schedule.ScheduleID == "" || created.Schedule.NextRunAt == "" || created.Schedule.Instructions != "summarize" {
		t.Fatalf("created schedule = %+v", created.Schedule)
	}

	listBody, err := callTextTool(t.Context(), byName["list_schedules"], `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var listed scheduleListResponse
	if err := json.Unmarshal([]byte(listBody), &listed); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listed.Schedules) != 1 {
		t.Fatalf("list = %+v, want 1 schedule", listed.Schedules)
	}

	if _, err := callTextTool(t.Context(), byName["delete_schedule"], `{"schedule_id":"`+created.Schedule.ScheduleID+`"}`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := reg.Get(t.Context(), created.Schedule.ScheduleID); !errors.Is(err, scheduledomain.ErrNotFound) {
		t.Fatalf("get deleted err = %v, want ErrNotFound", err)
	}
}

func TestSchedulesDisabledCapabilityBuildsNoTools(t *testing.T) {
	tools, err := BuildSchedules(scheduleapp.Disabled())
	if err != nil || len(tools) != 0 {
		t.Fatalf("BuildSchedules(disabled) = (%d tools, %v), want none", len(tools), err)
	}
}

func TestSchedulesListIsBoundedAndContinuable(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	createdAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	for index := range 101 {
		id := fmt.Sprintf("sch_%03d", index)
		scheduled, err := scheduledomain.Restore(scheduledomain.Snapshot{
			ID: id, Instructions: "review", Cron: "@daily",
			CreatedAt: createdAt.Add(time.Duration(index) * time.Second), Revision: 1,
		})
		if err != nil {
			t.Fatalf("restore %s: %v", id, err)
		}
		reg.items[id] = scheduled
	}
	tools, err := BuildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	list := scheduleByName(tools)["list_schedules"]
	firstBody, err := callTextTool(t.Context(), list, `{}`)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	var first scheduleListResponse
	if err := json.Unmarshal([]byte(firstBody), &first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Schedules) != 100 || first.NextCursor == "" {
		t.Fatalf("first page = %d rows, cursor %q; want 100 and continuation", len(first.Schedules), first.NextCursor)
	}
	secondBody, err := callTextTool(t.Context(), list, `{"cursor":`+strconv.Quote(first.NextCursor)+`}`)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	var second scheduleListResponse
	if err := json.Unmarshal([]byte(secondBody), &second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(second.Schedules) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %d rows, cursor %q; want final row", len(second.Schedules), second.NextCursor)
	}
}

func TestSchedulesHaveActionSpecificStrictSchemas(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := BuildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byName := scheduleByName(tools)
	if _, err := callTextTool(t.Context(), byName["create_schedule"], `{"cron":"0 9 * * *"}`); err == nil {
		t.Fatal("create without instructions succeeded")
	}
	if _, err := callTextTool(t.Context(), byName["list_schedules"], `{"op":"list"}`); err == nil {
		t.Fatal("list accepted an obsolete op field")
	}
	if _, err := callTextTool(t.Context(), byName["delete_schedule"], `{"id":"sch_old"}`); err == nil {
		t.Fatal("delete accepted obsolete id field")
	}
}

func TestSchedulesCreateRejectsUnavailableWorkdir(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := BuildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	_, err = callTextTool(t.Context(), scheduleByName(tools)["create_schedule"], `{"instructions":"summarize","cron":"0 9 * * *","workspace_path":"`+missing+`"}`)
	if !errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		t.Fatalf("create cwd err = %v, want ErrCWDUnavailable", err)
	}
	if len(reg.items) != 0 {
		t.Fatalf("created %d schedule(s), want none", len(reg.items))
	}
}

func scheduleByName(tools []toolcontract.Tool) map[string]toolcontract.Tool {
	out := make(map[string]toolcontract.Tool, len(tools))
	for _, candidate := range tools {
		out[candidate.Definition().Name] = candidate
	}
	return out
}

type memoryScheduleRegistry struct {
	items map[string]scheduledomain.Schedule
}

func newMemoryScheduleRegistry() *memoryScheduleRegistry {
	return &memoryScheduleRegistry{items: map[string]scheduledomain.Schedule{}}
}

func newTestScheduleCoordinator(reg scheduleapp.ManagementStore) *scheduleapp.Coordinator {
	value, err := scheduleapp.New(scheduleapp.Dependencies{
		Store:      reg,
		Paths:      workspaceadapter.Resolver{},
		Models:     scheduleModelAdmitter{},
		Identities: scheduleTestIdentities{},
	})
	if err != nil {
		panic(err)
	}
	return value
}

type scheduleModelAdmitter struct{}

func (scheduleModelAdmitter) AdmitSelection(modelref.Selection) error { return nil }

type scheduleTestIdentities struct{}

func (scheduleTestIdentities) NewScheduleID() string { return "sch_test_1" }

func (m *memoryScheduleRegistry) ListPage(_ context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]scheduledomain.Schedule, error) {
	out := make([]scheduledomain.Schedule, 0, len(m.items))
	for _, sc := range m.items {
		if !afterCreatedAt.IsZero() || afterID != "" {
			if sc.CreatedAt().After(afterCreatedAt) || (sc.CreatedAt().Equal(afterCreatedAt) && sc.ID() >= afterID) {
				continue
			}
		}
		out = append(out, sc)
	}
	slices.SortFunc(out, func(left, right scheduledomain.Schedule) int {
		if !left.CreatedAt().Equal(right.CreatedAt()) {
			if left.CreatedAt().After(right.CreatedAt()) {
				return -1
			}
			return 1
		}
		return cmp.Compare(right.ID(), left.ID())
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memoryScheduleRegistry) Get(_ context.Context, id string) (scheduledomain.Schedule, error) {
	sc, ok := m.items[id]
	if !ok {
		return scheduledomain.Schedule{}, scheduledomain.ErrNotFound
	}
	return sc, nil
}

func (m *memoryScheduleRegistry) Insert(_ context.Context, sc scheduledomain.Schedule) error {
	m.items[sc.ID()] = sc
	return nil
}

func (m *memoryScheduleRegistry) Update(_ context.Context, sc scheduledomain.Schedule, _ uint64) (scheduledomain.Schedule, error) {
	if _, ok := m.items[sc.ID()]; !ok {
		return scheduledomain.Schedule{}, scheduledomain.ErrNotFound
	}
	m.items[sc.ID()] = sc
	return sc, nil
}

func (m *memoryScheduleRegistry) Delete(_ context.Context, id string) (bool, error) {
	_, found := m.items[id]
	delete(m.items, id)
	return found, nil
}

func (m *memoryScheduleRegistry) Due(context.Context, time.Time, int) ([]scheduledomain.Schedule, error) {
	return nil, nil
}

func (m *memoryScheduleRegistry) RecordRun(context.Context, scheduledomain.RunRecord) error {
	return nil
}
