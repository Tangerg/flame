package model

import (
	"context"
	"errors"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/models/catalog"
	"reflect"
	"testing"
)

func TestCatalogPricesPreserveInputDefaultsAndFreeCache(t *testing.T) {
	for _, test := range []struct {
		name                string
		read, write         *float64
		wantRead, wantWrite float64
	}{
		{name: "input default", wantRead: 3, wantWrite: 3},
		{name: "free cache", read: new(float64(0)), write: new(float64(0))},
		{name: "explicit rates", read: new(0.3), write: new(3.75), wantRead: 0.3, wantWrite: 3.75},
	} {
		t.Run(test.name, func(t *testing.T) {
			model, err := catalogModel("provider", catalog.Model{ID: "model", Pricing: catalog.PricingSchedule{{
				InputPer1M: 3, OutputPer1M: 15, CacheReadPer1M: test.read, CacheWritePer1M: test.write,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			pricing := model.Details().Pricing
			if pricing == nil || pricing.CacheReadPerMillion != test.wantRead || pricing.CacheWritePerMillion != test.wantWrite {
				t.Fatalf("catalog cache price changed meaning: %+v", pricing)
			}
		})
	}
}

type capabilityAdapters struct {
	profiles []modelprovider.Profile
	err      error
}

func (f capabilityAdapters) Profiles(context.Context) ([]modelprovider.Profile, error) {
	return f.profiles, f.err
}
func (f capabilityAdapters) RemoteModels(context.Context, modelprovider.Connection) ([]string, error) {
	return nil, f.err
}
func (f capabilityAdapters) OpenChat(context.Context, modelchoice.Selection, modelprovider.Connection) (ResolvedChat, error) {
	return ResolvedChat{}, f.err
}
func TestCapabilitiesAdmitOnlyInstalledProviderFacts(t *testing.T) {
	entry := catalog.Model{ID: "model", Reasoning: catalog.Reasoning{Supported: true, Levels: []string{"custom"}, DefaultLevel: "custom"}}
	adapters := capabilityAdapters{profiles: []modelprovider.Profile{{ID: "installed", Authentication: modelprovider.KeyOptional, ModelSource: modelprovider.BundledModels, DefaultModel: "model", Models: []catalog.Model{entry}}}}
	c, err := NewCapabilities(adapters, fakeEndpointJournal{})
	if err != nil {
		t.Fatal(err)
	}
	selection := testsupport.MustModelSelection("installed", "model")
	if err := c.AdmitSelection(t.Context(), selection); err != nil {
		t.Fatal(err)
	}
	profiles, err := c.Profiles(t.Context())
	if err != nil || len(profiles) != 1 || profiles[0].Metadata.ID() != "installed" || len(profiles[0].Models) != 1 {
		t.Fatalf("profiles=%+v err=%v", profiles, err)
	}
	if !reflect.DeepEqual(profiles[0].Models[0].Details().ReasoningLevels, []string{"custom"}) {
		t.Fatal("installed reasoning facts changed")
	}
	if err := c.AdmitSelection(t.Context(), testsupport.MustModelSelection("anthropic", "claude-opus-5")); !errors.Is(err, modelchoice.ErrUnsupported) {
		t.Fatalf("uninstalled provider=%v", err)
	}
}
func TestCapabilitiesPreserveUnavailableAdapterFailure(t *testing.T) {
	cause := errors.New("selected adapter unavailable")
	c, err := NewCapabilities(capabilityAdapters{err: cause}, fakeEndpointJournal{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AdmitSelection(t.Context(), testsupport.MustModelSelection("installed", "model")); !errors.Is(err, cause) || errors.Is(err, modelchoice.ErrUnsupported) {
		t.Fatalf("admission=%v", err)
	}
	if _, err := c.Profiles(t.Context()); !errors.Is(err, cause) {
		t.Fatalf("profiles=%v", err)
	}
}
