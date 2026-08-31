package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/modelconfig"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/cli/internal/testsupport/runtimefixture"
)

type usageServiceStub struct{}

func (usageServiceStub) SessionUsage(_ context.Context, sessionID string) (agent.SessionUsageReport, error) {
	cost := 0.25
	return agent.SessionUsageReport{
		SessionID: sessionID, Total: agent.UsageTotals{InputTokens: 1_200, OutputTokens: 300, CostUSD: &cost},
		ByModel: []agent.UsageBucket{{Key: "deepseek/model", Totals: agent.UsageTotals{InputTokens: 1_200}}},
	}, nil
}

func (usageServiceStub) Summary(_ context.Context, period agent.UsageSummaryPeriod) (agent.UsageSummary, error) {
	cost := 1.5
	return agent.UsageSummary{
		Period: period, Total: agent.UsageTotals{InputTokens: 8_000, OutputTokens: 2_000, CostUSD: &cost},
		ByProvider: []agent.UsageBucket{{Key: "deepseek", Runs: 4}}, Sessions: 2, Runs: 4,
	}, nil
}

type blockingUsageService struct {
	started  chan struct{}
	canceled chan struct{}
}

func (b blockingUsageService) SessionUsage(ctx context.Context, _ string) (agent.SessionUsageReport, error) {
	close(b.started)
	<-ctx.Done()
	close(b.canceled)
	return agent.SessionUsageReport{}, context.Cause(ctx)
}

func (blockingUsageService) Summary(context.Context, agent.UsageSummaryPeriod) (agent.UsageSummary, error) {
	panic("summary must not run after the session usage query is canceled")
}

func TestUsageAndModelRoleCommandsProjectRuntimeConfiguration(t *testing.T) {
	models := newModelConfigServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Usage: usageServiceStub{}, ModelConfig: models})
	host.Shows(t, "Ask flame")
	host.Type("/usage 30")
	host.Press(input.Enter)
	host.Shows(t, "Runtime usage")
	host.Shows(t, "last 30 days · 2 sessions · 4 runs")
	host.Shows(t, "input 1,200")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")

	host.Type("/roles")
	host.Press(input.Enter)
	host.Shows(t, "Auxiliary model roles")
	host.Shows(t, "inherit the run model")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/utility deepseek/maintenance")
	host.Press(input.Enter)
	host.Shows(t, "utility model · deepseek/maintenance")
	host.Type("/embedding off")
	host.Press(input.Enter)
	host.Shows(t, "embedding model · disabled")
	roles, err := models.Roles(t.Context())
	provider, model, configured := roles.Utility.ProviderModel()
	if err != nil || !configured || provider != "deepseek" || model != "maintenance" || roles.Embedding.Configured() {
		t.Fatalf("roles = (%+v, %v)", roles, err)
	}
	stop()
}

func TestParseModelRoleUsesRoleSpecificUnconfiguredIntent(t *testing.T) {
	t.Parallel()
	utility, err := parseModelRole(modelconfig.UtilityRole, utilityRoleInheritedArgument)
	if err != nil || utility.Configured() {
		t.Fatalf("utility inherit = (%+v, %v)", utility, err)
	}
	embedding, err := parseModelRole(modelconfig.EmbeddingRole, embeddingRoleDisabledArgument)
	if err != nil || embedding.Configured() {
		t.Fatalf("embedding off = (%+v, %v)", embedding, err)
	}
	if _, err := parseModelRole(modelconfig.UtilityRole, embeddingRoleDisabledArgument); err == nil {
		t.Fatal("utility accepted embedding's disabled command")
	}
	if _, err := parseModelRole(modelconfig.EmbeddingRole, utilityRoleInheritedArgument); err == nil {
		t.Fatal("embedding accepted utility's inherited command")
	}
	configured, err := parseModelRole(modelconfig.UtilityRole, "deepseek/maintenance")
	provider, model, present := configured.ProviderModel()
	if err != nil || !present || provider != "deepseek" || model != "maintenance" {
		t.Fatalf("configured role = (%+v, %v)", configured, err)
	}
	for _, argument := range []string{" deepseek/maintenance", "deepseek /maintenance", "deepseek/ maintenance"} {
		if _, err := parseModelRole(modelconfig.UtilityRole, argument); err == nil {
			t.Fatalf("configured role normalized identity input %q", argument)
		}
	}
}

func TestRuntimeStatusConsumesTheNegotiatedDiscoveryProfile(t *testing.T) {
	profile := runtimebinding.Profile{
		Protocol:  runtimebinding.Protocol{Version: "2.0"},
		Server:    runtimebinding.Server{Name: "flame-runtime", Version: "1.2.3", DefaultWorkspace: "/workspace", Home: "/home/test"},
		RunEvents: []string{"segment.started"}, RuntimeTopics: []string{"files.changed"},
		StreamingMethods: []string{"runs.start"},
		Features: map[runtimebinding.FeatureName]runtimebinding.Feature{
			runtimebinding.FeatureMCP: {Enabled: true},
		},
		Limits: runtimebinding.Limits{
			RunConcurrency:                   boundedRunConcurrency(t, 4),
			CommandReplay:                    testCommandReplay(t, "idp_test", 10*time.Minute),
			RunReplay:                        runtimebinding.ReplayLimits{Scope: "runtimeInstanceRootSegment", MaxEvents: 1024, MaxBytes: 1 << 20},
			MCPAuthorizationRetentionSeconds: 600,
			RuntimeSubscription:              runtimebinding.SubscriptionLimits{MaxTopics: 16, MaxWatches: 32},
		},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), RuntimeProfile: &profile})
	host.Shows(t, "Ask flame")
	host.Type("/status")
	host.Press(input.Enter)
	for _, want := range []string{
		"flame-runtime 1.2.3", "protocol: 2.0", "default workspace: /workspace", "available features: mcp",
		"run concurrency: at most 4 runs", "run replay: 1024 events / 1 MiB", "command replay retention: 10m",
		"runtime subscriptions: 16 topics / 32 watches", "1 run events / 1 topics / 1 streaming methods",
	} {
		host.Shows(t, want)
	}
	stop()
}

func boundedRunConcurrency(t *testing.T, maximum int) runtimebinding.RunConcurrencyLimit {
	t.Helper()
	limit, err := runtimebinding.NewBoundedRunConcurrencyLimit(maximum)
	if err != nil {
		t.Fatal(err)
	}
	return limit
}

func testCommandReplay(t *testing.T, namespace string, retention time.Duration) commandreplay.Capability {
	t.Helper()
	capability, err := commandreplay.NewCapability(namespace, retention)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func protectedCommandReplayGuard(t *testing.T, namespace string, until time.Time) commandreplay.Guard {
	t.Helper()
	guard, err := commandreplay.NewProtectedGuard(namespace, until)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func TestSessionReplacementCancelsAnOutstandingSideQuery(t *testing.T) {
	usageService := blockingUsageService{started: make(chan struct{}), canceled: make(chan struct{})}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Usage: usageService})
	host.Shows(t, "Ask flame")
	host.Type("/usage")
	host.Press(input.Enter)
	awaitSignal(t, usageService.started, "session usage query")

	host.Type("/new")
	host.Press(input.Enter)
	awaitSignal(t, usageService.canceled, "old session query cancellation")
	host.Hides(t, "Runtime usage")
	stop()
}

type modelConfigServiceStub struct {
	mu        sync.Mutex
	roles     modelconfig.Roles
	providers []modelconfig.Provider
	updates   chan modelconfig.UpdateProvider
}

type blockingProviderUpdateService struct {
	*modelConfigServiceStub
	started  chan modelconfig.UpdateProvider
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingProviderUpdateService) UpdateProvider(
	ctx context.Context,
	update modelconfig.UpdateProvider,
) (modelconfig.Provider, error) {
	select {
	case b.started <- update:
	default:
	}
	select {
	case <-b.release:
		return b.modelConfigServiceStub.UpdateProvider(ctx, update)
	case <-ctx.Done():
		select {
		case b.canceled <- struct{}{}:
		default:
		}
		return modelconfig.Provider{}, context.Cause(ctx)
	}
}

func newModelConfigServiceStub() *modelConfigServiceStub {
	return &modelConfigServiceStub{
		roles: modelconfig.Roles{
			Utility:   modelconfig.InheritedUtilityRole(),
			Embedding: modelconfig.DisabledEmbeddingRole(),
		},
		providers: []modelconfig.Provider{terminalTestProvider(
			"deepseek", "https://api.deepseek.example", "sk****42", modelconfig.KeyStored,
		)},
		updates: make(chan modelconfig.UpdateProvider, 1),
	}
}

func terminalTestProvider(id, rawBaseURL, masked string, source modelconfig.KeySource) modelconfig.Provider {
	credential, err := modelconfig.NewCredential(masked, source)
	if err != nil {
		panic(err)
	}
	provider, err := modelconfig.NewProvider(modelconfig.ProviderSpec{
		ID: id, BaseURL: &rawBaseURL, Credential: &credential, Configured: true,
		CredentialRequirement: modelconfig.APIKeyRequired,
	})
	if err != nil {
		panic(err)
	}
	return provider
}

func (m *modelConfigServiceStub) Roles(context.Context) (modelconfig.Roles, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.roles, nil
}

func (m *modelConfigServiceStub) SetRole(_ context.Context, role modelconfig.Role) (modelconfig.Role, error) {
	if err := role.Validate(); err != nil {
		return modelconfig.Role{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if role.Kind() == modelconfig.UtilityRole {
		m.roles.Utility = role
	} else {
		m.roles.Embedding = role
	}
	return role, nil
}

func (m *modelConfigServiceStub) Providers(context.Context) ([]modelconfig.Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]modelconfig.Provider(nil), m.providers...), nil
}

func (m *modelConfigServiceStub) UpdateProvider(_ context.Context, update modelconfig.UpdateProvider) (modelconfig.Provider, error) {
	if err := update.Validate(); err != nil {
		return modelconfig.Provider{}, err
	}
	cloned := update
	if update.BaseURL != nil {
		value := *update.BaseURL
		cloned.BaseURL = &value
	}
	if update.APIKey != nil {
		value := *update.APIKey
		cloned.APIKey = &value
	}
	m.updates <- cloned
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.providers[0], nil
}

func (*modelConfigServiceStub) TestProvider(_ context.Context, providerID string) (modelconfig.TestResult, error) {
	if providerID == "deepseek" {
		return modelconfig.TestResult{OK: true}, nil
	}
	return modelconfig.TestResult{}, errors.New("unknown provider")
}

func TestProviderConfigurationMasksSecretsAndPreservesExplicitChanges(t *testing.T) {
	models := newModelConfigServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), ModelConfig: models})
	host.Shows(t, "Ask flame")
	host.Type("/providers")
	host.Press(input.Enter)
	host.Shows(t, "sk****42")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/provider-test deepseek")
	host.Press(input.Enter)
	host.Shows(t, "provider deepseek is reachable")

	host.Type("/provider-config deepseek")
	host.Press(input.Enter)
	host.Shows(t, "Configure provider · deepseek")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	secret := "SECRET_PROVIDER_KEY_42"
	host.Type(secret)
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("provider configuration did not survive a minimal viewport")
	}
	host.Shows(t, "Configure provider · deepseek")
	if strings.Contains(host.Frames(), secret) {
		t.Fatal("masked provider key appeared in terminal output")
	}
	host.Press(input.Enter)
	host.Shows(t, "provider updated · deepseek")
	update := <-models.updates
	if update.BaseURL != nil || update.APIKey == nil || update.APIKey.Kind != modelconfig.SetValue || update.APIKey.Value != secret {
		t.Fatalf("provider update = %+v", update)
	}
	stop()
}

func TestEnvironmentProviderCanBeOverriddenByStoredKey(t *testing.T) {
	models := newModelConfigServiceStub()
	models.providers[0] = terminalTestProvider(
		"deepseek", "https://api.deepseek.example", "sk****env", modelconfig.KeyEnvironment,
	)
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: runtimefixture.New(), ModelConfig: models,
	})
	host.Shows(t, "Ask flame")
	host.Type("/provider-config deepseek")
	host.Press(input.Enter)
	host.Shows(t, "Configure provider · deepseek")
	host.Shows(t, "Set a stored key")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	host.Type("STORED_PROVIDER_OVERRIDE")
	host.Press(input.Enter)
	host.Shows(t, "provider updated · deepseek")
	update := awaitValue(t, models.updates, "environment provider override")
	if update.APIKey == nil || update.APIKey.Kind != modelconfig.SetValue || update.APIKey.Value != "STORED_PROVIDER_OVERRIDE" {
		t.Fatalf("provider update = %+v", update)
	}
	stop()
}

func TestProviderMutationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	baseService := newModelConfigServiceStub()
	service := &blockingProviderUpdateService{
		modelConfigServiceStub: baseService,
		started:                make(chan modelconfig.UpdateProvider, 1),
		release:                make(chan struct{}),
		canceled:               make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)

	backend := runtimefixture.New()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, ModelConfig: service, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/provider-config deepseek")
	host.Press(input.Enter)
	host.Shows(t, "Configure provider · deepseek")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Press(input.Tab)
	host.Type("ROTATED_PROVIDER_KEY")
	host.Press(input.Enter)
	update := awaitValue(t, service.started, "provider update mutation")
	if update.Provider != "deepseek" || update.APIKey == nil || update.APIKey.Kind != modelconfig.SetValue {
		t.Fatalf("provider update = %+v", update)
	}

	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	title := "Provider refresh installed"
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
		t.Fatal("same-session projection replacement canceled the application-owned provider mutation")
	default:
	}

	release()
	host.Shows(t, "provider updated · deepseek")
	completed := awaitValue(t, service.updates, "completed provider update mutation")
	if completed.APIKey == nil || completed.APIKey.Value != "ROTATED_PROVIDER_KEY" {
		t.Fatalf("completed provider update = %+v", completed)
	}
	stop()
}

type goalServiceStub struct {
	mu         sync.Mutex
	current    *agent.Goal
	reads      atomic.Int32
	writes     atomic.Int32
	readErr    chan error
	readSignal chan struct{}
}

func (g *goalServiceStub) GetGoal(context.Context, string) (agent.Goal, bool, error) {
	g.reads.Add(1)
	select {
	case g.readSignal <- struct{}{}:
	default:
	}
	select {
	case err := <-g.readErr:
		return agent.Goal{}, false, err
	default:
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current == nil {
		return agent.Goal{}, false, nil
	}
	return *g.current, true, nil
}

func (g *goalServiceStub) StartGoal(_ context.Context, start agent.StartGoal) (agent.Goal, error) {
	g.writes.Add(1)
	if err := start.Validate(); err != nil {
		return agent.Goal{}, err
	}
	at := time.Unix(1, 0).UTC()
	current, err := agent.RestoreGoal(agent.GoalSnapshot{
		SessionID: start.SessionID, Objective: start.Objective, Status: agent.GoalActive,
		Provider: start.Provider, Model: start.Model, Budget: start.Budget,
		CreatedAt: at, UpdatedAt: at,
	})
	if err != nil {
		return agent.Goal{}, err
	}
	g.set(current)
	return current, nil
}

func (g *goalServiceStub) UpdateGoal(_ context.Context, update agent.UpdateGoal) (agent.Goal, error) {
	g.writes.Add(1)
	if err := update.Validate(); err != nil {
		return agent.Goal{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current == nil {
		return agent.Goal{}, errors.New("no goal")
	}
	snapshot := g.current.Snapshot()
	snapshot.Objective = update.Objective
	snapshot.UpdatedAt = snapshot.UpdatedAt.Add(time.Nanosecond)
	current, err := agent.RestoreGoal(snapshot)
	if err != nil {
		return agent.Goal{}, err
	}
	g.current = &current
	return current, nil
}

func (g *goalServiceStub) ClearGoal(context.Context, string) error {
	g.writes.Add(1)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.current = nil
	return nil
}

func (g *goalServiceStub) StopGoal(context.Context, string) (agent.Goal, error) {
	g.writes.Add(1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current == nil {
		return agent.Goal{}, errors.New("no goal")
	}
	snapshot := g.current.Snapshot()
	snapshot.Status = agent.GoalPaused
	snapshot.ReasonCode = agent.GoalStoppedByUser
	snapshot.UpdatedAt = snapshot.UpdatedAt.Add(time.Nanosecond)
	current, err := agent.RestoreGoal(snapshot)
	if err != nil {
		return agent.Goal{}, err
	}
	g.current = &current
	return current, nil
}

func (g *goalServiceStub) ResumeGoal(context.Context, string) (agent.Goal, error) {
	g.writes.Add(1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current == nil {
		return agent.Goal{}, errors.New("no goal")
	}
	snapshot := g.current.Snapshot()
	snapshot.Status = agent.GoalActive
	snapshot.ReasonCode = agent.GoalReasonNone
	snapshot.ReasonDetail = ""
	snapshot.UpdatedAt = snapshot.UpdatedAt.Add(time.Nanosecond)
	current, err := agent.RestoreGoal(snapshot)
	if err != nil {
		return agent.Goal{}, err
	}
	g.current = &current
	return current, nil
}

func (g *goalServiceStub) set(current agent.Goal) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.current = &current
}

func testGoal(t testing.TB, objective string) agent.Goal {
	t.Helper()
	at := time.Unix(1, 0).UTC()
	current, err := agent.RestoreGoal(agent.GoalSnapshot{
		SessionID: "ses_demo_1", Objective: objective, Status: agent.GoalActive,
		Budget: agent.UnlimitedGoalBudget(), CreatedAt: at, UpdatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func reviseGoal(t testing.TB, current agent.Goal, revise func(*agent.GoalSnapshot)) agent.Goal {
	t.Helper()
	snapshot := current.Snapshot()
	revise(&snapshot)
	snapshot.UpdatedAt = snapshot.UpdatedAt.Add(time.Nanosecond)
	revised, err := agent.RestoreGoal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return revised
}

func goalPointer(current agent.Goal) *agent.Goal { return &current }

func TestGoalLifecycleAndInvalidationRefreshTheOpenGoalReader(t *testing.T) {
	goals := new(goalServiceStub)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 2), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 2), supported: []changefeed.Topic{changefeed.GoalsChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Goals: goals, Changes: source})
	host.Shows(t, "Ask flame")
	subscription := awaitValue(t, source.subscription, "goal invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != changefeed.GoalsChanged {
		t.Fatalf("goal subscription = %+v", subscription)
	}
	host.Type("/goal")
	host.Press(input.Enter)
	host.Shows(t, "No autonomous goal")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/goal-start finish the release")
	host.Press(input.Enter)
	host.Shows(t, "finish the release")
	host.Shows(t, "active")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/goal-update ship the release")
	host.Press(input.Enter)
	host.Shows(t, "ship the release")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/goal-stop")
	host.Press(input.Enter)
	host.Shows(t, "stoppedByUser")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/goal-resume")
	host.Press(input.Enter)
	host.Shows(t, "active")

	current, exists, err := goals.GetGoal(t.Context(), "ses_demo_1")
	if err != nil || !exists {
		t.Fatalf("goal = (%+v, %t, %v)", current, exists, err)
	}
	goals.set(reviseGoal(t, current, func(snapshot *agent.GoalSnapshot) {
		snapshot.Status = agent.GoalBlocked
		snapshot.ReasonCode = agent.GoalBlockedByModel
		snapshot.ReasonDetail = "needs clarification"
	}))
	baseline := goals.reads.Load()
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.GoalsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "goals.changed delivery")
	host.Shows(t, "needs clarification")
	if goals.reads.Load() <= baseline {
		t.Fatal("goals.changed did not refetch the goal")
	}
	goals.set(reviseGoal(t, current, func(snapshot *agent.GoalSnapshot) {
		snapshot.Status = agent.GoalCompleting
		snapshot.ReasonCode = agent.GoalReasonNone
		snapshot.ReasonDetail = ""
	}))
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.GoalsChanged), Sequence: 2,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "completing goals.changed delivery")
	host.Shows(t, "completing")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	baselineWrites := goals.writes.Load()
	host.Type("/goal-stop")
	host.Press(input.Enter)
	host.Shows(t, "goal is completing final accounting")
	if writes := goals.writes.Load(); writes != baselineWrites {
		t.Fatalf("goal mutations after settlement command = %d, want %d", writes, baselineWrites)
	}
	settling, exists, err := goals.GetGoal(t.Context(), "ses_demo_1")
	if err != nil || !exists || settling.Status() != agent.GoalCompleting {
		t.Fatalf("goal after rejected settlement command = (%+v, %t, %v)", settling, exists, err)
	}
	host.Type("/goal-clear")
	host.Press(input.Enter)
	host.Shows(t, "No autonomous goal")
	stop()
}

func TestGoalInvalidationConvergesAfterATransientReadFailure(t *testing.T) {
	goals := &goalServiceStub{
		current: goalPointer(testGoal(t, "original objective")),
		readErr: make(chan error, 1),
	}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.GoalsChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Goals: goals, Changes: source})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "goal invalidation subscription")
	host.Type("/goal")
	host.Press(input.Enter)
	host.Shows(t, "original objective")

	goals.set(testGoal(t, "converged objective"))
	goals.readErr <- fmt.Errorf("temporary goal read failure: %w", agent.ErrDisconnected)
	baseline := goals.reads.Load()
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.GoalsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "goals.changed delivery")
	host.Shows(t, "converged objective")
	if reads := goals.reads.Load() - baseline; reads < 2 {
		t.Fatalf("goal refresh reads = %d, want a failed attempt followed by recovery", reads)
	}
	stop()
}

func TestGoalInvalidationDoesNotRetryAnIncompatibleProjection(t *testing.T) {
	goals := &goalServiceStub{
		current:    goalPointer(testGoal(t, "original objective")),
		readErr:    make(chan error, 1),
		readSignal: make(chan struct{}, 4),
	}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.GoalsChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Goals: goals, Changes: source})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "goal invalidation subscription")
	host.Type("/goal")
	host.Press(input.Enter)
	host.Shows(t, "original objective")
	drainSignals(goals.readSignal)

	goals.readErr <- agent.ErrIncompatibleRuntime
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.GoalsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "goals.changed delivery")
	awaitSignal(t, goals.readSignal, "incompatible goal read")
	select {
	case <-goals.readSignal:
		t.Fatal("incompatible goal projection was retried")
	case <-time.After(3 * runtimeRecoveryFirstDelay(t)):
	}
	stop()
}

func TestGoalInvalidationDoesNotRetryAPermanentProjectionFailure(t *testing.T) {
	goals := &goalServiceStub{
		current:    goalPointer(testGoal(t, "original objective")),
		readErr:    make(chan error, 1),
		readSignal: make(chan struct{}, 4),
	}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.GoalsChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Goals: goals, Changes: source})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "goal invalidation subscription")
	host.Type("/goal")
	host.Press(input.Enter)
	host.Shows(t, "original objective")
	drainSignals(goals.readSignal)

	permanent := errors.New("goal projection rejected")
	goals.readErr <- permanent
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.GoalsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "goals.changed delivery")
	awaitSignal(t, goals.readSignal, "permanent goal read")
	select {
	case <-goals.readSignal:
		t.Fatal("permanent goal projection failure was retried")
	case <-time.After(3 * runtimeRecoveryFirstDelay(t)):
	}
	host.Press(input.Esc)
	host.Shows(t, permanent.Error())
	stop()
}

type blockingWorkspaceListService struct {
	*workspaceServiceStub
	started  chan struct{}
	canceled chan struct{}
}

func (b *blockingWorkspaceListService) List(ctx context.Context) ([]workspace.Summary, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	select {
	case b.canceled <- struct{}{}:
	default:
	}
	return nil, context.Cause(ctx)
}

func TestLatestReaderQueryRetiresAnOlderBoundedContextProjection(t *testing.T) {
	workspaces := &blockingWorkspaceListService{
		workspaceServiceStub: newWorkspaceServiceStub(),
		started:              make(chan struct{}, 1),
		canceled:             make(chan struct{}, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Workspaces: workspaces, Goals: new(goalServiceStub)})
	host.Shows(t, "Ask flame")
	host.Type("/workspaces")
	host.Press(input.Enter)
	awaitSignal(t, workspaces.started, "workspace reader query")
	host.Type("/goal")
	host.Press(input.Enter)
	host.Shows(t, "No autonomous goal")
	awaitSignal(t, workspaces.canceled, "superseded workspace reader query")
	host.Hides(t, "Runtime workspaces")
	stop()
}

type blockingGoalMutationService struct {
	*goalServiceStub
	started  chan struct{}
	release  chan struct{}
	canceled chan struct{}
}

func (b *blockingGoalMutationService) StopGoal(ctx context.Context, sessionID string) (agent.Goal, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
		return b.goalServiceStub.StopGoal(ctx, sessionID)
	case <-ctx.Done():
		select {
		case b.canceled <- struct{}{}:
		default:
		}
		return agent.Goal{}, context.Cause(ctx)
	}
}

func TestReaderRefreshDoesNotCancelAGoalLifecycleCommand(t *testing.T) {
	base := new(goalServiceStub)
	base.set(testGoal(t, "finish safely"))
	service := &blockingGoalMutationService{
		goalServiceStub: base,
		started:         make(chan struct{}, 1), release: make(chan struct{}), canceled: make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Goals: service})
	host.Shows(t, "Ask flame")
	host.Type("/goal-stop")
	host.Press(input.Enter)
	awaitSignal(t, service.started, "goal stop command")
	host.Type("/goal")
	host.Press(input.Enter)
	host.Shows(t, "finish safely")
	select {
	case <-service.canceled:
		t.Fatal("reader refresh canceled the goal lifecycle command")
	default:
	}
	release()
	host.Shows(t, "stoppedByUser")
	if writes := base.writes.Load(); writes != 1 {
		t.Fatalf("goal lifecycle writes = %d, want 1", writes)
	}
	stop()
}

func TestGoalMutationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	backend := runtimefixture.New()
	base := new(goalServiceStub)
	base.set(testGoal(t, "finish safely"))
	service := &blockingGoalMutationService{
		goalServiceStub: base,
		started:         make(chan struct{}, 1), release: make(chan struct{}), canceled: make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: backend, Goals: service, Changes: source, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/goal-stop")
	host.Press(input.Enter)
	awaitSignal(t, service.started, "goal stop mutation")
	title := "Projection changed during goal mutation"
	installChangedSessionProjection(t, backend, source, "ses_demo_1", title)
	host.Shows(t, title)
	select {
	case <-service.canceled:
		t.Fatal("session projection replacement canceled the goal mutation")
	default:
	}
	release()
	host.Shows(t, "stoppedByUser")
	if writes := base.writes.Load(); writes != 1 {
		t.Fatalf("goal lifecycle writes = %d, want 1", writes)
	}
	stop()
}

func TestGoalMutationDoesNotInstallAReaderAfterSessionSwitch(t *testing.T) {
	base := new(goalServiceStub)
	base.set(testGoal(t, "finish safely"))
	service := &blockingGoalMutationService{
		goalServiceStub: base,
		started:         make(chan struct{}, 1), release: make(chan struct{}), canceled: make(chan struct{}, 1),
	}
	release := sync.OnceFunc(func() { close(service.release) })
	t.Cleanup(release)
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New(), Goals: service, SessionID: "ses_demo_1"})
	host.Shows(t, "Ask flame")
	host.Type("/goal-stop")
	host.Press(input.Enter)
	awaitSignal(t, service.started, "goal stop mutation")
	host.Type("/new")
	host.Press(input.Enter)
	host.Shows(t, "session · Untitled session")
	select {
	case <-service.canceled:
		t.Fatal("session switch canceled the admitted goal mutation")
	default:
	}
	release()
	host.Shows(t, "goal · paused")
	host.Hides(t, "Session goal")
	if writes := base.writes.Load(); writes != 1 {
		t.Fatalf("goal lifecycle writes = %d, want 1", writes)
	}
	stop()
}

func runUIWithRuntimeServices(t *testing.T, config Config) (*programtest.Host, func()) {
	t.Helper()
	if config.SessionID == "" && config.Workspace == "" {
		config.SessionID = "ses_demo_1"
	}
	host := programtest.New(t, programtest.Config{Width: 96, Height: 28})
	config.Host = host
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, config) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func awaitValue[T any](t *testing.T, values <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + what)
		return *new(T)
	}
}
