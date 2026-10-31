package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (m *modelConfigScreens) ShowModelRoles() {
	m.reader.show(m.modelRolesReaderQuery())
}

func (m *modelConfigScreens) modelRolesReaderQuery() *readerQuery {
	return &readerQuery{
		status: "loading model roles", follows: followsTopic(protocol.TopicModelsChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			roles, err := m.service.Roles(ctx)
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

func (m *modelConfigScreens) SetModelRole(kind models.RoleKind, argument string) error {
	role, err := parseModelRole(kind, argument)
	if err != nil {
		return err
	}
	m.status.note("updating " + string(kind) + " model role")
	started := m.tasks.admissionMutation(modelConfigOperation, false,
		func(ctx context.Context) (models.Role, error) { return m.service.SetRole(ctx, role) },
		func(updated models.Role, err error) {
			if err != nil {
				m.message("update " + string(kind) + " role failed: " + err.Error())
				return
			}
			label, labelErr := updated.Label()
			if labelErr != nil {
				m.message("update " + string(kind) + " role returned invalid state: " + labelErr.Error())
				return
			}
			m.message(string(kind) + " model · " + label)
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
	case kind == models.UtilityRole && strings.EqualFold(argument, utilityRoleDefaultArgument):
		return models.RuntimeDefaultUtilityRole(), nil
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
	utilityRoleDefaultArgument    = "default"
	embeddingRoleDisabledArgument = "off"
)

func modelRoleUsage(kind models.RoleKind) error {
	if kind == models.UtilityRole {
		return errors.New("usage: /utility <provider/model|default>")
	}
	return errors.New("usage: /embedding <provider/model|off>")
}
