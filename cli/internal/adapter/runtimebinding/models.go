package runtimebinding

import (
	"context"
	"errors"
	"fmt"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/integration/models"
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

func (r *Connection) Roles(ctx context.Context) (models.Roles, error) {
	utility, err := r.modelConfig.GetUtilityRole(ctx, r.callOptions())
	if err != nil {
		return models.Roles{}, classifyError(err)
	}
	if utility == nil {
		return models.Roles{}, runtimeContractViolation("model roles returned nil utility role")
	}
	embedding, err := r.modelConfig.GetEmbeddingRole(ctx, r.callOptions())
	if err != nil {
		return models.Roles{}, classifyError(err)
	}
	if embedding == nil {
		return models.Roles{}, runtimeContractViolation("model roles returned nil embedding role")
	}
	utilityRole, err := projectUtilityRole(*utility)
	if err != nil {
		return models.Roles{}, runtimeContractViolation("model roles returned an invalid utility role: %v", err)
	}
	embeddingRole, err := projectEmbeddingRole(*embedding)
	if err != nil {
		return models.Roles{}, runtimeContractViolation("model roles returned an invalid embedding role: %v", err)
	}
	roles := models.Roles{Utility: utilityRole, Embedding: embeddingRole}
	if err := roles.Validate(); err != nil {
		return models.Roles{}, runtimeContractViolation("model roles returned an invalid projection: %v", err)
	}
	return roles, nil
}

func (r *Connection) SetRole(ctx context.Context, role models.Role) (models.Role, error) {
	if err := role.Validate(); err != nil {
		return models.Role{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return models.Role{}, err
	}
	provider, model, _ := role.ProviderModel()
	var projected models.Role
	switch role.Kind() {
	case models.UtilityRole:
		result, callErr := r.modelConfig.SetUtilityRole(ctx, protocol.UtilityRole{Provider: provider, Model: model}, options)
		if callErr != nil {
			return models.Role{}, classifyError(callErr)
		}
		if result == nil {
			return models.Role{}, runtimeContractViolation("set utility role returned nil")
		}
		projected, callErr = projectUtilityRole(*result)
		if callErr != nil {
			return models.Role{}, runtimeContractViolation("set utility role returned an invalid role: %v", callErr)
		}
	case models.EmbeddingRole:
		result, callErr := r.modelConfig.SetEmbeddingRole(ctx, protocol.EmbeddingRole{Provider: provider, Model: model}, options)
		if callErr != nil {
			return models.Role{}, classifyError(callErr)
		}
		if result == nil {
			return models.Role{}, runtimeContractViolation("set embedding role returned nil")
		}
		projected, callErr = projectEmbeddingRole(*result)
		if callErr != nil {
			return models.Role{}, runtimeContractViolation("set embedding role returned an invalid role: %v", callErr)
		}
	}
	if err := projected.Validate(); err != nil {
		return models.Role{}, runtimeContractViolation("set model role returned an invalid projection: %v", err)
	}
	if projected != role {
		projectedLabel, projectedErr := projected.Label()
		roleLabel, roleErr := role.Label()
		if projectedErr != nil || roleErr != nil {
			return models.Role{}, runtimeContractViolation("set %s role acknowledgement could not be described", role.Kind())
		}
		return models.Role{}, runtimeContractViolation("set %s role returned %q for %q", role.Kind(), projectedLabel, roleLabel)
	}
	return projected, nil
}

func projectUtilityRole(role protocol.UtilityRole) (models.Role, error) {
	if role.Provider == "" && role.Model == "" {
		return models.InheritedUtilityRole(), nil
	}
	return models.NewConfiguredRole(models.UtilityRole, role.Provider, role.Model)
}

func projectEmbeddingRole(role protocol.EmbeddingRole) (models.Role, error) {
	if role.Provider == "" && role.Model == "" {
		return models.DisabledEmbeddingRole(), nil
	}
	return models.NewConfiguredRole(models.EmbeddingRole, role.Provider, role.Model)
}

func (r *Connection) Providers(ctx context.Context) ([]models.Provider, error) {
	result, err := r.modelConfig.ListProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list providers", result)
	if err != nil {
		return nil, err
	}
	return projectUniqueValuesFallible("list providers", values, projectProvider, func(provider models.Provider) string {
		return provider.ID()
	})
}

func (r *Connection) UpdateProvider(ctx context.Context, update models.UpdateProvider) (models.Provider, error) {
	if err := update.Validate(); err != nil {
		return models.Provider{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return models.Provider{}, err
	}
	request := protocol.UpdateProviderRequest{Provider: update.Provider}
	request.BaseURL = projectProviderChange(update.BaseURL)
	request.APIKey = projectProviderChange(update.APIKey)
	result, err := r.modelConfig.UpdateProvider(ctx, request, options)
	if err != nil {
		return models.Provider{}, classifyError(err)
	}
	if result == nil {
		return models.Provider{}, runtimeContractViolation("update provider returned nil")
	}
	provider, err := projectProvider(*result)
	if err != nil {
		return models.Provider{}, runtimeContractViolation("update provider returned an invalid provider: %v", err)
	}
	if provider.ID() != update.Provider {
		return models.Provider{}, runtimeContractViolation("update provider returned id %q for %q", provider.ID(), update.Provider)
	}
	if err := validateProviderUpdate(update, provider); err != nil {
		return models.Provider{}, runtimeContractViolation("update provider returned an invalid acknowledgement: %v", err)
	}
	return provider, nil
}

func validateProviderUpdate(update models.UpdateProvider, result models.Provider) error {
	var problems []error
	if change := update.BaseURL; change != nil {
		baseURL, present := result.BaseURL()
		switch change.Kind {
		case models.SetValue:
			if !present || baseURL != change.Value {
				problems = append(problems, fmt.Errorf("runtime returned base URL %q (present %v), want %q", baseURL, present, change.Value))
			}
		case models.ClearValue:
			if present {
				problems = append(problems, fmt.Errorf("runtime retained base URL %q after clearing it", baseURL))
			}
		}
	}
	if change := update.APIKey; change != nil {
		credential, configured := result.Credential()
		switch change.Kind {
		case models.SetValue:
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
		case models.ClearValue:
			// Clearing a stored key may reveal a read-only environment fallback,
			// but the effective credential must no longer claim stored ownership.
			if configured && credential.Stored() {
				problems = append(problems, errors.New("runtime still reports a stored API key after clearing it"))
			}
		}
	}
	return errors.Join(problems...)
}

func (r *Connection) TestProvider(ctx context.Context, providerID string) (models.TestResult, error) {
	if err := protocol.ValidateProviderIdentity(providerID); err != nil {
		return models.TestResult{}, fmt.Errorf("test provider: %w", err)
	}
	result, err := r.modelConfig.TestProvider(ctx, protocol.TestProviderRequest{Provider: providerID}, r.callOptions())
	if err != nil {
		return models.TestResult{}, classifyError(err)
	}
	if result == nil {
		return models.TestResult{}, runtimeContractViolation("test provider returned nil")
	}
	projected := models.TestResult{OK: result.OK, Problem: projectRuntimeProblem(result.Error)}
	if err := projected.Validate(); err != nil {
		return models.TestResult{}, runtimeContractViolation("test provider returned an invalid result: %v", err)
	}
	return projected, nil
}

func projectProvider(value protocol.Provider) (models.Provider, error) {
	var credential *models.Credential
	if value.Credential != nil {
		projected, err := models.NewCredential(
			value.Credential.Masked,
			models.KeySource(value.Credential.Source),
		)
		if err != nil {
			return models.Provider{}, err
		}
		credential = &projected
	}
	return models.NewProvider(models.ProviderSpec{
		ID: value.ID, BaseURL: value.BaseURL, Credential: credential, Configured: value.Configured,
		CredentialRequirement: models.CredentialRequirement(value.CredentialRequirement),
		RequiresBaseURL:       value.RequiresBaseURL, EmbeddingCapable: value.EmbeddingCapable,
	})
}

func projectProviderChange(change *models.ValueChange) *protocol.ProviderConfigChange {
	if change == nil {
		return nil
	}
	projected := &protocol.ProviderConfigChange{Type: protocol.ProviderConfigChangeType(change.Kind)}
	if change.Kind == models.SetValue {
		value := change.Value
		projected.Value = &value
	}
	return projected
}
