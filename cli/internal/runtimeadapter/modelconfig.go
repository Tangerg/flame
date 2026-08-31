package runtimeadapter

import (
	"context"
	"errors"
	"fmt"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	cliidentity "github.com/Tangerg/flame/cli/internal/identity"
	"github.com/Tangerg/flame/cli/internal/modelconfig"
)

type modelConfigBinding interface {
	GetUtilityRole(context.Context, flameruntime.CallOptions) (*protocol.UtilityRole, error)
	SetUtilityRole(context.Context, protocol.UtilityRole, flameruntime.CommandOptions) (*protocol.UtilityRole, error)
	GetEmbeddingRole(context.Context, flameruntime.CallOptions) (*protocol.EmbeddingRole, error)
	SetEmbeddingRole(context.Context, protocol.EmbeddingRole, flameruntime.CommandOptions) (*protocol.EmbeddingRole, error)
	ListProviders(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.Provider], error)
	UpdateProvider(context.Context, protocol.UpdateProviderRequest, flameruntime.CommandOptions) (*protocol.Provider, error)
	TestProvider(context.Context, protocol.TestProviderRequest, flameruntime.CallOptions) (*protocol.ProviderTestResult, error)
}

var _ modelconfig.Service = (*Connection)(nil)

func (r *Connection) Roles(ctx context.Context) (modelconfig.Roles, error) {
	utility, err := r.modelConfig.GetUtilityRole(ctx, r.callOptions())
	if err != nil {
		return modelconfig.Roles{}, classifyError(err)
	}
	if utility == nil {
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned nil utility role")
	}
	embedding, err := r.modelConfig.GetEmbeddingRole(ctx, r.callOptions())
	if err != nil {
		return modelconfig.Roles{}, classifyError(err)
	}
	if embedding == nil {
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned nil embedding role")
	}
	utilityRole, err := projectUtilityRole(*utility)
	if err != nil {
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned an invalid utility role: %v", err)
	}
	embeddingRole, err := projectEmbeddingRole(*embedding)
	if err != nil {
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned an invalid embedding role: %v", err)
	}
	roles := modelconfig.Roles{Utility: utilityRole, Embedding: embeddingRole}
	if err := roles.Validate(); err != nil {
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned an invalid projection: %v", err)
	}
	return roles, nil
}

func (r *Connection) SetRole(ctx context.Context, role modelconfig.Role) (modelconfig.Role, error) {
	if err := role.Validate(); err != nil {
		return modelconfig.Role{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return modelconfig.Role{}, err
	}
	provider, model, _ := role.ProviderModel()
	var projected modelconfig.Role
	switch role.Kind() {
	case modelconfig.UtilityRole:
		result, callErr := r.modelConfig.SetUtilityRole(ctx, protocol.UtilityRole{Provider: provider, Model: model}, options)
		if callErr != nil {
			return modelconfig.Role{}, classifyError(callErr)
		}
		if result == nil {
			return modelconfig.Role{}, runtimeContractViolation("set utility role returned nil")
		}
		projected, callErr = projectUtilityRole(*result)
		if callErr != nil {
			return modelconfig.Role{}, runtimeContractViolation("set utility role returned an invalid role: %v", callErr)
		}
	case modelconfig.EmbeddingRole:
		result, callErr := r.modelConfig.SetEmbeddingRole(ctx, protocol.EmbeddingRole{Provider: provider, Model: model}, options)
		if callErr != nil {
			return modelconfig.Role{}, classifyError(callErr)
		}
		if result == nil {
			return modelconfig.Role{}, runtimeContractViolation("set embedding role returned nil")
		}
		projected, callErr = projectEmbeddingRole(*result)
		if callErr != nil {
			return modelconfig.Role{}, runtimeContractViolation("set embedding role returned an invalid role: %v", callErr)
		}
	}
	if err := projected.Validate(); err != nil {
		return modelconfig.Role{}, runtimeContractViolation("set model role returned an invalid projection: %v", err)
	}
	if projected != role {
		projectedLabel, projectedErr := projected.Label()
		roleLabel, roleErr := role.Label()
		if projectedErr != nil || roleErr != nil {
			return modelconfig.Role{}, runtimeContractViolation("set %s role acknowledgement could not be described", role.Kind())
		}
		return modelconfig.Role{}, runtimeContractViolation("set %s role returned %q for %q", role.Kind(), projectedLabel, roleLabel)
	}
	return projected, nil
}

func projectUtilityRole(role protocol.UtilityRole) (modelconfig.Role, error) {
	if role.Provider == "" && role.Model == "" {
		return modelconfig.InheritedUtilityRole(), nil
	}
	return modelconfig.NewConfiguredRole(modelconfig.UtilityRole, role.Provider, role.Model)
}

func projectEmbeddingRole(role protocol.EmbeddingRole) (modelconfig.Role, error) {
	if role.Provider == "" && role.Model == "" {
		return modelconfig.DisabledEmbeddingRole(), nil
	}
	return modelconfig.NewConfiguredRole(modelconfig.EmbeddingRole, role.Provider, role.Model)
}

func (r *Connection) Providers(ctx context.Context) ([]modelconfig.Provider, error) {
	result, err := r.modelConfig.ListProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list providers", result)
	if err != nil {
		return nil, err
	}
	return projectUniqueValuesFallible("list providers", values, projectProvider, func(provider modelconfig.Provider) string {
		return provider.ID()
	})
}

func (r *Connection) UpdateProvider(ctx context.Context, update modelconfig.UpdateProvider) (modelconfig.Provider, error) {
	if err := update.Validate(); err != nil {
		return modelconfig.Provider{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return modelconfig.Provider{}, err
	}
	request := protocol.UpdateProviderRequest{Provider: update.Provider}
	request.BaseURL = projectProviderChange(update.BaseURL)
	request.APIKey = projectProviderChange(update.APIKey)
	result, err := r.modelConfig.UpdateProvider(ctx, request, options)
	if err != nil {
		return modelconfig.Provider{}, classifyError(err)
	}
	if result == nil {
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned nil")
	}
	provider, err := projectProvider(*result)
	if err != nil {
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned an invalid provider: %v", err)
	}
	if provider.ID() != update.Provider {
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned id %q for %q", provider.ID(), update.Provider)
	}
	if err := validateProviderUpdate(update, provider); err != nil {
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned an invalid acknowledgement: %v", err)
	}
	return provider, nil
}

func validateProviderUpdate(update modelconfig.UpdateProvider, result modelconfig.Provider) error {
	var problems []error
	if change := update.BaseURL; change != nil {
		baseURL, present := result.BaseURL()
		switch change.Kind {
		case modelconfig.SetValue:
			if !present || baseURL != change.Value {
				problems = append(problems, fmt.Errorf("runtime returned base URL %q (present %v), want %q", baseURL, present, change.Value))
			}
		case modelconfig.ClearValue:
			if present {
				problems = append(problems, fmt.Errorf("runtime retained base URL %q after clearing it", baseURL))
			}
		}
	}
	if change := update.APIKey; change != nil {
		credential, configured := result.Credential()
		switch change.Kind {
		case modelconfig.SetValue:
			if !configured || !credential.Stored() {
				problems = append(problems, fmt.Errorf(
					"runtime returned configured=%v source=%q after setting a stored key",
					configured,
					credential.Source(),
				))
			}
			if configured && credential.Exposes(change.Value) {
				problems = append(problems, errors.New("runtime exposed the raw API key instead of a mask"))
			}
		case modelconfig.ClearValue:
			// Clearing a stored key may reveal a read-only environment fallback,
			// but the effective credential must no longer claim stored ownership.
			if configured && credential.Stored() {
				problems = append(problems, errors.New("runtime still reports a stored API key after clearing it"))
			}
		}
	}
	return errors.Join(problems...)
}

func (r *Connection) TestProvider(ctx context.Context, providerID string) (modelconfig.TestResult, error) {
	if err := cliidentity.ValidateProvider(providerID); err != nil {
		return modelconfig.TestResult{}, fmt.Errorf("test provider: %w", err)
	}
	result, err := r.modelConfig.TestProvider(ctx, protocol.TestProviderRequest{Provider: providerID}, r.callOptions())
	if err != nil {
		return modelconfig.TestResult{}, classifyError(err)
	}
	if result == nil {
		return modelconfig.TestResult{}, runtimeContractViolation("test provider returned nil")
	}
	projected := modelconfig.TestResult{OK: result.OK, Problem: projectRuntimeProblem(result.Error)}
	if err := projected.Validate(); err != nil {
		return modelconfig.TestResult{}, runtimeContractViolation("test provider returned an invalid result: %v", err)
	}
	return projected, nil
}

func projectProvider(value protocol.Provider) (modelconfig.Provider, error) {
	var credential *modelconfig.Credential
	if value.Credential != nil {
		projected, err := modelconfig.NewCredential(
			value.Credential.Masked,
			modelconfig.KeySource(value.Credential.Source),
		)
		if err != nil {
			return modelconfig.Provider{}, err
		}
		credential = &projected
	}
	return modelconfig.NewProvider(modelconfig.ProviderSpec{
		ID: value.ID, BaseURL: value.BaseURL, Credential: credential, Configured: value.Configured,
		CredentialRequirement: modelconfig.CredentialRequirement(value.CredentialRequirement),
		RequiresBaseURL:       value.RequiresBaseURL, EmbeddingCapable: value.EmbeddingCapable,
	})
}

func projectProviderChange(change *modelconfig.ValueChange) *protocol.ProviderConfigChange {
	if change == nil {
		return nil
	}
	projected := &protocol.ProviderConfigChange{Type: protocol.ProviderConfigChangeType(change.Kind)}
	if change.Kind == modelconfig.SetValue {
		value := change.Value
		projected.Value = &value
	}
	return projected
}
