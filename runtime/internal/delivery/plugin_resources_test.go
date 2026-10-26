package delivery

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type resourceCallerKey struct{}

type resourcePluginOwner struct {
	pluginUseCases
	disabled bool
	reads    int
}

func (*resourcePluginOwner) Component(context.Context, identity.InstallationID, identity.Digest, plugin.ContributionID) (plugin.ClientComponent, error) {
	return plugin.ClientComponent{Entry: "io.github.tangerg.flame/components/component.js"}, nil
}
func (p *resourcePluginOwner) Resource(ctx context.Context, _ identity.InstallationID, _ identity.Digest, _ plugin.ContributionID, path string) ([]byte, error) {
	p.reads++
	if p.disabled {
		return nil, plugin.ErrDisabled
	}
	if ctx.Value(resourceCallerKey{}) != "caller" {
		return nil, errors.New("original caller context lost")
	}
	if path != "io.github.tangerg.flame/components/component.js" {
		return nil, plugin.ErrNotFound
	}
	return []byte("export const Board = () => null;"), nil
}

func TestResourceLeaseRetainsCallerAndRefusesWithdrawalAndCancellation(t *testing.T) {
	owner, cancel := context.WithCancel(context.WithValue(t.Context(), resourceCallerKey{}, "caller"))
	defer cancel()
	plugins := &resourcePluginOwner{}
	handler := &Handler{plugins: plugins, resources: newResourceLeases(), features: featureAvailability{plugins: true}}
	endpoint := mustNewEndpoint(t, handler, EndpointConfig{})
	result := endpoint.Invoke(owner, pluginsOpenResources, protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("resources").String()}, ComponentID: "board"}, Options{})
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	lease, ok := result.ValueAs[*protocol.PluginResourceLease]()
	if !ok {
		t.Fatal("resource lease response omitted")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range result.Events {
		}
	}()
	value, err := endpoint.LeasedPluginResource(t.Context(), lease.Token, lease.Entry)
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(value.Body)
	if err != nil || string(body) != "export const Board = () => null;" {
		t.Fatalf("resource body=%q,%v", body, err)
	}
	plugins.disabled = true
	if _, err := endpoint.LeasedPluginResource(t.Context(), lease.Token, lease.Entry); !errors.Is(err, protocol.ErrPluginDisabled) {
		t.Fatalf("withdrawn source=%v", err)
	}
	reads := plugins.reads
	cancel()
	<-done
	if _, err := endpoint.LeasedPluginResource(t.Context(), lease.Token, lease.Entry); !errors.Is(err, protocol.ErrPluginUnavailable) || plugins.reads != reads {
		t.Fatalf("canceled lease reached resource owner: %v", err)
	}
}
