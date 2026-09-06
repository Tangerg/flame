package runtimebinding

import (
	"context"
	"strings"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/failure"
)

type usageBindingStub struct {
	session func(context.Context, protocol.SessionUsageRequest, flameruntime.CallOptions) (*protocol.Usage, error)
	summary func(context.Context, protocol.UsageSummaryRequest, flameruntime.CallOptions) (*protocol.UsageSummary, error)
}

func (u usageBindingStub) GetSessionUsage(ctx context.Context, request protocol.SessionUsageRequest, options flameruntime.CallOptions) (*protocol.Usage, error) {
	return u.session(ctx, request, options)
}

func (u usageBindingStub) GetUsageSummary(ctx context.Context, request protocol.UsageSummaryRequest, options flameruntime.CallOptions) (*protocol.UsageSummary, error) {
	return u.summary(ctx, request, options)
}

func TestUsageAdapterProjectsSessionAndSummaryReports(t *testing.T) {
	cost := 0.25
	stub := usageBindingStub{
		session: func(_ context.Context, request protocol.SessionUsageRequest, options flameruntime.CallOptions) (*protocol.Usage, error) {
			if request.SessionID != "ses_1" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
				t.Fatalf("session usage request = %+v, options = %+v", request, options)
			}
			return &protocol.Usage{
				ModelUsage: protocol.ModelUsage{InputTokens: 12, CostUSD: new(cost)},
				ByModel: map[string]protocol.ModelUsage{
					"z/model": {OutputTokens: 2}, "a/model": {InputTokens: 3},
				},
			}, nil
		},
		summary: func(_ context.Context, request protocol.UsageSummaryRequest, _ flameruntime.CallOptions) (*protocol.UsageSummary, error) {
			if request.SinceDays == nil || *request.SinceDays != 30 {
				t.Fatalf("summary request = %+v", request)
			}
			return &protocol.UsageSummary{
				Total: protocol.ModelUsage{InputTokens: 20}, Sessions: 2, Runs: 4,
				ByProvider: []protocol.UsageBucket{{Key: "deepseek", Runs: 4}},
			}, nil
		},
	}
	runtime := &Connection{usage: stub, meta: requestMeta("test")}
	session, err := runtime.SessionUsage(t.Context(), "ses_1")
	if err != nil || len(session.ByModel) != 2 || session.ByModel[0].Key != "a/model" || session.Total.CostUSD == nil {
		t.Fatalf("SessionUsage = (%+v, %v)", session, err)
	}
	cost = 9
	if *session.Total.CostUSD != 0.25 {
		t.Fatal("session usage projection aliases runtime cost storage")
	}
	summary, err := runtime.Summary(t.Context(), recentUsagePeriod(t, 30))
	if err != nil || summary.Runs != 4 || len(summary.ByProvider) != 1 {
		t.Fatalf("Summary = (%+v, %v)", summary, err)
	}
}

func TestUsageAdapterKeepsAllTimeAbsentOnTheWire(t *testing.T) {
	t.Parallel()
	runtime := &Connection{usage: usageBindingStub{
		summary: func(_ context.Context, request protocol.UsageSummaryRequest, _ flameruntime.CallOptions) (*protocol.UsageSummary, error) {
			if request.SinceDays != nil {
				t.Fatalf("all-time summary sent sinceDays = %d", *request.SinceDays)
			}
			return &protocol.UsageSummary{}, nil
		},
	}, meta: requestMeta("test")}

	report, err := runtime.Summary(t.Context(), agent.AllTimeUsage())
	if err != nil {
		t.Fatal(err)
	}
	if days, recent, err := report.Period.Days(); err != nil || recent || days != 0 {
		t.Fatalf("summary period = (%d, %t, %v), want all-time", days, recent, err)
	}
}

func TestUsageAdapterRejectsUnknownPeriodBeforeCallingRuntime(t *testing.T) {
	t.Parallel()
	called := false
	runtime := &Connection{usage: usageBindingStub{
		summary: func(context.Context, protocol.UsageSummaryRequest, flameruntime.CallOptions) (*protocol.UsageSummary, error) {
			called = true
			return &protocol.UsageSummary{}, nil
		},
	}, meta: requestMeta("test")}

	if _, err := runtime.Summary(t.Context(), agent.UsageSummaryPeriod{}); err == nil {
		t.Fatal("Summary accepted an unknown period")
	}
	if called {
		t.Fatal("Summary called the runtime binding before rejecting an unknown period")
	}
}

func TestUsageAdapterRejectsInvalidRuntimeReports(t *testing.T) {
	t.Parallel()
	runtime := &Connection{usage: usageBindingStub{
		session: func(context.Context, protocol.SessionUsageRequest, flameruntime.CallOptions) (*protocol.Usage, error) {
			return &protocol.Usage{ByModel: map[string]protocol.ModelUsage{" invalid/model": {}}}, nil
		},
		summary: func(context.Context, protocol.UsageSummaryRequest, flameruntime.CallOptions) (*protocol.UsageSummary, error) {
			return &protocol.UsageSummary{ByModel: []protocol.UsageBucket{{Key: "same"}, {Key: "same"}}}, nil
		},
	}, meta: requestMeta("test")}
	_, err := runtime.SessionUsage(t.Context(), "ses_1")
	requireRuntimeContractViolation(t, err)
	_, err = runtime.Summary(t.Context(), recentUsagePeriod(t, 7))
	requireRuntimeContractViolation(t, err)
}

func TestSessionUsageRejectsInvalidIdentityBeforeCallingRuntime(t *testing.T) {
	t.Parallel()
	called := false
	runtime := &Connection{usage: usageBindingStub{
		session: func(context.Context, protocol.SessionUsageRequest, flameruntime.CallOptions) (*protocol.Usage, error) {
			called = true
			return &protocol.Usage{}, nil
		},
	}, meta: requestMeta("test")}

	if _, err := runtime.SessionUsage(t.Context(), " ses_1"); err == nil || !strings.Contains(err.Error(), "sessionId") {
		t.Fatalf("SessionUsage error = %v, want sessionId field", err)
	}
	if called {
		t.Fatal("SessionUsage called Runtime with an invalid session identity")
	}
}

func recentUsagePeriod(t *testing.T, days int) agent.UsageSummaryPeriod {
	t.Helper()
	period, err := agent.RecentUsageDays(days)
	if err != nil {
		t.Fatalf("agent.RecentUsageDays(%d): %v", days, err)
	}
	return period
}

type modelConfigBindingStub struct {
	utility       protocol.UtilityRole
	embedding     protocol.EmbeddingRole
	providers     []protocol.Provider
	utilityReply  *protocol.UtilityRole
	providerReply *protocol.Provider
	utilitySet    func(protocol.UtilityRole, flameruntime.CommandOptions)
	embeddingSet  func(protocol.EmbeddingRole, flameruntime.CommandOptions)
	updated       func(protocol.UpdateProviderRequest, flameruntime.CommandOptions)
}

func (m *modelConfigBindingStub) GetUtilityRole(context.Context, flameruntime.CallOptions) (*protocol.UtilityRole, error) {
	return &m.utility, nil
}

func (m *modelConfigBindingStub) SetUtilityRole(_ context.Context, request protocol.UtilityRole, options flameruntime.CommandOptions) (*protocol.UtilityRole, error) {
	m.utilitySet(request, options)
	if m.utilityReply != nil {
		return m.utilityReply, nil
	}
	m.utility = request
	return &m.utility, nil
}

func (m *modelConfigBindingStub) GetEmbeddingRole(context.Context, flameruntime.CallOptions) (*protocol.EmbeddingRole, error) {
	return &m.embedding, nil
}

func (m *modelConfigBindingStub) SetEmbeddingRole(_ context.Context, request protocol.EmbeddingRole, options flameruntime.CommandOptions) (*protocol.EmbeddingRole, error) {
	m.embeddingSet(request, options)
	m.embedding = request
	return &m.embedding, nil
}

func (m *modelConfigBindingStub) ListProviders(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.Provider], error) {
	return protocol.NewPage(m.providers), nil
}

func (m *modelConfigBindingStub) UpdateProvider(_ context.Context, request protocol.UpdateProviderRequest, options flameruntime.CommandOptions) (*protocol.Provider, error) {
	m.updated(request, options)
	if m.providerReply != nil {
		return m.providerReply, nil
	}
	return &m.providers[0], nil
}

func TestModelConfigurationRejectsMutationIdentityDrift(t *testing.T) {
	t.Parallel()
	stub := &modelConfigBindingStub{
		utilityReply:  &protocol.UtilityRole{Provider: "other", Model: "model"},
		providerReply: &protocol.Provider{ID: "other"},
		utilitySet:    func(protocol.UtilityRole, flameruntime.CommandOptions) {},
		updated:       func(protocol.UpdateProviderRequest, flameruntime.CommandOptions) {},
	}
	runtime := &Connection{modelConfig: stub, meta: requestMeta("test")}
	role, err := models.NewConfiguredRole(models.UtilityRole, "deepseek", "chat")
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.SetRole(t.Context(), role)
	requireRuntimeContractViolation(t, err)
	change := models.ValueChange{Kind: protocol.ProviderConfigClear}
	_, err = runtime.UpdateProvider(t.Context(), models.UpdateProvider{Provider: "deepseek", APIKey: &change})
	requireRuntimeContractViolation(t, err)
}

func TestModelConfigurationRejectsPartialRoleProjection(t *testing.T) {
	t.Parallel()
	runtime := &Connection{modelConfig: &modelConfigBindingStub{
		utility:   protocol.UtilityRole{Provider: "deepseek"},
		embedding: protocol.EmbeddingRole{},
	}, meta: requestMeta("test")}

	_, err := runtime.Roles(t.Context())
	requireRuntimeContractViolation(t, err)
}

func TestModelConfigurationRejectsOutOfOrderProviderCatalog(t *testing.T) {
	t.Parallel()
	runtime := &Connection{modelConfig: &modelConfigBindingStub{providers: []protocol.Provider{
		{ID: "zeta", Configured: true, CredentialRequirement: protocol.ProviderAPIKeyOptional},
		{ID: "alpha", Configured: true, CredentialRequirement: protocol.ProviderAPIKeyOptional},
	}}, meta: requestMeta("test")}

	_, err := runtime.Providers(t.Context())
	requireRuntimeContractViolation(t, err)
}

func TestProviderUpdateRejectsAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	setBaseURL := models.ValueChange{Kind: protocol.ProviderConfigSet, Value: "https://new.example"}
	setAPIKey := models.ValueChange{Kind: protocol.ProviderConfigSet, Value: "stored-secret"}
	update := models.UpdateProvider{Provider: "deepseek", BaseURL: &setBaseURL, APIKey: &setAPIKey}
	valid := func() protocol.Provider {
		baseURL := setBaseURL.Value
		return protocol.Provider{
			ID: "deepseek", BaseURL: &baseURL,
			Credential: &protocol.ProviderCredential{Masked: "st****et", Source: protocol.ProviderKeySourceStored},
			Configured: true, CredentialRequirement: protocol.ProviderAPIKeyRequired,
		}
	}
	tests := []struct {
		name   string
		mutate func(*protocol.Provider)
	}{
		{name: "base URL", mutate: func(result *protocol.Provider) {
			baseURL := "https://old.example"
			result.BaseURL = &baseURL
		}},
		{name: "missing key", mutate: func(result *protocol.Provider) { result.Credential = nil }},
		{name: "environment key", mutate: func(result *protocol.Provider) {
			result.Credential = &protocol.ProviderCredential{Masked: "st****et", Source: protocol.ProviderKeySourceEnv}
		}},
		{name: "raw key", mutate: func(result *protocol.Provider) {
			result.Credential = &protocol.ProviderCredential{Masked: setAPIKey.Value, Source: protocol.ProviderKeySourceStored}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := valid()
			test.mutate(&result)
			stub := &modelConfigBindingStub{
				providerReply: &result,
				updated:       func(protocol.UpdateProviderRequest, flameruntime.CommandOptions) {},
			}
			runtime := &Connection{modelConfig: stub, meta: requestMeta("test")}
			_, err := runtime.UpdateProvider(t.Context(), update)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestProviderUpdateAcceptsClearWithEnvironmentFallback(t *testing.T) {
	t.Parallel()
	clear := models.ValueChange{Kind: protocol.ProviderConfigClear}
	result := protocol.Provider{
		ID: "deepseek", Credential: &protocol.ProviderCredential{Masked: "en****ey", Source: protocol.ProviderKeySourceEnv},
		Configured: true, CredentialRequirement: protocol.ProviderAPIKeyRequired,
	}
	stub := &modelConfigBindingStub{
		providerReply: &result,
		updated:       func(protocol.UpdateProviderRequest, flameruntime.CommandOptions) {},
	}
	runtime := &Connection{modelConfig: stub, meta: requestMeta("test")}
	if _, err := runtime.UpdateProvider(t.Context(), models.UpdateProvider{
		Provider: "deepseek", BaseURL: &clear, APIKey: &clear,
	}); err != nil {
		t.Fatalf("UpdateProvider clear with environment fallback: %v", err)
	}

	stillConfigured := "https://still-configured.example"
	result.BaseURL = &stillConfigured
	if _, err := runtime.UpdateProvider(t.Context(), models.UpdateProvider{
		Provider: "deepseek", BaseURL: &clear,
	}); err == nil {
		t.Fatal("UpdateProvider accepted a base URL after clear")
	} else {
		requireRuntimeContractViolation(t, err)
	}

	result.BaseURL = nil
	result.Credential = &protocol.ProviderCredential{Masked: "st****ed", Source: protocol.ProviderKeySourceStored}
	if _, err := runtime.UpdateProvider(t.Context(), models.UpdateProvider{
		Provider: "deepseek", APIKey: &clear,
	}); err == nil {
		t.Fatal("UpdateProvider accepted a stored key after clear")
	} else {
		requireRuntimeContractViolation(t, err)
	}
}

func TestProjectProviderPreservesConfiguredOptionalCredentialState(t *testing.T) {
	provider, err := projectProvider(protocol.Provider{
		ID: "ollama", Configured: true, CredentialRequirement: protocol.ProviderAPIKeyOptional,
		EmbeddingCapable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Configured() || provider.RequiresAPIKey() {
		t.Fatal("optional credential provider lost its configured state")
	}
	if _, present := provider.Credential(); present {
		t.Fatal("optional credential provider invented a credential")
	}
}

func TestProjectProviderValidatesDiscardedRuntimeFields(t *testing.T) {
	t.Parallel()
	invalidDefault := " default-model"
	_, err := projectProvider(protocol.Provider{
		ID: "provider", CredentialRequirement: protocol.ProviderAPIKeyOptional,
		DefaultEmbeddingModel: &invalidDefault,
	})
	if err == nil || !strings.Contains(err.Error(), "defaultEmbeddingModel") {
		t.Fatalf("projectProvider error = %v, want defaultEmbeddingModel field", err)
	}
}

func (*modelConfigBindingStub) TestProvider(_ context.Context, request protocol.TestProviderRequest, _ flameruntime.CallOptions) (*protocol.ProviderTestResult, error) {
	return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
		Type: protocol.ProblemProviderUnavailable, Detail: request.Provider,
		DocURL: "https://docs.example/providers", RetryAfterSeconds: 3,
	}}, nil
}

func TestModelConfigurationAdapterPreservesRoleAndSecretMutationSemantics(t *testing.T) {
	stub := &modelConfigBindingStub{
		utility: protocol.UtilityRole{Provider: "deepseek", Model: "chat"},
		providers: []protocol.Provider{{
			ID: "deepseek", Credential: &protocol.ProviderCredential{Masked: "sk****42", Source: protocol.ProviderKeySourceStored},
			Configured: true, CredentialRequirement: protocol.ProviderAPIKeyRequired,
			EmbeddingCapable: true,
		}},
	}
	assertCommand := func(options flameruntime.CommandOptions) {
		if options.IdempotencyKey == "" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
			t.Fatalf("command options = %+v", options)
		}
	}
	stub.utilitySet = func(request protocol.UtilityRole, options flameruntime.CommandOptions) {
		assertCommand(options)
		if request.Provider != "openai" || request.Model != "utility" {
			t.Fatalf("utility role request = %+v", request)
		}
	}
	stub.embeddingSet = func(request protocol.EmbeddingRole, options flameruntime.CommandOptions) {
		assertCommand(options)
		if request != (protocol.EmbeddingRole{}) {
			t.Fatalf("embedding role request = %+v", request)
		}
	}
	stub.updated = func(request protocol.UpdateProviderRequest, options flameruntime.CommandOptions) {
		assertCommand(options)
		if request.APIKey == nil || request.APIKey.Type != protocol.ProviderConfigSet || request.APIKey.Value == nil || *request.APIKey.Value != "secret" {
			t.Fatalf("provider update request = %+v", request)
		}
		if request.BaseURL == nil || request.BaseURL.Type != protocol.ProviderConfigClear || request.BaseURL.Value != nil {
			t.Fatalf("provider endpoint update = %+v", request.BaseURL)
		}
	}
	runtime := &Connection{modelConfig: stub, meta: requestMeta("test")}
	roles, err := runtime.Roles(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	utilityLabel, labelErr := roles.Utility.Label()
	if labelErr != nil || utilityLabel != "deepseek/chat" || roles.Embedding.Configured() {
		t.Fatalf("Roles = (%+v, %v)", roles, err)
	}
	utilityRole, roleErr := models.NewConfiguredRole(models.UtilityRole, "openai", "utility")
	if roleErr != nil {
		t.Fatal(roleErr)
	}
	if _, setRoleErr := runtime.SetRole(t.Context(), utilityRole); setRoleErr != nil {
		t.Fatal(setRoleErr)
	}
	if _, setRoleErr := runtime.SetRole(t.Context(), models.DisabledEmbeddingRole()); setRoleErr != nil {
		t.Fatal(setRoleErr)
	}
	providers, err := runtime.Providers(t.Context())
	if err != nil || len(providers) != 1 || !providers[0].Configured() {
		t.Fatalf("Providers = (%+v, %v)", providers, err)
	}
	secret := models.ValueChange{Kind: protocol.ProviderConfigSet, Value: "secret"}
	clear := models.ValueChange{Kind: protocol.ProviderConfigClear}
	if _, updateProviderErr := runtime.UpdateProvider(t.Context(), models.UpdateProvider{Provider: "deepseek", BaseURL: &clear, APIKey: &secret}); updateProviderErr != nil {
		t.Fatal(updateProviderErr)
	}
	tested, err := runtime.TestProvider(t.Context(), "deepseek")
	if err != nil || tested.OK || tested.Problem == nil || failure.String(tested.Problem) != "provider_unavailable: deepseek · retry after 3s · docs https://docs.example/providers" {
		t.Fatalf("TestProvider = (%+v, %v)", tested, err)
	}
	if _, err := runtime.TestProvider(t.Context(), " deepseek"); err == nil {
		t.Fatal("TestProvider normalized a provider identity")
	}
}

type goalBindingStub struct {
	t            *testing.T
	current      *protocol.Goal
	startResult  *protocol.Goal
	updateResult *protocol.Goal
	stopResult   *protocol.Goal
	resumeResult *protocol.Goal
	last         string
	getCalls     int
}

func (g *goalBindingStub) UpdateGoal(_ context.Context, request protocol.UpdateGoalRequest, options flameruntime.CommandOptions) (*protocol.Goal, error) {
	if request.SessionID != "ses_1" || request.Objective == "" || options.IdempotencyKey == "" {
		g.t.Fatalf("update goal request = %+v, options = %+v", request, options)
	}
	g.last = "update"
	if g.updateResult != nil {
		return ownedGoalResult(g.updateResult), nil
	}
	updated := *g.current
	updated.Objective = request.Objective
	g.current = &updated
	return ownedGoalResult(g.current), nil
}

func (g *goalBindingStub) ClearGoal(_ context.Context, request protocol.GoalRequest, options flameruntime.CommandOptions) error {
	if request.SessionID == "" || options.IdempotencyKey == "" {
		g.t.Fatalf("clear goal request = %+v, options = %+v", request, options)
	}
	g.last = "clear"
	g.current = nil
	return nil
}

func (g *goalBindingStub) GetGoal(context.Context, protocol.GoalRequest, flameruntime.CallOptions) (*protocol.Goal, error) {
	g.getCalls++
	return ownedGoalResult(g.current), nil
}

func (g *goalBindingStub) StartGoal(_ context.Context, request protocol.StartGoalRequest, options flameruntime.CommandOptions) (*protocol.Goal, error) {
	if request.SessionID != "ses_1" || request.Objective != "finish" || request.Budget == nil || request.Budget.MaxRuns == nil || *request.Budget.MaxRuns != 3 || options.IdempotencyKey == "" {
		g.t.Fatalf("start goal request = %+v, options = %+v", request, options)
	}
	g.last = "start"
	if g.startResult != nil {
		return ownedGoalResult(g.startResult), nil
	}
	g.current = activeProtocolGoal()
	g.current.Provider, g.current.Model = request.Provider, request.Model
	g.current.ReasoningEffort = request.ReasoningEffort
	return ownedGoalResult(g.current), nil
}

func (g *goalBindingStub) StopGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error) {
	g.last = "stop"
	if g.stopResult != nil {
		return ownedGoalResult(g.stopResult), nil
	}
	stopped := *g.current
	stopped.Status = protocol.GoalPaused
	stopped.Reason = &protocol.GoalReason{Code: protocol.GoalReasonStoppedByUser}
	g.current = &stopped
	return ownedGoalResult(g.current), nil
}

func (g *goalBindingStub) ResumeGoal(context.Context, protocol.GoalRequest, flameruntime.CommandOptions) (*protocol.Goal, error) {
	g.last = "resume"
	if g.resumeResult != nil {
		return ownedGoalResult(g.resumeResult), nil
	}
	resumed := *g.current
	resumed.Status = protocol.GoalActive
	resumed.Reason = nil
	g.current = &resumed
	return ownedGoalResult(g.current), nil
}

func activeProtocolGoal() *protocol.Goal {
	maxRuns := 3
	return &protocol.Goal{
		SessionID: "ses_1", Objective: "finish", Status: protocol.GoalActive,
		Provider: "openai", Model: "gpt-5.6-sol",
		Budget: &protocol.GoalBudget{MaxRuns: &maxRuns}, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}
}

func TestGoalAdapterProjectsTheCompleteLifecycle(t *testing.T) {
	stub := &goalBindingStub{t: t}
	runtime := &Connection{goals: stub, meta: requestMeta("test")}
	if _, exists, err := runtime.GetGoal(t.Context(), "ses_1"); err != nil || exists {
		t.Fatalf("empty GetGoal = (%t, %v)", exists, err)
	}
	started, err := runtime.StartGoal(t.Context(), protocol.StartGoalRequest{
		SessionID: "ses_1", Objective: "finish",
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", Budget: limitedGoalBudget(t, 3),
	})
	if err != nil || started.Status != protocol.GoalActive || started.ReasoningEffort != "xhigh" || stub.last != "start" {
		t.Fatalf("StartGoal = (%+v, %v), last %q", started, err, stub.last)
	}
	updated, err := runtime.UpdateGoal(t.Context(), protocol.UpdateGoalRequest{SessionID: "ses_1", Objective: "ship"})
	if err != nil || updated.Objective != "ship" || stub.last != "update" {
		t.Fatalf("UpdateGoal = (%+v, %v), last %q", updated, err, stub.last)
	}
	stopped, err := runtime.StopGoal(t.Context(), "ses_1")
	if err != nil || stopped.Status != protocol.GoalPaused || stopped.Reason == nil || stub.last != "stop" {
		t.Fatalf("StopGoal = (%+v, %v), last %q", stopped, err, stub.last)
	}
	resumed, err := runtime.ResumeGoal(t.Context(), "ses_1")
	if err != nil || resumed.Status != protocol.GoalActive || stub.last != "resume" {
		t.Fatalf("ResumeGoal = (%+v, %v), last %q", resumed, err, stub.last)
	}
	completing := *stub.current
	completing.Status = protocol.GoalCompleting
	stub.current = &completing
	observed, exists, err := runtime.GetGoal(t.Context(), "ses_1")
	if err != nil || !exists || observed.Status != protocol.GoalCompleting || observed.Reason != nil {
		t.Fatalf("completing GetGoal = (%+v, %t, %v)", observed, exists, err)
	}
	if err := runtime.ClearGoal(t.Context(), "ses_1"); err != nil || stub.last != "clear" {
		t.Fatalf("ClearGoal = %v, last %q", err, stub.last)
	}
}

func TestGoalAdapterAcceptsRuntimeResolvedInheritedSelection(t *testing.T) {
	t.Parallel()
	resolved := activeProtocolGoal()
	resolved.Provider = "deepseek"
	resolved.Model = "deepseek-chat"
	stub := &goalBindingStub{t: t, startResult: resolved}
	runtime := &Connection{goals: stub, meta: requestMeta("test")}

	started, err := runtime.StartGoal(t.Context(), protocol.StartGoalRequest{
		SessionID: "ses_1", Objective: "finish", Budget: limitedGoalBudget(t, 3),
	})
	if err != nil {
		t.Fatalf("StartGoal with inherited selection: %v", err)
	}
	if started.Provider != "deepseek" || started.Model != "deepseek-chat" {
		t.Fatalf("resolved selection = %s/%s", started.Provider, started.Model)
	}
}

func TestGoalAdapterRejectsAResponseForAnotherSession(t *testing.T) {
	t.Parallel()
	stub := &goalBindingStub{t: t, current: activeProtocolGoal()}
	runtime := &Connection{goals: stub, meta: requestMeta("test")}

	_, _, err := runtime.GetGoal(t.Context(), "ses_other")
	requireRuntimeContractViolation(t, err)
}

func TestGoalAdapterRejectsInvalidNestedBudgetBeforeCallingRuntime(t *testing.T) {
	t.Parallel()
	stub := &goalBindingStub{t: t}
	runtime := &Connection{goals: stub, meta: requestMeta("test")}
	invalid := -1

	_, err := runtime.StartGoal(t.Context(), protocol.StartGoalRequest{
		SessionID: "ses_1", Objective: "finish",
		Budget: &protocol.GoalBudget{MaxRuns: &invalid},
	})
	if err == nil {
		t.Fatal("StartGoal accepted a negative nested budget")
	}
	if stub.last != "" {
		t.Fatalf("StartGoal called runtime before validating nested budget: %q", stub.last)
	}
}

func TestGoalAdapterRejectsInvalidSessionIdentityBeforeCallingRuntime(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Connection) error
	}{
		{name: "get", invoke: func(runtime *Connection) error {
			_, _, err := runtime.GetGoal(t.Context(), " ses_1 ")
			return err
		}},
		{name: "clear", invoke: func(runtime *Connection) error {
			return runtime.ClearGoal(t.Context(), " ses_1 ")
		}},
		{name: "stop", invoke: func(runtime *Connection) error {
			_, err := runtime.StopGoal(t.Context(), " ses_1 ")
			return err
		}},
		{name: "resume", invoke: func(runtime *Connection) error {
			_, err := runtime.ResumeGoal(t.Context(), " ses_1 ")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &goalBindingStub{t: t, current: activeProtocolGoal()}
			runtime := &Connection{goals: stub, meta: requestMeta("test")}
			if err := test.invoke(runtime); err == nil {
				t.Fatal("invalid session identity was accepted")
			}
			if stub.last != "" || stub.getCalls != 0 {
				t.Fatalf("invalid session identity reached Runtime: last=%q getCalls=%d", stub.last, stub.getCalls)
			}
		})
	}
}

func TestGoalAdapterRejectsInvalidNestedUsageInRuntimeResult(t *testing.T) {
	t.Parallel()
	goal := activeProtocolGoal()
	goal.Used.Steps = -1
	runtime := &Connection{
		goals: &goalBindingStub{t: t, current: goal},
		meta:  requestMeta("test"),
	}

	_, _, err := runtime.GetGoal(t.Context(), "ses_1")
	requireRuntimeContractViolation(t, err)
}

func TestGoalAdapterRejectsMissingLifecycleTimes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		field string
		clear func(*protocol.Goal)
	}{
		{name: "creation", field: "createdAt", clear: func(value *protocol.Goal) { value.CreatedAt = time.Time{} }},
		{name: "update", field: "updatedAt", clear: func(value *protocol.Goal) { value.UpdatedAt = time.Time{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			goal := activeProtocolGoal()
			test.clear(goal)
			runtime := &Connection{goals: &goalBindingStub{t: t, current: goal}, meta: requestMeta("test")}
			_, _, err := runtime.GetGoal(t.Context(), "ses_1")
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("GetGoal error = %v, want %q", err, test.field)
			}
		})
	}
}

func TestGoalAdapterRejectsMutationAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	paused := *activeProtocolGoal()
	paused.Status = protocol.GoalPaused
	paused.Reason = &protocol.GoalReason{Code: protocol.GoalReasonStoppedByUser}
	tests := []struct {
		name   string
		stub   *goalBindingStub
		invoke func(*Connection) error
	}{
		{
			name: "start fields",
			stub: &goalBindingStub{startResult: func() *protocol.Goal {
				result := *activeProtocolGoal()
				result.Objective = "ignored"
				return &result
			}()},
			invoke: func(runtime *Connection) error {
				_, err := runtime.StartGoal(t.Context(), protocol.StartGoalRequest{
					SessionID: "ses_1", Objective: "finish", Budget: limitedGoalBudget(t, 3),
				})
				return err
			},
		},
		{
			name: "stop remains active",
			stub: &goalBindingStub{stopResult: activeProtocolGoal()},
			invoke: func(runtime *Connection) error {
				_, err := runtime.StopGoal(t.Context(), "ses_1")
				return err
			},
		},
		{
			name: "update objective",
			stub: &goalBindingStub{current: activeProtocolGoal(), updateResult: func() *protocol.Goal {
				result := *activeProtocolGoal()
				result.Objective = "ignored"
				return &result
			}()},
			invoke: func(runtime *Connection) error {
				_, err := runtime.UpdateGoal(t.Context(), protocol.UpdateGoalRequest{SessionID: "ses_1", Objective: "ship"})
				return err
			},
		},
		{
			name: "resume remains paused",
			stub: &goalBindingStub{resumeResult: &paused},
			invoke: func(runtime *Connection) error {
				_, err := runtime.ResumeGoal(t.Context(), "ses_1")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.stub.t = t
			runtime := &Connection{goals: test.stub, meta: requestMeta("test")}
			requireRuntimeContractViolation(t, test.invoke(runtime))
		})
	}
}

func limitedGoalBudget(t testing.TB, maxRuns int) *protocol.GoalBudget {
	t.Helper()
	return &protocol.GoalBudget{MaxRuns: &maxRuns}
}

func ownedGoalResult(value *protocol.Goal) *protocol.Goal {
	if value == nil {
		return nil
	}
	owned := cloneGoal(*value)
	return &owned
}
