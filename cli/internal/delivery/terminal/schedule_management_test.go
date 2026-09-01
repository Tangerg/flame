package terminal

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

type scheduleServiceStub struct {
	mu        sync.Mutex
	schedules []protocol.Schedule
	created   chan protocol.CreateScheduleRequest
	updated   chan protocol.UpdateScheduleRequest
	deleted   chan string
	run       chan string
	reads     atomic.Int32
	now       time.Time
}

type blockingScheduleRunService struct {
	*scheduleServiceStub
	started  chan string
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingScheduleRunService) RunNow(ctx context.Context, id string) (protocol.RunScheduleNowResponse, error) {
	select {
	case b.started <- id:
	default:
	}
	select {
	case <-b.release:
		return b.scheduleServiceStub.RunNow(ctx, id)
	case <-ctx.Done():
		select {
		case b.canceled <- struct{}{}:
		default:
		}
		return protocol.RunScheduleNowResponse{}, context.Cause(ctx)
	}
}

func newScheduleServiceStub() *scheduleServiceStub {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	return &scheduleServiceStub{
		schedules: []protocol.Schedule{{
			ID: "sch_review", Title: "Repository review", Instructions: "review the repository",
			Workspace: &protocol.WorkspaceRef{Path: "/workspace"}, Cron: "0 * * * *", Enabled: true,
			NextRunAt: &next, CreatedAt: now, Revision: 1,
		}},
		created: make(chan protocol.CreateScheduleRequest, 1), updated: make(chan protocol.UpdateScheduleRequest, 4),
		deleted: make(chan string, 1), run: make(chan string, 1), now: now,
	}
}

func (s *scheduleServiceStub) Schedules(context.Context) ([]protocol.Schedule, error) {
	s.reads.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]protocol.Schedule, len(s.schedules))
	for index, scheduled := range s.schedules {
		result[index] = cloneSchedule(scheduled)
	}
	return result, nil
}

func (s *scheduleServiceStub) Create(_ context.Context, request protocol.CreateScheduleRequest) (protocol.Schedule, error) {
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.Schedule{}, err
	}
	s.created <- request
	next := s.now.Add(2 * time.Hour)
	created := protocol.Schedule{
		ID: "sch_created", Title: request.Title, Instructions: request.Instructions,
		Workspace: cloneWorkspaceRef(request.Workspace), Provider: request.Provider, Model: request.Model,
		ReasoningEffort: request.ReasoningEffort, Cron: request.Cron,
		Enabled: true, NextRunAt: &next, CreatedAt: s.now, Revision: 1,
	}
	s.mu.Lock()
	s.schedules = append(s.schedules, created)
	s.mu.Unlock()
	return cloneSchedule(created), nil
}

func (s *scheduleServiceStub) Update(_ context.Context, request protocol.UpdateScheduleRequest) (protocol.Schedule, error) {
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.Schedule{}, err
	}
	s.updated <- request
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.schedules {
		scheduled := &s.schedules[index]
		if scheduled.ID != request.ID {
			continue
		}
		if scheduled.Revision != request.ExpectedRevision {
			return protocol.Schedule{}, errors.New("revision conflict")
		}
		applyScheduleUpdate(scheduled, request)
		scheduled.Revision++
		return cloneSchedule(*scheduled), nil
	}
	return protocol.Schedule{}, errors.New("schedule not found")
}

func (s *scheduleServiceStub) Delete(_ context.Context, id string) error {
	s.deleted <- id
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.schedules {
		if s.schedules[index].ID == id {
			s.schedules = append(s.schedules[:index], s.schedules[index+1:]...)
			return nil
		}
	}
	return errors.New("schedule not found")
}

func (s *scheduleServiceStub) RunNow(_ context.Context, id string) (protocol.RunScheduleNowResponse, error) {
	s.run <- id
	return protocol.RunScheduleNowResponse{SessionID: "ses_scheduled", RunID: "run_scheduled"}, nil
}

func cloneSchedule(scheduled protocol.Schedule) protocol.Schedule {
	scheduled.Workspace = cloneWorkspaceRef(scheduled.Workspace)
	if scheduled.LastRunAt != nil {
		scheduled.LastRunAt = new(*scheduled.LastRunAt)
	}
	if scheduled.NextRunAt != nil {
		scheduled.NextRunAt = new(*scheduled.NextRunAt)
	}
	return scheduled
}

func cloneWorkspaceRef(workspace *protocol.WorkspaceRef) *protocol.WorkspaceRef {
	if workspace == nil {
		return nil
	}
	return &protocol.WorkspaceRef{Path: workspace.Path}
}

func applyScheduleUpdate(scheduled *protocol.Schedule, request protocol.UpdateScheduleRequest) {
	if request.Title != nil {
		scheduled.Title = *request.Title
	}
	if request.Instructions != nil {
		scheduled.Instructions = *request.Instructions
	}
	if request.Workspace != nil {
		scheduled.Workspace = cloneWorkspaceRef(request.Workspace)
	} else if request.WorkspaceMode == protocol.ScheduleWorkspaceDefault {
		scheduled.Workspace = nil
	}
	if request.Provider != nil {
		scheduled.Provider, scheduled.Model = *request.Provider, *request.Model
	}
	if request.ReasoningEffort != nil {
		scheduled.ReasoningEffort = *request.ReasoningEffort
	}
	if request.Cron != nil {
		scheduled.Cron = *request.Cron
	}
	if request.Enabled != nil {
		scheduled.Enabled = *request.Enabled
		if scheduled.Enabled {
			next := scheduled.CreatedAt.Add(time.Hour)
			scheduled.NextRunAt = &next
		} else {
			scheduled.NextRunAt = nil
		}
	}
}

func TestScheduleCatalogReader(t *testing.T) {
	service := newScheduleServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service})
	host.Shows(t, "Ask flame")
	host.Type("/schedules")
	host.Press(input.Enter)
	host.Shows(t, "Repository review")
	host.Shows(t, "sch_review")
	stop()
}

func TestScheduleCreateFormSurvivesExtremeResize(t *testing.T) {
	service := newScheduleServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service})
	host.Shows(t, "Ask flame")
	host.Type("/schedule-create")
	host.Press(input.Enter)
	host.Shows(t, "Create scheduled run")
	host.Type("Daily audit")
	host.Press(input.Tab)
	host.Type("audit the repository")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Type("deepseek")
	host.Press(input.Enter)
	host.Shows(t, "provider and model must both be set or both be empty")
	select {
	case candidate := <-service.created:
		t.Fatalf("incomplete model selection reached the service: %+v", candidate)
	default:
	}
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("schedule form did not survive a minimal viewport")
	}
	host.Shows(t, "Create scheduled run")
	host.Press(input.Tab)
	host.Type("deepseek-v4-flash")
	host.Press(input.Enter)
	host.Shows(t, "Daily audit")
	created := awaitValue(t, service.created, "schedule creation")
	if created.Instructions != "audit the repository" || created.Cron != "0 9 * * 1-5" || created.Workspace == nil ||
		created.Provider != "deepseek" || created.Model != "deepseek-v4-flash" {
		t.Fatalf("created schedule candidate = %+v", created)
	}
	stop()
}

func TestScheduleFormDoesNotNormalizeModelIdentity(t *testing.T) {
	draft := scheduleFormDraft{
		instructions: "review the repository",
		provider:     " deepseek",
		model:        "deepseek-v4-flash",
		cron:         defaultScheduleCron,
	}
	if _, err := draft.candidate(); err == nil {
		t.Fatal("schedule create form normalized a provider identity")
	}

	original := protocol.Schedule{
		ID: "sch_review", Instructions: "review the repository", Provider: "deepseek",
		Model: "deepseek-v4-flash", Cron: defaultScheduleCron, Enabled: true, Revision: 1,
	}
	update := newScheduleFormDraft(scheduleFormUpdate, original, "")
	update.provider = " deepseek"
	if _, _, err := update.patch(original); err == nil {
		t.Fatal("schedule update form normalized a provider identity")
	}
	if err := validateScheduleModelPair(" ", ""); err == nil {
		t.Fatal("schedule form treated whitespace as an absent identity")
	}
}

func TestWorkspaceReplacementRetiresAPresentedScheduleForm(t *testing.T) {
	backend := runtimefixture.New()
	service := newScheduleServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, Schedules: service, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime invalidation subscription")
	host.Type("/schedule-create")
	host.Press(input.Enter)
	host.Shows(t, "Create scheduled run")

	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	replacementWorkspace := filepath.Join(t.TempDir(), "replacement")
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Workspace: &replacementWorkspace,
		ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "workspace replacement invalidation")
	host.Hides(t, "Create scheduled run")
	host.Press(input.Enter)
	select {
	case candidate := <-service.created:
		t.Fatalf("retired schedule form created %+v", candidate)
	default:
	}
	stop()
}

func TestScheduleMutationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	baseService := newScheduleServiceStub()
	service := &blockingScheduleRunService{
		scheduleServiceStub: baseService,
		started:             make(chan string, 1),
		release:             make(chan struct{}),
		canceled:            make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)

	backend := runtimefixture.New()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, Schedules: service, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/schedule-run sch_review")
	host.Press(input.Enter)
	if id := awaitValue(t, service.started, "schedule run mutation"); id != "sch_review" {
		t.Fatalf("schedule run id = %q, want sch_review", id)
	}

	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: protocol.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	title := "Schedule refresh installed"
	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	host.Shows(t, title)
	select {
	case <-service.canceled:
		t.Fatal("same-session projection replacement canceled the application-owned schedule mutation")
	default:
	}

	release()
	host.Shows(t, "schedule started · session ses_scheduled · run run_scheduled")
	if id := awaitValue(t, service.run, "completed schedule run mutation"); id != "sch_review" {
		t.Fatalf("completed schedule id = %q, want sch_review", id)
	}
	stop()
}

func TestScheduleEditEnableRunAndDeleteCommands(t *testing.T) {
	t.Run("edit", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service})
		host.Shows(t, "Ask flame")
		host.Type("/schedule-edit sch_review")
		host.Press(input.Enter)
		host.Shows(t, "Edit scheduled run · sch_review")
		host.Type(" updated")
		host.Press(input.Enter)
		host.Shows(t, "Repository review updated")
		patch := awaitValue(t, service.updated, "schedule edit")
		if patch.Title == nil || *patch.Title != "Repository review updated" {
			t.Fatalf("edit patch = %+v", patch)
		}
		stop()
	})

	t.Run("disable", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service})
		host.Shows(t, "Ask flame")
		host.Type("/schedule-disable sch_review")
		host.Press(input.Enter)
		host.Shows(t, "status   disabled")
		patch := awaitValue(t, service.updated, "schedule disable")
		if patch.Enabled == nil || *patch.Enabled {
			t.Fatalf("disable patch = %+v", patch)
		}
		stop()
	})

	t.Run("enable", func(t *testing.T) {
		service := newScheduleServiceStub()
		service.schedules[0].Enabled = false
		service.schedules[0].NextRunAt = nil
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service})
		host.Shows(t, "Ask flame")
		host.Type("/schedule-enable sch_review")
		host.Press(input.Enter)
		host.Shows(t, "status   enabled")
		patch := awaitValue(t, service.updated, "schedule enable")
		if patch.Enabled == nil || !*patch.Enabled {
			t.Fatalf("enable patch = %+v", patch)
		}
		stop()
	})

	t.Run("run now", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service})
		host.Shows(t, "Ask flame")
		host.Type("/schedule-run sch_review")
		host.Press(input.Enter)
		host.Shows(t, "session ses_scheduled · run run_scheduled")
		if id := awaitValue(t, service.run, "schedule run"); id != "sch_review" {
			t.Fatalf("run schedule id = %q", id)
		}
		stop()
	})

	t.Run("delete", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service})
		host.Shows(t, "Ask flame")
		host.Type("/schedule-delete sch_review")
		host.Press(input.Enter)
		host.Shows(t, "Delete scheduled run")
		host.Press(input.Down)
		host.Press(input.Enter)
		host.Shows(t, "none configured")
		if id := awaitValue(t, service.deleted, "schedule deletion"); id != "sch_review" {
			t.Fatalf("deleted schedule id = %q", id)
		}
		stop()
	})
}

func TestSchedulesChangedRefetchesOnlyTheOpenScheduleReader(t *testing.T) {
	service := newScheduleServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.SchedulesChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Schedules: service, Changes: source})
	host.Shows(t, "Ask flame")
	subscription := awaitValue(t, source.subscription, "schedule invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != changefeed.SchedulesChanged {
		t.Fatalf("schedule subscription = %+v", subscription)
	}
	host.Type("/schedules")
	host.Press(input.Enter)
	host.Shows(t, "Repository review")
	baseline := service.reads.Load()
	service.mu.Lock()
	service.schedules[0].Title = "Updated repository review"
	service.mu.Unlock()
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SchedulesChanged), Sequence: 1,
		ScheduleIDs: []string{"sch_review"},
	}
	awaitSignal(t, source.applied, "schedules.changed delivery")
	host.Shows(t, "Updated repository review")
	if service.reads.Load() <= baseline {
		t.Fatal("schedules.changed did not refetch the open schedule projection")
	}
	stop()
}

func TestScheduleDraftClearsWorkspaceBindingAndRejectsAmbiguousIdentity(t *testing.T) {
	t.Parallel()
	original := newScheduleServiceStub().schedules[0]
	draft := newScheduleFormDraft(scheduleFormUpdate, original, "")
	draft.workspace = ""
	request, changed, err := draft.patch(original)
	if err != nil || !changed || request.WorkspaceMode != protocol.ScheduleWorkspaceDefault {
		t.Fatalf("workspace clearing request = (%+v, %v, %v)", request, changed, err)
	}
	duplicate := cloneSchedule(original)
	duplicate.ID = "sch_review_other"
	if _, err := resolveSchedule([]protocol.Schedule{original, duplicate}, "sch_rev"); err == nil {
		t.Fatal("ambiguous schedule prefix was accepted")
	}
}
