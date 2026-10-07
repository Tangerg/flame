package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type modelCatalogBinding interface {
	ListProviders(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.Provider], error)
	ListModels(context.Context, protocol.ListModelsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.Model], error)
}

type approvalBinding interface {
	SetApprovalRule(context.Context, protocol.SetApprovalRuleRequest, flameruntime.CommandOptions) error
	GetApprovalMode(context.Context, flameruntime.CallOptions) (*protocol.ApprovalModeResult, error)
	SetApprovalMode(context.Context, protocol.SetApprovalModeRequest, flameruntime.CommandOptions) (*protocol.ApprovalModeResult, error)
	ListApprovalRules(context.Context, protocol.ListApprovalRulesRequest, flameruntime.CallOptions) (*protocol.ListApprovalRulesResult, error)
	ForgetApprovalRule(context.Context, protocol.ForgetApprovalRuleRequest, flameruntime.CommandOptions) error
}

// ListModels returns every successfully discovered provider catalog together
// with provider-qualified discovery errors. Cancellation, a closed Runtime, or
// a protocol violation invalidates the whole read and returns no models.
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
	var discoveryErrors []error
	seenProviders := make(map[string]struct{}, len(providerValues))
	seenModels := make(map[string]struct{})
	for _, provider := range providerValues {
		if _, duplicate := seenProviders[provider.ID]; duplicate {
			return nil, runtimeContractViolation("model catalog repeats provider %q", provider.ID)
		}
		seenProviders[provider.ID] = struct{}{}
		page, err := r.modelCatalog.ListModels(ctx, protocol.ListModelsRequest{Provider: provider.ID}, r.callOptions())
		if err != nil {
			err = classifyError(err)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
				errors.Is(err, conversation.ErrDisconnected) || errors.Is(err, conversation.ErrIncompatibleRuntime) {
				return nil, err
			}
			discoveryErrors = append(discoveryErrors, fmt.Errorf("%s: %w", provider.ID, err))
			continue
		}
		values, err := requireCompletePage("list models for "+provider.ID, page)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			if value.Provider != provider.ID {
				return nil, runtimeContractViolation("models for provider %q returned model %q from %q", provider.ID, value.ID, value.Provider)
			}
			identity := value.Provider + "\x00" + value.ID
			if _, duplicate := seenModels[identity]; duplicate {
				return nil, runtimeContractViolation("models for provider %q repeats model %q", provider.ID, value.ID)
			}
			seenModels[identity] = struct{}{}
			models = append(models, value)
		}
	}
	return models, errors.Join(discoveryErrors...)
}

func (r *Connection) GetApprovalMode(ctx context.Context) (protocol.ApprovalModeResult, error) {
	result, err := r.approvals.GetApprovalMode(ctx, r.callOptions())
	if err != nil {
		return protocol.ApprovalModeResult{}, classifyError(err)
	}
	if result == nil {
		return protocol.ApprovalModeResult{}, runtimeContractViolation("get approval mode returned nil")
	}
	return *result, nil
}

func (r *Connection) SetApprovalMode(ctx context.Context, mode protocol.ApprovalMode) (protocol.ApprovalModeResult, error) {
	request := protocol.SetApprovalModeRequest{Mode: mode}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.ApprovalModeResult{}, err
	}
	options := r.commandOptions()
	result, err := r.approvals.SetApprovalMode(ctx, request, options)
	if err != nil {
		return protocol.ApprovalModeResult{}, classifyError(err)
	}
	if result == nil {
		return protocol.ApprovalModeResult{}, runtimeContractViolation("set approval mode returned nil")
	}
	return *result, nil
}

func (r *Connection) ListApprovalRules(ctx context.Context, sessionID string) ([]protocol.ApprovalRule, error) {
	request := protocol.ListApprovalRulesRequest{SessionID: sessionID}
	if err := request.ValidateWire(); err != nil {
		return nil, fmt.Errorf("list approval rules: %w", err)
	}
	result, err := r.approvals.ListApprovalRules(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("list approval rules returned nil")
	}
	seen := make(map[string]struct{}, len(result.Rules))
	for _, rule := range result.Rules {
		if _, duplicate := seen[rule.ID]; duplicate {
			return nil, runtimeContractViolation("list approval rules repeats %q", rule.ID)
		}
		seen[rule.ID] = struct{}{}
	}
	return result.Rules, nil
}

func (r *Connection) DeleteApprovalRule(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete approval rule: id is empty")
	}
	options := r.commandOptions()
	return classifyError(r.approvals.ForgetApprovalRule(ctx, protocol.ForgetApprovalRuleRequest{ID: id}, options))
}

func (r *Connection) SetApprovalRule(ctx context.Context, request protocol.SetApprovalRuleRequest) error {
	if err := protocol.ValidateWireTree(request); err != nil {
		return err
	}
	return classifyError(r.approvals.SetApprovalRule(ctx, request, r.commandOptions()))
}
