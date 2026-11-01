package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// modelConfigScreens inspects and configures model providers and roles.
type modelConfigScreens struct {
	screenHost
	service        ModelConfiguration
	providerDialog *kit.Dialog
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
	baseChoice := headless.NewSelect(headless.SelectConfig[formChange]{Same: headless.Equal[formChange], Label: "Endpoint change", Value: headless.Bind(&baseMode), Rows: 3})
	baseChoice.SetOptions([]headless.Option[formChange]{
		{Label: "Keep current endpoint", Value: formChangeKeep},
		{Label: "Set endpoint", Value: formChangeSet},
		{Label: "Clear endpoint", Value: formChangeClear},
	})
	endpointDraft := &providerEndpointDraft{value: baseURL, policy: provider.Endpoint()}
	baseField := headless.NewText(headless.TextConfig{
		Placeholder: endpointDraft.policy.Example, Value: endpointDraft,
		Check: func(value string) error {
			if !baseMode.SetsValue() {
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
	keyChoice := headless.NewSelect(headless.SelectConfig[formChange]{Same: headless.Equal[formChange], Label: "API key change", Value: headless.Bind(&keyMode), Rows: 3})
	keyOptions := []headless.Option[formChange]{
		{Label: "Keep current key", Value: formChangeKeep},
		{Label: "Set a stored key", Value: formChangeSet},
	}
	credential, hasCredential := provider.Credential()
	if hasCredential && credential.Stored() {
		keyOptions = append(keyOptions, headless.Option[formChange]{Label: "Clear stored key", Value: formChangeClear})
	}
	keyChoice.SetOptions(keyOptions)
	keyPlaceholder := ""
	if hasCredential {
		keyPlaceholder = credential.Masked()
	}
	keyField := headless.NewText(headless.TextConfig{
		Label: "New API key", Placeholder: keyPlaceholder, Value: headless.Bind(&apiKey),
		Check: func(value string) error {
			if keyMode.SetsValue() {
				return requiredText(value)
			}
			return nil
		},
	})
	keyField.SetMask("•")
	baseField.Clipboard = m.clipboard
	keyField.Clipboard = m.clipboard
	form := headless.NewForm(baseChoice, baseField, keyChoice, keyField)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	clearKey := func() {
		apiKey = ""
		keyField.SetText("")
	}
	dismiss := func() {
		clearKey()
		if m.providerDialog == dialog {
			dialog.Controller().Dismiss()
			m.providerDialog = nil
		}
	}
	form.Done = func() {
		if m.providerDialog != dialog {
			clearKey()
			return
		}
		update, err := providerUpdate(provider.ID(), baseMode, endpointDraft.Value(), keyMode, apiKey)
		if err != nil {
			m.message("provider form: " + err.Error())
			return
		}
		dismiss()
		if update.BaseURL == nil && update.APIKey == nil {
			m.message("provider configuration unchanged")
			return
		}
		m.updateProvider(update)
	}
	form.GaveUp = dismiss
	dressed := kit.NewForm(kit.FormConfig{
		Theme: m.theme, Glyphs: m.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: m.stack, Theme: m.theme, Glyphs: m.glyphs,
		Title: "Configure provider · " + provider.ID(), Body: dressed,
		Where: layout.Placement{Width: 82, Height: 18},
	})
	m.providerDialog = dialog
	dialog.Controller().Show()
}

type providerEndpointDraft struct {
	value  string
	policy flamemodel.EndpointPolicy
	field  *headless.Text
}

func (d *providerEndpointDraft) Value() string { return d.value }
func (d *providerEndpointDraft) Set(value string) {
	d.value = value
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
