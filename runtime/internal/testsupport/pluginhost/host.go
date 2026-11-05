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
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

type Fixture struct {
	Host  *frame.Host
	store *persistence.Bundle
}

func New(t testing.TB, store *persistence.Bundle) *Fixture {
	t.Helper()
	if store == nil || store.Frame == nil || store.Admission == nil {
		t.Fatal("pluginhost: original framework store is required")
	}
	host, err := frame.NewHost(context.Background(), frame.HostConfig{
		Admission:           store.Admission,
		Epoch:               store.Frame.Epoch(),
		Installations:       store.Frame,
		ExecutionProtection: store.Frame,
		Drivers:             []frame.Driver{refusalDriver{pluginmcp.ProcessProfile}, refusalDriver{frame.BrowserJS}, refusalDriver{frame.StaticResource}},
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
	var definitions []*frame.Plugin
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
	if err := host.Restore(context.Background(), records, definitions); err != nil {
		t.Fatal(err)
	}
	return &Fixture{Host: host, store: store}
}

// Apply proposes the fixture's product configuration through the real Host.
// The SQLite installation index advances only inside that Host transaction.
func (f *Fixture) Apply(ctx context.Context, installation plugin.Installation) error {
	record := installation.Snapshot()
	release, err := f.store.PluginReleases.Get(ctx, record.Selected)
	if err != nil {
		return err
	}
	projection, err := plugins.ProjectFrameworkRelease(release, pluginmcp.ProcessProfile)
	if err != nil {
		return err
	}
	if err := plugins.RegisterFrameworkRelease(f.Host, projection); err != nil {
		return err
	}
	configuration, err := plugins.EncodeFrameworkConfiguration(record)
	if err != nil {
		return err
	}
	config := frame.InstallationConfig{ID: record.ID.Host(), Plugin: projection.Plugin, Configuration: configuration, Enabled: record.Enabled}
	previous, exists := f.Host.Installation(config.ID)
	if exists {
		err = f.Host.Update(ctx, previous, config)
	} else {
		_, err = f.Host.Install(ctx, config)
	}
	if err != nil {
		return err
	}
	if record.Staged != nil {
		return f.store.Admission.Admit(ctx, func(admitted context.Context) error {
			return f.store.Installations.Stage(admitted, record.ID, *record.Staged)
		})
	}
	return nil
}

type refusalDriver struct{ profile frame.Profile }

func (d refusalDriver) Profile() frame.Profile { return d.profile }
func (d refusalDriver) RealmBound() bool       { return d.profile == frame.BrowserJS }
func (refusalDriver) Prepare(context.Context, *frame.Setup) ([]frame.ExportValue, error) {
	return nil, errors.New("pluginhost: fixture does not execute plugins")
}
