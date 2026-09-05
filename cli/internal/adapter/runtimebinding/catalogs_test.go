package runtimebinding

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type approvalBindingRecorder struct {
	listRequest   protocol.ListApprovalRulesRequest
	forgetRequest protocol.ForgetApprovalRuleRequest
	forgetOptions flameruntime.CommandOptions
	listCalls     int
	forgetCalls   int
	setMode       protocol.ApprovalMode
	listResult    *protocol.ListApprovalRulesResult
}

func TestModelCatalogRetainsSuccessfulProvidersAndReportsDiscoveryFailures(t *testing.T) {
	t.Parallel()
	unavailable := errors.New("endpoint unavailable")
	denied := errors.New("discovery denied")
	stub := modelCatalogBindingStub{
		providers: protocol.NewPage([]protocol.Provider{
			{ID: "alpha", CredentialRequirement: protocol.ProviderAPIKeyOptional},
			{ID: "beta", CredentialRequirement: protocol.ProviderAPIKeyOptional},
			{ID: "gamma", CredentialRequirement: protocol.ProviderAPIKeyOptional},
		}),
		models: map[string]*protocol.Page[protocol.Model]{
			"beta": protocol.NewPage([]protocol.Model{{ID: "chat", Provider: "beta"}}),
		},
		modelErrors: map[string]error{"alpha": unavailable, "gamma": denied},
	}
	connection := &Connection{modelCatalog: stub, meta: requestMeta("test")}
	models, err := connection.ListModels(t.Context())
	if len(models) != 1 || models[0].Provider != "beta" || models[0].ID != "chat" {
		t.Fatalf("successful discovery = %+v", models)
	}
	if !errors.Is(err, unavailable) || !errors.Is(err, denied) ||
		!strings.Contains(err.Error(), "alpha: endpoint unavailable") || !strings.Contains(err.Error(), "gamma: discovery denied") {
		t.Fatalf("discovery errors = %v", err)
	}
}

func TestModelCatalogAbortsPartialResultsWhenTheReadIsInvalid(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, flameruntime.ErrClosed, protocol.ErrCapabilityNotNeg} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			connection := &Connection{modelCatalog: modelCatalogBindingStub{
				providers: protocol.NewPage([]protocol.Provider{
					{ID: "alpha", CredentialRequirement: protocol.ProviderAPIKeyOptional},
					{ID: "beta", CredentialRequirement: protocol.ProviderAPIKeyOptional},
				}),
				models: map[string]*protocol.Page[protocol.Model]{
					"alpha": protocol.NewPage([]protocol.Model{{ID: "chat", Provider: "alpha"}}),
				},
				modelErrors: map[string]error{"beta": cause},
			}, meta: requestMeta("test")}
			models, err := connection.ListModels(t.Context())
			if models != nil || !errors.Is(err, cause) {
				t.Fatalf("ListModels = (%+v, %v), want no models and %v", models, err, cause)
			}
			if cause == flameruntime.ErrClosed && !errors.Is(err, agent.ErrDisconnected) {
				t.Fatalf("closed Runtime lost classification: %v", err)
			}
		})
	}
}

func (*approvalBindingRecorder) GetApprovalMode(context.Context, flameruntime.CallOptions) (*protocol.ApprovalModeResult, error) {
	return &protocol.ApprovalModeResult{Mode: protocol.ApprovalModeBalanced}, nil
}

func (a *approvalBindingRecorder) SetApprovalMode(_ context.Context, request protocol.SetApprovalModeRequest, _ flameruntime.CommandOptions) (*protocol.ApprovalModeResult, error) {
	if a.setMode != "" {
		return &protocol.ApprovalModeResult{Mode: a.setMode}, nil
	}
	return &protocol.ApprovalModeResult{Mode: request.Mode}, nil
}

func (a *approvalBindingRecorder) ListApprovalRules(_ context.Context, request protocol.ListApprovalRulesRequest, _ flameruntime.CallOptions) (*protocol.ListApprovalRulesResult, error) {
	a.listCalls++
	a.listRequest = request
	if a.listResult != nil {
		return a.listResult, nil
	}
	return &protocol.ListApprovalRulesResult{Rules: []protocol.ApprovalRule{{
		ID: "rule_1", Scope: protocol.ApprovalRuleScopeProject, Tool: "shell",
		Subject: "go test *", Dir: "/workspace", Decision: protocol.ApprovalRuleDecisionAllow,
	}}}, nil
}

func TestCatalogsRejectResponsesOutsideTheRequestedIdentity(t *testing.T) {
	t.Parallel()
	models := &Connection{modelCatalog: modelCatalogBindingStub{
		providers: protocol.NewPage([]protocol.Provider{{ID: "deepseek", CredentialRequirement: protocol.ProviderAPIKeyRequired}}),
		models: map[string]*protocol.Page[protocol.Model]{
			"deepseek": protocol.NewPage([]protocol.Model{{ID: "chat", Provider: "other"}}),
		},
	}, meta: requestMeta("test")}
	_, err := models.ListModels(t.Context())
	requireRuntimeContractViolation(t, err)

	approvals := &Connection{approvals: &approvalBindingRecorder{setMode: protocol.ApprovalModeYolo}, meta: requestMeta("test")}
	_, err = approvals.SetApprovalMode(t.Context(), protocol.ApprovalModeSafe)
	requireRuntimeContractViolation(t, err)
}

func TestModelCatalogRejectsOutOfOrderRuntimeResults(t *testing.T) {
	t.Parallel()
	provider := func(id string) protocol.Provider {
		return protocol.Provider{ID: id, CredentialRequirement: protocol.ProviderAPIKeyRequired}
	}
	for name, stub := range map[string]modelCatalogBindingStub{
		"providers": {
			providers: protocol.NewPage([]protocol.Provider{provider("zeta"), provider("alpha")}),
			models: map[string]*protocol.Page[protocol.Model]{
				"zeta":  protocol.NewPage([]protocol.Model{}),
				"alpha": protocol.NewPage([]protocol.Model{}),
			},
		},
		"models": {
			providers: protocol.NewPage([]protocol.Provider{provider("provider")}),
			models: map[string]*protocol.Page[protocol.Model]{
				"provider": protocol.NewPage([]protocol.Model{
					{ID: "zeta", Provider: "provider"},
					{ID: "alpha", Provider: "provider"},
				}),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runtime := &Connection{modelCatalog: stub, meta: requestMeta("test")}
			_, err := runtime.ListModels(t.Context())
			requireRuntimeContractViolation(t, err)
		})
	}
}

func (a *approvalBindingRecorder) ForgetApprovalRule(_ context.Context, request protocol.ForgetApprovalRuleRequest, options flameruntime.CommandOptions) error {
	a.forgetCalls++
	a.forgetRequest = request
	a.forgetOptions = options
	return nil
}

func TestModelCatalogProjectsEveryPublishedModelField(t *testing.T) {
	t.Parallel()

	capabilities := &protocol.ModelCapabilities{
		Reasoning: true, ReasoningLevels: []string{"low", "high"}, ReasoningDefaultLevel: "high",
		Multimodal: true, InputModalities: []protocol.Modality{protocol.ModalityText, protocol.ModalityImage},
		OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true, StructuredOutput: true,
	}
	pricing := &protocol.ModelPricing{
		InputUSDPerMillionTokens: 0.2, OutputUSDPerMillionTokens: 0.8,
		CacheReadUSDPerMillionTokens: 0.02, CacheWriteUSDPerMillionTokens: 0.1,
	}
	contextWindow, maxInput, maxOutput := int64(200_000), int64(180_000), int64(20_000)
	stub := modelCatalogBindingStub{
		providers: protocol.NewPage([]protocol.Provider{{ID: "provider", CredentialRequirement: protocol.ProviderAPIKeyRequired}}),
		models: map[string]*protocol.Page[protocol.Model]{
			"provider": protocol.NewPage([]protocol.Model{{
				ID: "reasoner", Provider: "provider", DisplayName: "Reasoner",
				TokenLimits: &protocol.ModelTokenLimits{
					ContextWindow: &contextWindow, MaxInputTokens: &maxInput, MaxOutputTokens: &maxOutput,
				},
				KnowledgeCutoff: "2026-01-31", Deprecated: true,
				Capabilities: capabilities, Pricing: pricing,
			}}),
		},
	}
	runtime := &Connection{modelCatalog: stub, meta: requestMeta("test")}
	models, err := runtime.ListModels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %+v", models)
	}
	model := models[0]
	wantInput := []protocol.Modality{protocol.ModalityText, protocol.ModalityImage}
	if model.ID != "reasoner" || model.Provider != "provider" || model.DisplayName != "Reasoner" ||
		model.TokenLimits == nil || model.TokenLimits.ContextWindow == nil || *model.TokenLimits.ContextWindow != 200_000 ||
		model.TokenLimits.MaxInputTokens == nil || *model.TokenLimits.MaxInputTokens != 180_000 ||
		model.TokenLimits.MaxOutputTokens == nil || *model.TokenLimits.MaxOutputTokens != 20_000 ||
		model.KnowledgeCutoff != "2026-01-31" || !model.Deprecated || model.Capabilities == nil || model.Pricing == nil ||
		!model.Capabilities.Reasoning || model.Capabilities.ReasoningDefaultLevel != "high" ||
		!model.Capabilities.Multimodal || !model.Capabilities.ToolUse || !model.Capabilities.StructuredOutput ||
		!reflect.DeepEqual(model.Capabilities.InputModalities, wantInput) ||
		model.Pricing.CacheWriteUSDPerMillionTokens != 0.1 {
		t.Fatalf("projected model = %+v", model)
	}
	capabilities.ReasoningLevels[0] = "mutated"
	capabilities.InputModalities[0] = protocol.ModalityAudio
	pricing.InputUSDPerMillionTokens = 99
	if model.Capabilities.ReasoningLevels[0] != "low" ||
		model.Capabilities.InputModalities[0] != protocol.ModalityText ||
		model.Pricing.InputUSDPerMillionTokens != 0.2 {
		t.Fatal("model projection aliases runtime-owned metadata")
	}
}

func TestApprovalCatalogRejectsNonExactSessionIdentityBeforeRuntimeBoundary(t *testing.T) {
	t.Parallel()

	recorder := &approvalBindingRecorder{}
	runtime := &Connection{approvals: recorder, meta: requestMeta("test")}
	if _, err := runtime.ListApprovalRules(t.Context(), "  session_1  "); err == nil {
		t.Fatal("ListApprovalRules accepted a session identity that requires trimming")
	}
	if recorder.listRequest.SessionID != "" {
		t.Fatalf("invalid identity crossed the Runtime boundary: %+v", recorder.listRequest)
	}
	rules, err := runtime.ListApprovalRules(t.Context(), "session_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != "rule_1" || rules[0].Scope != protocol.ApprovalRuleScopeProject ||
		rules[0].Subject != "go test *" || rules[0].Dir != "/workspace" || rules[0].Decision != protocol.ApprovalRuleDecisionAllow {
		t.Fatalf("approval rules = %+v", rules)
	}
	if recorder.listRequest.SessionID != "session_1" {
		t.Fatalf("list request = %+v", recorder.listRequest)
	}
	if err := runtime.DeleteApprovalRule(t.Context(), "  rule_1  "); err != nil {
		t.Fatal(err)
	}
	if recorder.forgetRequest.ID != "rule_1" || recorder.forgetOptions.IdempotencyKey == "" || recorder.forgetCalls != 1 {
		t.Fatalf("forget request = %+v, options = %+v, calls = %d", recorder.forgetRequest, recorder.forgetOptions, recorder.forgetCalls)
	}

	if _, err := runtime.ListApprovalRules(t.Context(), "  "); err == nil {
		t.Fatal("empty session identity crossed the approval boundary")
	}
	if err := runtime.DeleteApprovalRule(t.Context(), "\t"); err == nil {
		t.Fatal("empty rule identity crossed the approval boundary")
	}
	if recorder.listCalls != 1 || recorder.forgetCalls != 1 {
		t.Fatalf("invalid identities reached runtime: list=%d forget=%d", recorder.listCalls, recorder.forgetCalls)
	}
}

func TestApprovalCatalogRejectsInvalidAndDuplicateRules(t *testing.T) {
	t.Parallel()
	valid := protocol.ApprovalRule{
		ID: "rule_1", Scope: protocol.ApprovalRuleScopeGlobal, Tool: "shell",
		Decision: protocol.ApprovalRuleDecisionAllow,
	}
	tests := []struct {
		name  string
		rules []protocol.ApprovalRule
	}{
		{
			name: "invalid nested rule",
			rules: []protocol.ApprovalRule{{
				ID: "rule_1", Scope: protocol.ApprovalRuleScopeProject, Tool: "shell",
				Decision: protocol.ApprovalRuleDecisionAllow,
			}},
		},
		{name: "duplicate identity", rules: []protocol.ApprovalRule{valid, valid}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &approvalBindingRecorder{listResult: &protocol.ListApprovalRulesResult{Rules: test.rules}}
			runtime := &Connection{approvals: stub, meta: requestMeta("test")}
			_, err := runtime.ListApprovalRules(t.Context(), "ses_1")
			requireRuntimeContractViolation(t, err)
		})
	}
}
