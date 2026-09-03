package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type modelCatalogBinding interface {
	ListProviders(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.Provider], error)
	ListModels(context.Context, protocol.ListModelsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.Model], error)
}

type approvalBinding interface {
	GetApprovalMode(context.Context, flameruntime.CallOptions) (*protocol.ApprovalModeResult, error)
	SetApprovalMode(context.Context, protocol.SetApprovalModeRequest, flameruntime.CommandOptions) (*protocol.ApprovalModeResult, error)
	ListApprovalRules(context.Context, protocol.ListApprovalRulesRequest, flameruntime.CallOptions) (*protocol.ListApprovalRulesResult, error)
	ForgetApprovalRule(context.Context, protocol.ForgetApprovalRuleRequest, flameruntime.CommandOptions) error
}

func (r *Connection) ListModels(ctx context.Context) ([]protocol.Model, error) {
	providers, err := r.modelCatalog.ListProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	providerValues, err := requireCompletePage("list providers", providers)
	if err != nil {
		return nil, err
	}

	var models []protocol.Model
	seenProviders := make(map[string]struct{}, len(providerValues))
	seenModels := make(map[string]struct{})
	for _, provider := range providerValues {
		if err := protocol.ValidateWireTree(provider); err != nil {
			return nil, runtimeContractViolation("model catalog returned an invalid provider: %v", err)
		}
		if _, duplicate := seenProviders[provider.ID]; duplicate {
			return nil, runtimeContractViolation("model catalog repeats provider %q", provider.ID)
		}
		seenProviders[provider.ID] = struct{}{}
		page, err := r.modelCatalog.ListModels(ctx, protocol.ListModelsRequest{Provider: provider.ID}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		values, err := requireCompletePage("list models for "+provider.ID, page)
		if err != nil {
			return nil, err
		}
		for index, value := range values {
			if value.Provider != provider.ID {
				return nil, runtimeContractViolation("models for provider %q returned model %q from %q", provider.ID, value.ID, value.Provider)
			}
			if err := protocol.ValidateWireTree(value); err != nil {
				return nil, runtimeContractViolation("models for provider %q returned invalid item %d: %v", provider.ID, index+1, err)
			}
			identity := value.Provider + "\x00" + value.ID
			if _, duplicate := seenModels[identity]; duplicate {
				return nil, runtimeContractViolation("models for provider %q repeats model %q", provider.ID, value.ID)
			}
			seenModels[identity] = struct{}{}
			models = append(models, cloneProtocolModel(value))
		}
	}
	return models, nil
}

func cloneProtocolModel(value protocol.Model) protocol.Model {
	if value.TokenLimits != nil {
		limits := *value.TokenLimits
		limits.ContextWindow = clonePointer(limits.ContextWindow)
		limits.MaxInputTokens = clonePointer(limits.MaxInputTokens)
		limits.MaxOutputTokens = clonePointer(limits.MaxOutputTokens)
		value.TokenLimits = &limits
	}
	if value.Capabilities != nil {
		capabilities := *value.Capabilities
		capabilities.ReasoningLevels = slices.Clone(capabilities.ReasoningLevels)
		capabilities.InputModalities = slices.Clone(capabilities.InputModalities)
		capabilities.OutputModalities = slices.Clone(capabilities.OutputModalities)
		value.Capabilities = &capabilities
	}
	if value.Pricing != nil {
		pricing := *value.Pricing
		value.Pricing = &pricing
	}
	return value
}

func (r *Connection) GetApprovalMode(ctx context.Context) (protocol.ApprovalMode, error) {
	result, err := r.approvals.GetApprovalMode(ctx, r.callOptions())
	if err != nil {
		return "", classifyError(err)
	}
	if result == nil {
		return "", runtimeContractViolation("get approval mode returned nil")
	}
	if err := protocol.ValidateWireTree(protocol.SetApprovalModeRequest{Mode: result.Mode}); err != nil {
		return "", runtimeContractViolation("get approval mode returned an invalid mode: %v", err)
	}
	return result.Mode, nil
}

func (r *Connection) SetApprovalMode(ctx context.Context, mode protocol.ApprovalMode) (protocol.ApprovalMode, error) {
	request := protocol.SetApprovalModeRequest{Mode: mode}
	if err := protocol.ValidateWireTree(request); err != nil {
		return "", err
	}
	options, err := r.commandOptions()
	if err != nil {
		return "", err
	}
	result, err := r.approvals.SetApprovalMode(ctx, request, options)
	if err != nil {
		return "", classifyError(err)
	}
	if result == nil {
		return "", runtimeContractViolation("set approval mode returned nil")
	}
	applied := result.Mode
	if err := protocol.ValidateWireTree(protocol.SetApprovalModeRequest{Mode: applied}); err != nil {
		return "", runtimeContractViolation("set approval mode returned an invalid mode: %v", err)
	}
	if applied != mode {
		return "", runtimeContractViolation("set approval mode returned %q for %q", applied, mode)
	}
	return applied, nil
}

func (r *Connection) ListApprovalRules(ctx context.Context, sessionID string) ([]protocol.ApprovalRule, error) {
	if err := protocol.ValidateSessionID(sessionID); err != nil {
		return nil, fmt.Errorf("list approval rules: %w", err)
	}
	result, err := r.approvals.ListApprovalRules(ctx, protocol.ListApprovalRulesRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("list approval rules returned nil")
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return nil, runtimeContractViolation("list approval rules returned an invalid result: %v", err)
	}
	seen := make(map[string]struct{}, len(result.Rules))
	for _, rule := range result.Rules {
		if _, duplicate := seen[rule.ID]; duplicate {
			return nil, runtimeContractViolation("list approval rules repeats %q", rule.ID)
		}
		seen[rule.ID] = struct{}{}
	}
	return slices.Clone(result.Rules), nil
}

func (r *Connection) DeleteApprovalRule(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete approval rule: id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.approvals.ForgetApprovalRule(ctx, protocol.ForgetApprovalRuleRequest{ID: id}, options))
}
