package model

import (
	"context"
	"errors"
	"slices"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/scope/core/chat"
)

// ProviderCapture is the checkpoint projection of one admitted construction.
// The original open receipt owns its private credentials and reconstruction.
type ProviderCapture struct {
	Binding plugins.ContributionBinding `json:"binding"`
	Profile modelprovider.Profile       `json:"profile"`
	Chat    modelprovider.Chat          `json:"chat"`
}

func (c ProviderCapture) Validate() error {
	if err := errors.Join(c.Profile.Validate(), c.Chat.Validate()); err != nil {
		return err
	}
	if c.Profile.ID != c.Chat.Provider {
		return errors.New("model: captured provider belongs to another selection")
	}
	contract, err := modelprovider.Contract()
	if err != nil {
		return err
	}
	if c.Binding.Contract != contract.Identity() || c.Binding.Activation == nil {
		return errors.New("model: exact installed provider binding is required")
	}
	if _, err := plugins.ContributionSource(c.Binding); err != nil {
		return err
	}
	_, err = plugins.OriginalActivation([]plugin.Activation{*c.Binding.Activation}, c.Binding.InstallationID, c.Binding.Digest, c.Binding.ServerName)
	return err
}

func (c ProviderCapture) ValidateSelection(selection modelchoice.Selection) error {
	if err := selection.ValidateExact(); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Chat.Provider != selection.Provider() || c.Chat.Model != selection.Model() {
		return errors.New("model: captured construction belongs to another selection")
	}
	return nil
}

func (c ProviderCapture) Clone() ProviderCapture {
	c.Profile.Models = slices.Clone(c.Profile.Models)
	for index, model := range c.Profile.Models {
		c.Profile.Models[index] = model.Clone()
	}
	if c.Profile.Embedding != nil {
		copy := *c.Profile.Embedding
		c.Profile.Embedding = &copy
	}
	if c.Chat.Metadata != nil {
		copy := c.Chat.Metadata.Clone()
		c.Chat.Metadata = &copy
	}
	if c.Binding.Activation != nil {
		activation := c.Binding.Activation.Clone()
		c.Binding.Activation = &activation
	}
	return c
}

func NewCapturedChat(selection modelchoice.Selection, capture ProviderCapture, model chat.Model, counter InputTokenCounter, close func(context.Context) error, fits func(context.Context, *chat.Request) (bool, error)) (ResolvedChat, error) {
	if err := capture.ValidateSelection(selection); err != nil {
		return ResolvedChat{}, err
	}
	if close == nil || fits == nil || capture.Chat.TokenCounting != (counter != nil) {
		return ResolvedChat{}, errors.New("model: captured provider lifetime and exact capabilities are required")
	}
	_, streaming := model.(chat.Streamer)
	if capture.Chat.Streaming != streaming {
		return ResolvedChat{}, errors.New("model: captured provider streaming capability changed")
	}
	resolved, err := NewResolvedChat(model, counter, capture.Chat.Metadata)
	if err != nil {
		return ResolvedChat{}, err
	}
	owned := capture.Clone()
	resolved.capture = &owned
	resolved.close = close
	resolved.fits = fits
	resolved.pricing = Pricing(capture.Profile.ID, capture.Profile.Models)
	return resolved, nil
}
