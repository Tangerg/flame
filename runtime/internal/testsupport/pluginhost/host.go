// Package pluginhost constructs the real installation authority for tests that
// inspect Flame projections without executing a plugin.
package pluginhost

import (
	"context"
	"errors"
	"testing"
	"time"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"

	framehost "github.com/Tangerg/frame/host"
)

type Fixture struct {
	Host  framehost.Host
	store *persistence.Bundle
}

func New(t testing.TB, store *persistence.Bundle) *Fixture {
	t.Helper()
	if store == nil || !store.Frame.Valid() || !store.Admission.Valid() {
		t.Fatal("pluginhost: original framework store is required")
	}
	host, err := framehost.New(context.Background(), framehost.Config{
		Authority: store.Frame,
		Drivers:   []framehost.Driver{refusalDriver{pluginmcp.ProcessProfile}, refusalDriver{framehost.BrowserJS}, refusalDriver{framehost.StaticResource}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	records, err := store.Frame.Installations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var definitions []*framehost.Plugin
	for _, record := range records {
		flame, err := plugins.DecodeFrameworkConfiguration(record)
		if err != nil {
			t.Fatal(err)
		}
		release, err := store.PluginReleases.Get(context.Background(), flame.Selected)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := plugins.ProjectFrameworkRelease(release, pluginmcp.ProcessProfile)
		if err != nil {
			t.Fatal(err)
		}
		if err := plugins.RegisterFrameworkRelease(host, projection); err != nil {
			t.Fatal(err)
		}
		definitions = append(definitions, projection.Plugin)
	}
	if err := host.Restore(context.Background(), definitions); err != nil {
		t.Fatal(err)
	}
	return &Fixture{Host: host, store: store}
}

type refusalDriver struct{ profile framehost.Profile }

func (d refusalDriver) Profile() framehost.Profile { return d.profile }
func (d refusalDriver) RealmBound() bool           { return d.profile == framehost.BrowserJS }
func (refusalDriver) Prepare(context.Context, framehost.Setup) ([]framehost.ExportValue, error) {
	return nil, errors.New("pluginhost: fixture does not execute plugins")
}
