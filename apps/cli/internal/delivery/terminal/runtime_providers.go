package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// modelConfigScreens inspects and configures model providers and roles.
type modelConfigScreens struct {
	screenHost
	service        ModelConfiguration
	providerDialog *presentationDialog
}

func (m *modelConfigScreens) ShowProviders() {
	m.reader.show(m.providersReaderQuery())
}

func (m *modelConfigScreens) providersReaderQuery() *readerQuery {
	return &readerQuery{
		status: "loading providers", follows: followsTopic(protocol.TopicModelsChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			providers, err := m.service.Providers(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return providersDocument(providers), nil
		},
	}
}

func providersDocument(providers []models.Provider) readerDocument {
	lines := make([]string, 0, len(providers))
	for _, provider := range providers {
		status := "not configured"
		if provider.Configured() {
			status = "ready"
		}
		if credential, configured := provider.Credential(); configured {
			status = credential.Masked()
			if credential.FromEnvironment() {
				status += " · from env"
			}
		}
		capabilities := ""
		if provider.EmbeddingCapable() {
			capabilities = " · embeddings"
		}
		endpoint := ""
		if baseURL, configured := provider.BaseURL(); configured {
			endpoint = " · " + baseURL
		} else if provider.RequiresBaseURL() {
			endpoint = " · endpoint required"
		}
		lines = append(lines, provider.ID()+"  "+status+endpoint+capabilities)
	}
	return paragraphDocument("Providers", fmt.Sprintf("%d available", len(providers)), lines)
}

func (m *modelConfigScreens) TestConfiguredProvider(providerID string) error {
	if err := model.ValidateProviderIdentity(providerID); err != nil {
		return fmt.Errorf("provider test: %w", err)
	}
	m.status.note("testing provider " + providerID)
	started := m.tasks.application(modelConfigOperation, false,
		func(ctx context.Context) (protocol.ProviderTestOutcome, error) {
			return m.service.TestProvider(ctx, providerID)
		},
		func(outcome protocol.ProviderTestOutcome, err error) {
			if err == nil {
				var message string
				message, err = providerProbeMessage(providerID, outcome)
				if err == nil {
					m.message(message)
					return
				}
			}
			m.message("provider test failed: " + err.Error())
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func (m *modelConfigScreens) ConfigureProvider(providerID string) error {
	if err := model.ValidateProviderIdentity(providerID); err != nil {
		return fmt.Errorf("provider configuration: %w", err)
	}
	presentation := m.session.context
	m.status.note("loading provider " + providerID)
	started := m.tasks.application(modelConfigOperation, false,
		func(ctx context.Context) (models.Provider, error) {
			providers, err := m.service.Providers(ctx)
			if err != nil {
				return models.Provider{}, err
			}
			for _, provider := range providers {
				if provider.ID() == providerID {
					return provider, nil
				}
			}
			return models.Provider{}, errors.New("provider not found: " + providerID)
		},
		func(provider models.Provider, err error) {
			if err != nil {
				m.message("configure provider failed: " + err.Error())
				return
			}
			if !m.session.context.current(presentation) {
				m.message("provider loaded after the active session changed; reopen configuration to continue")
				return
			}
			m.openProviderConfig(provider)
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func (m *modelConfigScreens) openProviderConfig(provider models.Provider) {
	baseURL, endpointConfigured := provider.BaseURL()
	baseMode := formChangeKeep
	if provider.RequiresBaseURL() && !endpointConfigured {
		baseMode = formChangeSet
	}
	keyMode, apiKey := formChangeKeep, ""
	baseChoice := base.NewSelect(base.SelectConfig[formChange]{Same: base.Equal[formChange], Label: "Endpoint change", Initial: baseMode, Rows: 3})
	baseChoice.SetOptions([]base.Option[formChange]{
		{Label: "Keep current endpoint", Value: formChangeKeep},
		{Label: "Set endpoint", Value: formChangeSet},
		{Label: "Clear endpoint", Value: formChangeClear},
	})
	baseChoice.Choose(baseMode)
	endpointDraft := &providerEndpointDraft{policy: provider.Endpoint()}
	baseField := base.NewText(base.TextConfig{
		Placeholder: endpointDraft.policy.Example, Initial: baseURL,
		Check: func(value string) error {
			if selected, ok := baseChoice.Chosen(); !ok || !selected.Value.SetsValue() {
				return nil
			}
			if err := requiredText(value); err != nil {
				return err
			}
			return endpointDraft.policy.ValidateOverride(strings.TrimSpace(value))
		},
	})
	endpointDraft.field = baseField
	endpointDraft.Set(baseURL)
	keyChoice := base.NewSelect(base.SelectConfig[formChange]{Same: base.Equal[formChange], Label: "API key change", Initial: keyMode, Rows: 3})
	keyOptions := []base.Option[formChange]{
		{Label: "Keep current key", Value: formChangeKeep},
		{Label: "Set a stored key", Value: formChangeSet},
	}
	credential, hasCredential := provider.Credential()
	if hasCredential && credential.Stored() {
		keyOptions = append(keyOptions, base.Option[formChange]{Label: "Clear stored key", Value: formChangeClear})
	}
	keyChoice.SetOptions(keyOptions)
	keyChoice.Choose(keyMode)
	keyPlaceholder := ""
	if hasCredential {
		keyPlaceholder = credential.Masked()
	}
	keyField := base.NewText(base.TextConfig{
		Label: "New API key", Placeholder: keyPlaceholder, Initial: apiKey,
		Check: func(value string) error {
			if selected, ok := keyChoice.Chosen(); ok && selected.Value.SetsValue() {
				return requiredText(value)
			}
			return nil
		},
	})
	keyField.SetMask("•")
	baseField.Clipboard = m.clipboard
	keyField.Clipboard = m.clipboard
	form := kit.FormConfig{Fields: []base.Field{baseChoice, baseField, keyChoice, keyField}, Theme: m.theme, Glyphs: m.glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	clearKey := func() {
		apiKey = ""
		keyField.SetText("")
	}
	dismiss := func() {
		clearKey()
		if m.providerDialog == dialog {
			dialog.Dismiss()
			m.providerDialog = nil
		}
	}
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if m.providerDialog != dialog {
			clearKey()
			return scene.Unchanged, nil
		}
		baseOption, baseOK := baseChoice.Chosen()
		keyOption, keyOK := keyChoice.Chosen()
		if !baseOK || !keyOK {
			return scene.Unchanged, errors.New("provider configuration choice is required")
		}
		update, err := providerUpdate(provider.ID(), baseOption.Value, baseField.Text(), keyOption.Value, keyField.Text())
		if err != nil {
			m.message("provider form: " + err.Error())
			return scene.Unchanged, nil
		}
		dismiss()
		if update.BaseURL == nil && update.APIKey == nil {
			m.message("provider configuration unchanged")
			return scene.Unchanged, nil
		}
		m.updateProvider(update)
		return scene.Applied, nil
	}

	dressed := kit.NewForm(form)
	dialog = newPresentationDialog(m.stack, kit.DialogConfig{Theme: m.theme, Glyphs: m.glyphs,
		Title: "Configure provider · " + provider.ID(), Body: formNavigation(dressed.Element(), dismiss),
		Where: layout.Placement{Width: 82, Height: 18},
	})
	m.providerDialog = dialog
	dialog.Show()
}

type providerEndpointDraft struct {
	policy flamemodel.EndpointPolicy
	field  *base.Text
}

func (d *providerEndpointDraft) Value() string { return d.field.Text() }
func (d *providerEndpointDraft) Set(value string) {
	d.field.SetText(value)
	label := "Endpoint URL"
	switch {
	case d.policy.AppendsPath != "":
		label += " (SDK appends " + d.policy.AppendsPath + ")"
		if effective := d.policy.EffectivePath(strings.TrimSpace(value)); effective != "" {
			label += " · " + effective
		}
	case d.policy.RequiredSuffix != "":
		label += " (must end with " + d.policy.RequiredSuffix + ")"
	case d.policy.Example != "":
		label += " (complete API base)"
	}
	d.field.Label = label
}

func providerUpdate(providerID string, baseMode formChange, baseURL string, keyMode formChange, apiKey string) (models.UpdateProvider, error) {
	update := models.UpdateProvider{Provider: providerID}
	baseChange, err := valueChange(baseMode, strings.TrimSpace(baseURL))
	if err != nil {
		return models.UpdateProvider{}, err
	}
	keyChange, err := valueChange(keyMode, apiKey)
	if err != nil {
		return models.UpdateProvider{}, err
	}
	update.BaseURL, update.APIKey = baseChange, keyChange
	return update, nil
}

func valueChange(mode formChange, value string) (*models.ValueChange, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	switch mode {
	case formChangeSet:
		change := &models.ValueChange{Kind: protocol.ProviderConfigSet, Value: value}
		return change, change.Validate()
	case formChangeClear:
		change := &models.ValueChange{Kind: protocol.ProviderConfigClear}
		return change, change.Validate()
	case formChangeKeep:
		return nil, nil
	}
	panic("validated form change became unreachable")
}

func (m *modelConfigScreens) updateProvider(update models.UpdateProvider) {
	m.status.note("updating provider " + update.Provider)
	started := m.tasks.admissionMutation(modelConfigOperation, false,
		func(ctx context.Context) (models.Provider, error) {
			return m.service.UpdateProvider(ctx, update)
		},
		func(provider models.Provider, err error) {
			if err != nil {
				m.message("update provider failed: " + err.Error())
				return
			}
			m.message("provider updated · " + provider.ID())
		},
	)
	if !started {
		m.message("another model configuration operation is running")
	}
}
