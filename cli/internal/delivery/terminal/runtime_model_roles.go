package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/application/integration/models"
)

func (a *app) ShowModelRoles() {
	if a.modelConfig == nil {
		a.message("this runtime composition has no model configuration service")
		return
	}
	a.executeRuntimeReaderQuery(a.modelRolesReaderQuery())
}

func (a *app) modelRolesReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading model roles", mode: runtimeReaderModelRoles,
		read: func(ctx context.Context) (readerDocument, error) {
			roles, err := a.modelConfig.Roles(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return modelRolesDocument(roles)
		},
	}
}

func modelRolesDocument(roles models.Roles) (readerDocument, error) {
	if err := roles.Validate(); err != nil {
		return readerDocument{}, fmt.Errorf("model roles document: %w", err)
	}
	utility, err := roles.Utility.Label()
	if err != nil {
		return readerDocument{}, fmt.Errorf("utility model role: %w", err)
	}
	embedding, err := roles.Embedding.Label()
	if err != nil {
		return readerDocument{}, fmt.Errorf("embedding model role: %w", err)
	}
	return paragraphDocument("Auxiliary model roles", "runtime configuration", []string{
		"utility    " + utility,
		"embedding  " + embedding,
	}), nil
}

func (a *app) SetModelRole(kind models.RoleKind, argument string) error {
	if a.modelConfig == nil {
		return errors.New("this runtime composition has no model configuration service")
	}
	role, err := parseModelRole(kind, argument)
	if err != nil {
		return err
	}
	a.status.note("updating " + string(kind) + " model role")
	started := a.runAdmissionMutation(modelConfigOperation, false,
		func(ctx context.Context) (models.Role, error) { return a.modelConfig.SetRole(ctx, role) },
		func(updated models.Role, err error) {
			if err != nil {
				a.message("update " + string(kind) + " role failed: " + err.Error())
				return
			}
			label, labelErr := updated.Label()
			if labelErr != nil {
				a.message("update " + string(kind) + " role returned invalid state: " + labelErr.Error())
				return
			}
			a.message(string(kind) + " model · " + label)
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func parseModelRole(kind models.RoleKind, argument string) (models.Role, error) {
	if err := kind.Validate(); err != nil {
		return models.Role{}, err
	}
	switch {
	case kind == models.UtilityRole && strings.EqualFold(argument, utilityRoleInheritedArgument):
		return models.InheritedUtilityRole(), nil
	case kind == models.EmbeddingRole && strings.EqualFold(argument, embeddingRoleDisabledArgument):
		return models.DisabledEmbeddingRole(), nil
	}
	provider, model, found := strings.Cut(argument, "/")
	if !found {
		return models.Role{}, modelRoleUsage(kind)
	}
	role, err := models.NewConfiguredRole(kind, provider, model)
	if err != nil {
		return models.Role{}, modelRoleUsage(kind)
	}
	return role, nil
}

const (
	utilityRoleInheritedArgument  = "inherit"
	embeddingRoleDisabledArgument = "off"
)

func modelRoleUsage(kind models.RoleKind) error {
	if kind == models.UtilityRole {
		return errors.New("usage: /utility <provider/model|inherit>")
	}
	return errors.New("usage: /embedding <provider/model|off>")
}
