package terminal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/core/input"
)

type installedPluginCatalog struct {
	mu           sync.Mutex
	installation *protocol.PluginInstallation
	err          error
}

func (c *installedPluginCatalog) ListPlugins(context.Context) ([]protocol.PluginInstallation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	if c.installation == nil {
		return nil, nil
	}
	return []protocol.PluginInstallation{*c.installation}, nil
}

func TestInstalledPluginsAndLocalExtensionsHaveSeparateCatalogs(t *testing.T) {
	catalog := &installedPluginCatalog{installation: &protocol.PluginInstallation{
		ID: "12345678-1234-1234-1234-123456789abc", Source: "/runtime/packages/notification",
		Selected: protocol.PluginRelease{Name: "installed.notification", Version: "1.0.0", Digest: strings.Repeat("a", 64)},
		Enabled:  false, Presentation: protocol.PluginPresentationWithheld,
		Realization: protocol.PluginRealization{Type: protocol.PluginRealizationAvailable},
	}}
	local := extensions.Plugin{ID: "local.commands", Version: "1.0.0", APIVersion: extensions.HostAPIVersion, Setup: func(*extensions.Scope) error { return nil }}
	host, stop := runUIWithRuntimeServices(t, Config{
		RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench,
		Runtime: runtimefixture.New(), Plugins: catalog,
		PluginSources: []extensions.Source{testPluginSource{Plugins: []extensions.Plugin{local}}},
	})
	host.Shows(t, "Ask flame")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Shows(t, "Installed plugins")
	host.Shows(t, "installed.notification@1.0.0")
	host.Shows(t, "12345678-1234-1234-1234-123456789abc")
	host.Shows(t, strings.Repeat("a", 64))
	host.Shows(t, "enabled       false")
	host.Shows(t, "presentation  withheld")
	host.Hides(t, "local.commands")
	host.Press(input.Esc)
	host.Hides(t, "Installed plugins")
	host.Type("/extensions")
	host.Press(input.Enter)
	host.Shows(t, "local.commands@1.0.0")
	host.Hides(t, "installed.notification")
	stop()
}

func TestInstalledPluginReaderRefreshesFromRuntimeFacts(t *testing.T) {
	for _, test := range []struct {
		name     string
		sequence uint64
	}{
		{name: "plugins.changed", sequence: 2},
		{name: "resync after a sequence gap", sequence: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &installedPluginCatalog{}
			source := runtimeResourceChangeSource(protocol.TopicPluginsChanged)
			host, stop := runUIWithRuntimeServices(t, Config{
				RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench,
				Runtime: runtimefixture.New(), Plugins: catalog, Changes: source,
			})
			host.Shows(t, "Ask flame")
			assertSingleRuntimeTopic(t, source.subscription, protocol.TopicPluginsChanged)
			source.events <- changefeed.Event{Type: protocol.RuntimePluginsChanged, Sequence: 1}
			awaitSignal(t, source.applied, "initial installed plugin change")
			host.Type("/plugins")
			host.Press(input.Enter)
			host.Shows(t, "No Runtime plugins installed")
			catalog.mu.Lock()
			catalog.installation = &protocol.PluginInstallation{
				ID:       "12345678-1234-1234-1234-123456789abc",
				Selected: protocol.PluginRelease{Name: "externally.installed", Version: "2.0.0", Digest: strings.Repeat("b", 64)},
				Enabled:  true, Presentation: protocol.PluginPresentationAdmitted,
				Realization: protocol.PluginRealization{Type: protocol.PluginRealizationAvailable},
			}
			catalog.mu.Unlock()
			event := changefeed.Event{Type: protocol.RuntimePluginsChanged, Sequence: test.sequence}
			source.events <- event
			awaitSignal(t, source.applied, "installed plugin change")
			host.Shows(t, "externally.installed@2.0.0")
			host.Hides(t, "No Runtime plugins installed")

			catalog.mu.Lock()
			catalog.err = errors.New("installation catalog unavailable")
			catalog.mu.Unlock()
			event.Sequence++
			source.events <- event
			awaitSignal(t, source.applied, "installed plugin read failure")
			host.Shows(t, "installation catalog unavailable")
			host.Shows(t, "externally.installed@2.0.0")
			catalog.mu.Lock()
			catalog.err = nil
			catalog.installation = nil
			catalog.mu.Unlock()
			event.Sequence++
			source.events <- event
			awaitSignal(t, source.applied, "installed plugin read recovery")
			host.Shows(t, "No Runtime plugins installed")
			host.Hides(t, "installation catalog unavailable")
			stop()
		})
	}
}
