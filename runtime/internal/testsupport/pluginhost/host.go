// Package pluginhost constructs the real installation authority for tests that
// inspect Flame projections without executing a plugin.
package pluginhost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type Fixture struct {
	Host  *qimu.Host
	store *persistence.Bundle
}

func New(t testing.TB, store *persistence.Bundle) *Fixture {
	t.Helper()
	if store == nil || store.Qimu == nil || store.Admission == nil {
		t.Fatal("pluginhost: original framework store is required")
	}
	host, err := qimu.NewHost(context.Background(), qimu.HostConfig{
		Admission:           store.Admission,
		Epoch:               store.Qimu.Epoch(),
		Installations:       store.Qimu,
		ExecutionProtection: store.Qimu,
		Drivers:             []qimu.Driver{refusalDriver{qimu.MCPProcess}, refusalDriver{qimu.BrowserJS}, refusalDriver{qimu.StaticResource}},
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
	records, err := store.Qimu.Installations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var definitions []*qimu.Plugin
	for _, record := range records {
		flame, err := plugins.DecodeFrameworkConfiguration(record)
		if err != nil {
			t.Fatal(err)
		}
		release, err := store.PluginReleases.Get(context.Background(), flame.Selected)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := plugins.ProjectFrameworkRelease(release)
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
func (f *Fixture) Apply(ctx context.Context, installation *plugin.Installation) error {
	if installation == nil {
		return plugin.ErrInvalid
	}
	record := installation.Snapshot()
	release, err := f.store.PluginReleases.Get(ctx, record.Selected)
	if err != nil {
		return err
	}
	projection, err := plugins.ProjectFrameworkRelease(release)
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
	config := qimu.InstallationConfig{ID: qimu.InstallationID(record.ID.String()), Plugin: projection.Plugin, Configuration: configuration, Enabled: record.Enabled}
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

type refusalDriver struct{ profile qimu.Profile }

func (d refusalDriver) Profile() qimu.Profile { return d.profile }
func (d refusalDriver) RealmBound() bool      { return d.profile == qimu.BrowserJS }
func (refusalDriver) Prepare(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
	return nil, errors.New("pluginhost: fixture does not execute plugins")
}
