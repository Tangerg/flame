package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

func TestRuntimeInfoWritesCompleteHumanAndMachineProfiles(t *testing.T) {
	t.Parallel()

	profile := commandRuntimeProfile(t)
	for _, test := range []struct {
		name  string
		args  []string
		check func(*testing.T, string)
	}{
		{
			name: "human", args: []string{"runtime", "info"},
			check: func(t *testing.T, output string) {
				for _, want := range []string{
					"flame-runtime 1.2.3", "protocol", "/workspace", "segment.started", "files.changed",
					"feature mcp", "client opt-in requested", "available", "1024 events", "600 seconds", "32 watches",
				} {
					if !strings.Contains(output, want) {
						t.Fatalf("runtime info omitted %q:\n%s", want, output)
					}
				}
			},
		},
		{
			name: "JSON", args: []string{"runtime", "info", "--json"},
			check: func(t *testing.T, output string) {
				var decoded struct {
					Discovery          protocol.DiscoverResponse    `json:"discovery"`
					ClientCapabilities *protocol.ClientCapabilities `json:"clientCapabilities"`
				}
				if err := json.Unmarshal([]byte(output), &decoded); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(decoded.Discovery, profile.Discovery()) ||
					!reflect.DeepEqual(decoded.ClientCapabilities, profile.ClientCapabilities()) {
					t.Fatalf("runtime info changed negotiated protocol values: %+v", decoded)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			root := NewRoot(Dependencies{OpenRuntime: func(context.Context) (Runtime, *runtimebinding.Profile, error) {
				return runtimefixture.New(), new(profile), nil
			}})
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(test.args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			test.check(t, output.String())
		})
	}
}

func commandRuntimeProfile(t *testing.T, edits ...func(*protocol.DiscoverResponse, *protocol.ClientCapabilities)) runtimebinding.Profile {
	t.Helper()
	discovery := runtimefixture.Discovery()
	discovery.Capabilities.Features[protocol.FeatureMCP] = protocol.FeatureCapability{
		Enabled: true, ClientOptIn: true, RequiredByRunProtocol: true,
	}
	client := &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{
		protocol.FeatureMCP: {Enabled: true},
	}}
	for _, edit := range edits {
		edit(discovery, client)
	}
	profile, err := runtimebinding.NewProfile(discovery, client)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
