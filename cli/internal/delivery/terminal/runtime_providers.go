package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/flame/cli/internal/application/integration/models"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) ShowProviders() {
	if a.modelConfig == nil {
		a.message("this runtime composition has no model configuration service")
		return
	}
	a.executeRuntimeReaderQuery(a.providersReaderQuery())
}

func (a *app) providersReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading providers", mode: runtimeReaderProviders,
		read: func(ctx context.Context) (readerDocument, error) {
			providers, err := a.modelConfig.Providers(ctx)
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

func (a *app) TestConfiguredProvider(providerID string) error {
	if a.modelConfig == nil {
		return errors.New("this runtime composition has no model configuration service")
	}
	if err := runtimeprotocol.ValidateProviderIdentity(providerID); err != nil {
		return fmt.Errorf("provider test: %w", err)
	}
	a.status.note("testing provider " + providerID)
	started := a.runApplicationOperation(modelConfigOperation, false,
		func(ctx context.Context) (models.TestResult, error) {
			return a.modelConfig.TestProvider(ctx, providerID)
		},
		func(result models.TestResult, err error) {
			if err != nil {
				a.message("provider test failed: " + err.Error())
				return
			}
			if result.OK {
				a.message("provider " + providerID + " is reachable")
				return
			}
			a.message("provider " + providerID + " failed: " + result.Problem.String())
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func (a *app) ConfigureProvider(providerID string) error {
	if a.modelConfig == nil {
		return errors.New("this runtime composition has no model configuration service")
	}
	if err := runtimeprotocol.ValidateProviderIdentity(providerID); err != nil {
		return fmt.Errorf("provider configuration: %w", err)
	}
	presentation := a.session.context
	a.status.note("loading provider " + providerID)
	started := a.runApplicationOperation(modelConfigOperation, false,
		func(ctx context.Context) (models.Provider, error) {
			providers, err := a.modelConfig.Providers(ctx)
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
				a.message("configure provider failed: " + err.Error())
				return
			}
			if !a.session.context.current(presentation) {
				a.message("provider loaded after the active session changed; reopen configuration to continue")
				return
			}
			a.openProviderConfig(provider)
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func (a *app) openProviderConfig(provider models.Provider) {
	baseURL, endpointConfigured := provider.BaseURL()
	baseMode := formChangeKeep
	if provider.RequiresBaseURL() && !endpointConfigured {
		baseMode = formChangeSet
	}
	keyMode, apiKey := formChangeKeep, ""
	baseChoice := &headless.Select[formChange]{Label: "Endpoint change", Value: headless.Bind(&baseMode), Rows: 3}
	baseChoice.SetOptions([]headless.Option[formChange]{
		{Label: "Keep current endpoint", Value: formChangeKeep},
		{Label: "Set endpoint", Value: formChangeSet},
		{Label: "Clear endpoint", Value: formChangeClear},
	})
	baseField := &headless.Text{
		Label: "Endpoint URL", Placeholder: "https://api.example.com", Value: headless.Bind(&baseURL),
		Check: func(value string) error {
			if baseMode.SetsValue() {
				return requiredText(value)
			}
			return nil
		},
	}
	keyChoice := &headless.Select[formChange]{Label: "API key change", Value: headless.Bind(&keyMode), Rows: 3}
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
	keyField := &headless.Text{
		Label: "New API key", Placeholder: keyPlaceholder, Value: headless.Bind(&apiKey),
		Check: func(value string) error {
			if keyMode.SetsValue() {
				return requiredText(value)
			}
			return nil
		},
	}
	keyField.Editor().SetMask("•")
	baseField.Editor().Clipboard = a.loop.Clipboard()
	keyField.Editor().Clipboard = a.loop.Clipboard()
	form := headless.NewForm(baseChoice, baseField, keyChoice, keyField)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	clearKey := func() {
		apiKey = ""
		keyField.Editor().SetText("")
	}
	dismiss := func() {
		clearKey()
		if a.dialogs.providerDialog == dialog {
			dialog.Controller().Dismiss()
			a.dialogs.providerDialog = nil
		}
	}
	form.Done = func() {
		if a.dialogs.providerDialog != dialog {
			clearKey()
			return
		}
		update, err := providerUpdate(provider.ID(), baseMode, baseURL, keyMode, apiKey)
		if err != nil {
			a.message("provider form: " + err.Error())
			return
		}
		dismiss()
		if update.BaseURL == nil && update.APIKey == nil {
			a.message("provider configuration unchanged")
			return
		}
		a.updateProvider(update)
	}
	form.GaveUp = dismiss
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Configure provider · " + provider.ID(), Body: dressed,
		Where: layout.Placement{Width: 82, Height: 18},
	})
	a.dialogs.providerDialog = dialog
	dialog.Controller().Show()
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
		change := &models.ValueChange{Kind: runtimeprotocol.ProviderConfigSet, Value: value}
		return change, change.Validate()
	case formChangeClear:
		change := &models.ValueChange{Kind: runtimeprotocol.ProviderConfigClear}
		return change, change.Validate()
	case formChangeKeep:
		return nil, nil
	}
	panic("validated form change became unreachable")
}

func (a *app) updateProvider(update models.UpdateProvider) {
	a.status.note("updating provider " + update.Provider)
	started := a.runAdmissionMutation(modelConfigOperation, false,
		func(ctx context.Context) (models.Provider, error) {
			return a.modelConfig.UpdateProvider(ctx, update)
		},
		func(provider models.Provider, err error) {
			if err != nil {
				a.message("update provider failed: " + err.Error())
				return
			}
			a.message("provider updated · " + provider.ID())
		},
	)
	if !started {
		a.message("another model configuration operation is running")
	}
}
