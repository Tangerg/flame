package terminal

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/capability"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/schedule"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"
)

type scheduleServiceStub struct {
	mu        sync.Mutex
	schedules []schedule.CreateV1Output
	created   chan schedule.CreateV1Input
	updated   chan schedule.UpdateV1Input
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

func (b *blockingScheduleRunService) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID replay.CommandID) (*protocol.CallPluginResponse, error) {
	var input struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
		return nil, err
	}
	id := input.ID
	select {
	case b.started <- id:
	default:
	}
	select {
	case <-b.release:
		return b.scheduleServiceStub.CallPlugin(ctx, request, commandID)
	case <-ctx.Done():
		select {
		case b.canceled <- struct{}{}:
		default:
		}
		return nil, context.Cause(ctx)
	}
}

func newScheduleServiceStub() *scheduleServiceStub {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	return &scheduleServiceStub{schedules: []schedule.CreateV1Output{{ID: "sch_review", Title: "Repository review", Instructions: "review the repository", CWD: "/workspace", Cron: "0 * * * *", NextRunAt: next, CreatedAt: now, Revision: 1}}, created: make(chan schedule.CreateV1Input, 1), updated: make(chan schedule.UpdateV1Input, 4), deleted: make(chan string, 1), run: make(chan string, 1), now: now}
}

func (s *scheduleServiceStub) Schedules(context.Context) ([]schedule.CreateV1Output, error) {
	s.reads.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]schedule.CreateV1Output, len(s.schedules))
	for index, scheduled := range s.schedules {
		result[index] = cloneSchedule(scheduled)
	}
	return result, nil
}

func (s *scheduleServiceStub) create(request schedule.CreateV1Input) (schedule.CreateV1Output, error) {
	s.created <- request
	created := schedule.CreateV1Output{ID: "sch_created", Title: request.Title, Instructions: request.Instructions, CWD: request.CWD, ModelSelection: request.ModelSelection, Cron: request.Cron, NextRunAt: s.now.Add(2 * time.Hour), CreatedAt: s.now, Revision: 1}
	s.mu.Lock()
	s.schedules = append(s.schedules, created)
	s.mu.Unlock()
	return cloneSchedule(created), nil
}

func (s *scheduleServiceStub) update(request schedule.UpdateV1Input) (schedule.CreateV1Output, error) {
	s.updated <- request
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.schedules {
		scheduled := &s.schedules[index]
		if scheduled.ID != request.ID {
			continue
		}
		if scheduled.Revision != request.ExpectedRevision {
			return schedule.CreateV1Output{}, errors.New("revision conflict")
		}
		applyScheduleUpdate(scheduled, request)
		scheduled.Revision++
		return cloneSchedule(*scheduled), nil
	}
	return schedule.CreateV1Output{}, errors.New("schedule not found")
}

func boundScheduleRequest(method string, input any) (protocol.CallPluginRequest, error) {
	encoded, err := jsoninput.Encode(input)
	return protocol.CallPluginRequest{InstallationID: "12345678-1234-1234-1234-123456789abc", Digest: "1111111111111111111111111111111111111111111111111111111111111111", Contribution: "schedules-provider", Contract: schedule.CapabilityJSON, Method: method, Target: capability.Target{Scope: "installation", Reference: "12345678-1234-1234-1234-123456789abc"}, Input: string(encoded.JSON())}, err
}
func (s *scheduleServiceStub) PrepareCreate(_ context.Context, request schedule.CreateV1Input) (protocol.CallPluginRequest, error) {
	return boundScheduleRequest(schedule.CreateV1Method, request)
}
func (s *scheduleServiceStub) PrepareDelete(_ context.Context, id string) (protocol.CallPluginRequest, error) {
	return boundScheduleRequest(schedule.DeleteV1Method, schedule.DeleteV1Input{ID: id})
}
func (s *scheduleServiceStub) DecodeCreate(reply *protocol.CallPluginResponse) (schedule.CreateV1Output, error) {
	var value schedule.CreateV1Output
	if err := json.Unmarshal([]byte(reply.Output), &value); err != nil {
		return schedule.CreateV1Output{}, err
	}
	return value, nil
}

func (s *scheduleServiceStub) CallPlugin(_ context.Context, request protocol.CallPluginRequest, _ replay.CommandID) (*protocol.CallPluginResponse, error) {
	if request.Method == schedule.CreateV1Method {
		var input schedule.CreateV1Input
		if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
			return nil, err
		}
		row, err := s.create(input)
		if err != nil {
			return nil, err
		}
		output, err := jsoninput.Encode(schedule.CreateV1Output{ID: row.ID, Title: row.Title, Instructions: row.Instructions, CWD: input.CWD, Cron: row.Cron, ModelSelection: input.ModelSelection, NextRunAt: row.NextRunAt, CreatedAt: row.CreatedAt, Revision: row.Revision})
		return &protocol.CallPluginResponse{Output: string(output.JSON())}, err
	}
	if request.Method == schedule.UpdateV1Method {
		var input schedule.UpdateV1Input
		if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
			return nil, err
		}
		value, err := s.update(input)
		if err != nil {
			return nil, err
		}
		encoded, err := jsoninput.Encode(value)
		return &protocol.CallPluginResponse{Output: string(encoded.JSON())}, err
	}
	var input struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal([]byte(request.Input), &input); err != nil {
		return nil, err
	}
	if request.Method == "flame.schedules/runNow.v1" {
		s.run <- input.ID
		return &protocol.CallPluginResponse{Output: `{"SessionID":"ses_scheduled","RunID":"run_scheduled"}`}, nil
	}
	if err := s.delete(input.ID); err != nil {
		return nil, err
	}
	return &protocol.CallPluginResponse{Output: `{}`}, nil
}
func (s *scheduleServiceStub) delete(id string) error {
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

func (s *scheduleServiceStub) PrepareRunNow(ctx context.Context, id string) (protocol.CallPluginRequest, error) {
	request, err := s.PrepareDelete(ctx, id)
	request.Method = "flame.schedules/runNow.v1"
	return request, err
}
func (s *scheduleServiceStub) DecodeRunNow(reply *protocol.CallPluginResponse) (conversation.RunReference, error) {
	var value schedule.RunNowV1Output
	err := json.Unmarshal([]byte(reply.Output), &value)
	return conversation.RunReference{SessionID: value.SessionID, RunID: value.RunID}, err
}

func cloneSchedule(value schedule.CreateV1Output) schedule.CreateV1Output {
	return value
}

func applyScheduleUpdate(value *schedule.CreateV1Output, request schedule.UpdateV1Input) {
	if request.Title != nil {
		value.Title = *request.Title
	}
	if request.Instructions != nil {
		value.Instructions = *request.Instructions
	}
	if request.CWD != nil {
		value.CWD = *request.CWD
	}
	if request.ModelSelection.Provider != nil {
		value.ModelSelection.Provider = *request.ModelSelection.Provider
	}
	if request.ModelSelection.Model != nil {
		value.ModelSelection.Model = *request.ModelSelection.Model
	}
	if request.Cron != nil {
		value.Cron = *request.Cron
	}
	if request.Enabled != nil {
		if *request.Enabled {
			value.NextRunAt = value.CreatedAt.Add(time.Hour)
		} else {
			value.NextRunAt = time.Time{}
		}
	}
}

func TestScheduleCatalogReader(t *testing.T) {
	service := newScheduleServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
	host.Shows(t, "Ask flame")
	host.Type("/schedules")
	host.Press(input.Enter)
	host.Shows(t, "Repository review")
	host.Shows(t, "sch_review")
	stop()
}

func TestScheduleCreateFormSurvivesExtremeResize(t *testing.T) {
	service := newScheduleServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
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
	if created.Instructions != "audit the repository" || created.Cron != "0 9 * * 1-5" || created.CWD == "" ||
		created.ModelSelection.Provider != "deepseek" || created.ModelSelection.Model != "deepseek-v4-flash" {
		t.Fatalf("created schedule candidate = %+v", created)
	}
	stop()
}

func TestScheduleCreateFormLeavesCronGrammarToRuntime(t *testing.T) {
	service := newScheduleServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
	host.Shows(t, "Ask flame")
	host.Type("/schedule-create")
	host.Press(input.Enter)
	host.Shows(t, "Create scheduled run")
	host.Press(input.Tab)
	host.Type("audit the repository")
	host.Press(input.Tab)
	host.Send(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
	const zoned = "CRON_TZ=Asia/Shanghai 0 9 * * 1-5"
	host.Type(zoned)
	host.Press(input.Enter)
	created := awaitValue(t, service.created, "schedule creation")
	if created.Cron != zoned {
		t.Fatalf("created schedule cron = %q, want %q", created.Cron, zoned)
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

	original := schedule.CreateV1Output{
		ID: "sch_review", Instructions: "review the repository",
		Cron: defaultScheduleCron, NextRunAt: time.Now(), Revision: 1,
	}
	original.ModelSelection.Provider, original.ModelSelection.Model = "deepseek", "deepseek-v4-flash"
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
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: backend, Schedules: service, Changes: source, SessionID: "ses_demo_1"})
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
	if _, err := backend.UpdateSession(t.Context(), conversation.UpdateSession{
		SessionID: snapshot.Session.ID, Workspace: &replacementWorkspace,
		ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: protocol.RuntimeSessionsChanged, Sequence: 1,
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
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: backend, Schedules: service, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/schedule-run sch_review")
	host.Press(input.Enter)
	if id := awaitValue(t, service.started, "schedule run mutation"); id != "sch_review" {
		t.Fatalf("schedule run id = %q, want sch_review", id)
	}

	if _, err := backend.RollbackSession(t.Context(), conversation.RollbackSession{
		SessionID: "ses_demo_1", Scope: protocol.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	title := "Schedule refresh installed"
	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.UpdateSession(t.Context(), conversation.UpdateSession{
		SessionID: snapshot.Session.ID, Title: &title, ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: protocol.RuntimeSessionsChanged, Sequence: 1,
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
		host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
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
		host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
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
		service.schedules[0].NextRunAt = time.Time{}
		host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
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
		host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
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
		host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service})
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
		applied: make(chan changefeed.Event, 1), supported: []protocol.RuntimeTopic{protocol.TopicPluginDataChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), Schedules: service, Changes: source})
	host.Shows(t, "Ask flame")
	subscription := awaitValue(t, source.subscription, "schedule invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != protocol.TopicPluginDataChanged {
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
		Type: protocol.RuntimePluginDataChanged, Sequence: 1,
	}
	awaitSignal(t, source.applied, "plugins.dataChanged delivery")
	host.Shows(t, "Updated repository review")
	if service.reads.Load() <= baseline {
		t.Fatal("plugins.dataChanged did not refetch the open schedule projection")
	}
	stop()
}

func TestScheduleDraftClearsWorkspaceBindingAndRejectsAmbiguousIdentity(t *testing.T) {
	t.Parallel()
	original := newScheduleServiceStub().schedules[0]
	draft := newScheduleFormDraft(scheduleFormUpdate, original, "")
	draft.workspace = ""
	request, changed, err := draft.patch(original)
	if err != nil || !changed || request.CWD == nil || *request.CWD != "" {
		t.Fatalf("workspace clearing request = (%+v, %v, %v)", request, changed, err)
	}
	duplicate := cloneSchedule(original)
	duplicate.ID = "sch_review_other"
	if _, err := resolveSchedule([]schedule.CreateV1Output{original, duplicate}, "sch_rev"); err == nil {
		t.Fatal("ambiguous schedule prefix was accepted")
	}
}

func (s *scheduleServiceStub) PrepareUpdate(_ context.Context, input schedule.UpdateV1Input) (protocol.CallPluginRequest, error) {
	return boundScheduleRequest(schedule.UpdateV1Method, input)
}
func (s *scheduleServiceStub) DecodeUpdate(id string, reply *protocol.CallPluginResponse) (schedule.UpdateV1Output, error) {
	value, err := s.DecodeCreate(reply)
	if err == nil && value.ID != id {
		err = errors.New("another schedule acknowledged the mutation")
	}
	return value, err
}
