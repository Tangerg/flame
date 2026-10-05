package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
)

type pluginCommandFixture struct {
	Runtime
	pluginRuntime
	calls    int
	received protocol.InstallPluginRequest
	command  replay.CommandID
}

func (f *pluginCommandFixture) InstallPlugin(_ context.Context, in protocol.InstallPluginRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
	f.calls++
	f.received = in
	f.command = id
	return &protocol.PluginInstallation{ID: "installed", Source: in.Source}, nil
}
func (f *pluginCommandFixture) SetPluginEnablement(_ context.Context, in protocol.SetPluginEnablementRequest, _ replay.CommandID) (*protocol.PluginInstallation, error) {
	f.calls++
	state := protocol.PluginInstallationApproved
	if in.Enabled {
		state = protocol.PluginInstallationEnabled
	}
	return &protocol.PluginInstallation{ID: in.InstallationID, State: state}, nil
}
func (f *pluginCommandFixture) ApprovePlugin(_ context.Context, in protocol.PluginReleaseRequest, _ replay.CommandID) (*protocol.PluginInstallation, error) {
	f.calls++
	return &protocol.PluginInstallation{ID: in.InstallationID, State: protocol.PluginInstallationApproved}, nil
}
func (f *pluginCommandFixture) ConfigurePlugin(_ context.Context, in protocol.ConfigurePluginRequest, _ replay.CommandID) (*protocol.PluginInstallation, error) {
	f.calls++
	var disabledServers, disabledSkills []string
	for name, change := range in.ServerChanges {
		if change == protocol.PluginComponentDisable {
			disabledServers = append(disabledServers, name)
		}
	}
	for name, change := range in.SkillChanges {
		if change == protocol.PluginComponentDisable {
			disabledSkills = append(disabledSkills, name)
		}
	}
	return &protocol.PluginInstallation{ID: in.InstallationID, DisabledServers: disabledServers, DisabledSkills: disabledSkills}, nil
}

func TestPluginCommandsPreserveAuthoredChangePresence(t *testing.T) {
	const id = "12345678-1234-1234-1234-123456789abc"
	const digest = "1111111111111111111111111111111111111111111111111111111111111111"
	for _, test := range []struct {
		name, command, request string
	}{
		{"request", "set-enablement", `null`},
		{"enablement", "set-enablement", `{"installationId":"` + id + `","enabled":null}`},
		{"missing enablement", "set-enablement", `{"installationId":"` + id + `"}`},
		{"grants", "approve", `{"installationId":"` + id + `","digest":"` + digest + `","grants":[]}`},
		{"missing digest", "approve", `{"installationId":"` + id + `"}`},
		{"inputs", "configure", `{"installationId":"` + id + `","digest":"` + digest + `","valueChanges":null,"serverChanges":{},"skillChanges":{}}`},
		{"server changes", "configure", `{"installationId":"` + id + `","digest":"` + digest + `","valueChanges":{},"serverChanges":null,"skillChanges":{}}`},
		{"wholesale disablement", "configure", `{"installationId":"` + id + `","digest":"` + digest + `","valueChanges":{},"disabledServers":[],"disabledSkills":[]}`},
		{"clear", "configure", `{"installationId":"` + id + `","digest":"` + digest + `","valueChanges":{"credential":{"type":"clear","value":null}},"serverChanges":{},"skillChanges":{}}`},
		{"set without value", "configure", `{"installationId":"` + id + `","digest":"` + digest + `","valueChanges":{"credential":{"type":"set"}},"serverChanges":{},"skillChanges":{}}`},
		{"clear with value", "configure", `{"installationId":"` + id + `","digest":"` + digest + `","valueChanges":{"credential":{"type":"clear","value":"replacement"}},"serverChanges":{},"skillChanges":{}}`},
		{"unknown change", "configure", `{"installationId":"` + id + `","digest":"` + digest + `","valueChanges":{"credential":{"type":"replace","value":"replacement"}},"serverChanges":{},"skillChanges":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := &pluginCommandFixture{Runtime: runtimefixture.New()}
			_, stderr, err := executeCommand(t, fixture, "", "plugins", test.command, "--request", test.request)
			if err == nil || fixture.calls != 0 || strings.Contains(stderr, "command-id:") {
				t.Fatalf("invalid reviewed JSON reached mutation: calls=%d, stderr=%q, error=%v", fixture.calls, stderr, err)
			}
		})
	}
}

func TestPluginCommandPreservesExplicitFalseEnablement(t *testing.T) {
	fixture := &pluginCommandFixture{Runtime: runtimefixture.New()}
	out, _, err := executeCommand(t, fixture, "", "plugins", "set-enablement", "--request", `{"installationId":"12345678-1234-1234-1234-123456789abc","enabled":false}`)
	if err != nil || fixture.calls != 1 || !strings.Contains(out, `"state":"approved"`) {
		t.Fatalf("explicit disablement = %s, calls=%d, error=%v", out, fixture.calls, err)
	}
}

func TestPluginConfigureSendsComponentDeltas(t *testing.T) {
	fixture := &pluginCommandFixture{Runtime: runtimefixture.New()}
	request := `{"installationId":"12345678-1234-1234-1234-123456789abc","digest":"` + strings.Repeat("1", 64) + `","valueChanges":{},"serverChanges":{"backend":"disable"},"skillChanges":{"review":"enable"}}`
	out, _, err := executeCommand(t, fixture, "", "plugins", "configure", "--request", request)
	if err != nil || fixture.calls != 1 || !strings.Contains(out, `"disabledServers":["backend"]`) {
		t.Fatalf("component delta = %s, calls=%d, error=%v", out, fixture.calls, err)
	}
}

func TestPluginCommandRejectsInvalidInputBeforeOpeningRuntime(t *testing.T) {
	for _, args := range [][]string{
		{"plugins", "install", "--request", `{"source":null}`},
		{"plugins", "install", "--request", `{"source":"/package"}`, "--command-id", "invalid"},
	} {
		root := NewRoot(Dependencies{OpenRuntime: func(context.Context, string) (Runtime, RuntimeProfile, error) {
			t.Fatal("invalid command opened Runtime")
			return nil, nil, nil
		}})
		root.SetArgs(args)
		if err := root.ExecuteContext(t.Context()); err == nil {
			t.Fatal("invalid command input was accepted")
		}
	}
}
func TestPluginCommandPreservesReviewedRequestAndReplayIdentity(t *testing.T) {
	fixture := &pluginCommandFixture{Runtime: runtimefixture.New()}
	key := "cli_" + strings.Repeat("a", 32)
	body := `{"source":"/reviewed-package"}`
	out, stderr, err := executeCommand(t, fixture, "", "plugins", "install", "--command-id", key, "--request", body)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.calls != 1 || string(fixture.command) != key || !strings.Contains(out, `"installed"`) || !strings.Contains(stderr, key) {
		t.Fatalf("command delivery: %s %s %+v", out, stderr, fixture)
	}
	if fixture.received.Source != "/reviewed-package" {
		t.Fatal("reviewed source changed")
	}
	_, _, err = executeCommand(t, fixture, "", "plugins", "install", "--command-id", key, "--request", strings.TrimSuffix(body, "}")+`,"unknown":true}`)
	if err == nil || fixture.calls != 1 {
		t.Fatal("unknown command member reached Runtime")
	}
}

func (f *pluginCommandFixture) UninstallPlugin(_ context.Context, _ protocol.PluginRequest, id replay.CommandID) error {
	f.calls++
	f.command = id
	return nil
}

func TestPluginUninstallIsAnAcknowledgementWithoutResult(t *testing.T) {
	fixture := &pluginCommandFixture{Runtime: runtimefixture.New()}
	out, stderr, err := executeCommand(t, fixture, "", "plugins", "uninstall", "--request", `{"installationId":"12345678-1234-1234-1234-123456789abc"}`)
	if err != nil || fixture.calls != 1 || out != "" || !strings.Contains(stderr, string(fixture.command)) {
		t.Fatalf("uninstall = %q %q, calls=%d, error=%v", out, stderr, fixture.calls, err)
	}
}
