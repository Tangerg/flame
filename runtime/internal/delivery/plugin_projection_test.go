package delivery

import (
	json "encoding/json/v2"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSecretInputProjectionCarriesStateNeverText(t *testing.T) {
	const credential = "credential-text-that-must-not-leave"
	release := testsupport.Release(t, "1", plugin.Declaration{
		Name: "package",
		Servers: []plugin.Server{
			{Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp"},
			{Name: testsupport.ServerName("process"), Transport: mcpserver.TransportStdio, Command: "fixture"},
		},
		Inputs: []plugin.Input{
			{ID: "secret", Server: testsupport.ServerName("server"), Target: plugin.Authorization, Secret: true},
			{ID: "header", Server: testsupport.ServerName("server"), Target: plugin.Header, Key: "X-Region", Secret: true},
			{ID: "region", Server: testsupport.ServerName("process"), Target: plugin.Environment, Key: "REGION"},
			{ID: "unset.secret", Server: testsupport.ServerName("server"), Target: plugin.Header, Key: "X-Secret", Secret: true},
			{ID: "unset.plain", Server: testsupport.ServerName("process"), Target: plugin.Environment, Key: "PLAIN"},
		},
	})
	for _, values := range []map[string]string{
		{"secret": credential, "header": credential, "region": "eu"},
		{"secret": "", "header": "", "region": ""},
	} {
		record := plugin.Record{ID: testsupport.InstallationID(t), State: plugin.Enabled, Values: values, Selected: release.Digest()}
		projected, err := presentInstallation(plugins.Inspection{Record: record, Selected: release, Realization: plugins.Realization{Release: plugins.ReleaseAvailable}, Presentation: plugins.PresentationAdmitted})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]protocol.PluginInputState{
			"secret":       {Type: protocol.PluginInputConfigured},
			"header":       {Type: protocol.PluginInputConfigured},
			"region":       {Type: protocol.PluginInputValue, Value: new(values["region"])},
			"unset.secret": {Type: protocol.PluginInputUnset},
			"unset.plain":  {Type: protocol.PluginInputUnset},
		}
		if !reflect.DeepEqual(projected.InputStates, want) {
			t.Fatalf("input states = %+v, want %+v", projected.InputStates, want)
		}
		if err := protocol.ValidateWireTree(*projected); err != nil {
			t.Fatalf("projected installation violates the wire contract: %v", err)
		}
		body, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), credential) {
			t.Fatalf("secret text reached the projection: %s", body)
		}
	}
}

func TestPluginRealizationProjectionIsAClosedUnion(t *testing.T) {
	backend, err := mcpserver.ParseServerName("backend")
	if err != nil {
		t.Fatal(err)
	}
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package"})
	record := plugin.Record{ID: testsupport.InstallationID(t), State: plugin.Unapproved, Selected: release.Digest()}
	for _, test := range []struct {
		realization plugins.Realization
		want        protocol.PluginRealization
	}{
		{plugins.Realization{Release: plugins.ReleaseAvailable}, protocol.PluginRealization{Type: protocol.PluginRealizationAvailable}},
		{plugins.Realization{Release: plugins.ReleaseAvailable, Sources: []mcpapp.Source{{Server: mcpserver.Server{Name: backend}, Availability: mcpapp.SourceUnavailableBackend}}}, protocol.PluginRealization{Type: protocol.PluginRealizationAvailable, UnavailableBackends: []string{"backend"}}},
		{plugins.Realization{Release: plugins.ReleaseUnavailable}, protocol.PluginRealization{Type: protocol.PluginRealizationReleaseUnavailable}},
	} {
		projected, err := presentInstallation(plugins.Inspection{Record: record, Selected: release, Realization: test.realization, Presentation: plugins.PresentationWithheld})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(projected.Realization, test.want) {
			t.Fatalf("realization = %+v, want %+v", projected.Realization, test.want)
		}
		if err := protocol.ValidateWireTree(projected.Realization); err != nil {
			t.Fatalf("realization violates the wire contract: %v", err)
		}
	}
	if _, err := presentInstallation(plugins.Inspection{Record: record, Selected: release, Presentation: plugins.PresentationWithheld}); err == nil {
		t.Fatal("an unobserved realization was projected")
	}
	if _, err := presentInstallation(plugins.Inspection{Record: record, Selected: release, Realization: plugins.Realization{Release: plugins.ReleaseAvailable}}); err == nil {
		t.Fatal("an undecided presentation was projected")
	}
}

func TestPluginDiagnosticsAreTypedEndToEnd(t *testing.T) {
	diagnostics := []plugin.Diagnostic{
		{Component: plugin.Component{Kind: plugin.ComponentManifestField, Name: "homepage-extra"}, Code: plugin.DiagnosticUnknownField},
		{Component: plugin.Component{Kind: plugin.ComponentFlameExtension}, Code: plugin.DiagnosticInvalidDeclaration},
		{Component: plugin.Component{Kind: plugin.ComponentExtensionField, Name: "inputs"}, Code: plugin.DiagnosticInvalidDependencies},
		{Component: plugin.Component{Kind: plugin.ComponentContribution, Name: "views"}, Code: plugin.DiagnosticUnsupportedContribution},
		{Component: plugin.Component{Kind: plugin.ComponentMCP}, Code: plugin.DiagnosticComponentLimit},
		{Component: plugin.Component{Kind: plugin.ComponentMCPServer, Name: ""}, Code: plugin.DiagnosticInvalidDeclaration},
		{Component: plugin.Component{Kind: plugin.ComponentSkills}, Code: plugin.DiagnosticUnavailableComponent},
		{Component: plugin.Component{Kind: plugin.ComponentSkill, Name: "review"}, Code: plugin.DiagnosticInvalidDeclaration},
	}
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "package", Diagnostics: diagnostics})
	if err != nil {
		t.Fatalf("typed diagnostics rejected: %v", err)
	}
	wire, err := presentPluginRelease(release)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.PluginRelease
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := protocol.ValidateWireTree(decoded); err != nil {
		t.Fatalf("diagnostics violate the wire contract: %s: %v", body, err)
	}
	if len(decoded.Diagnostics) != len(diagnostics) {
		t.Fatalf("diagnostics = %+v", decoded.Diagnostics)
	}
	for index, diagnostic := range diagnostics {
		got := decoded.Diagnostics[index]
		if string(got.Code) != string(diagnostic.Code) || string(got.Component.Type) != string(diagnostic.Component.Kind) {
			t.Fatalf("diagnostic %d = %+v, want %+v", index, got, diagnostic)
		}
		if named := got.Component.Name != nil; named {
			if *got.Component.Name != diagnostic.Component.Name {
				t.Fatalf("diagnostic %d name = %q, want %q", index, *got.Component.Name, diagnostic.Component.Name)
			}
		} else if diagnostic.Component.Name != "" {
			t.Fatalf("diagnostic %d lost its component name", index)
		}
	}
	for _, invalid := range []plugin.Diagnostic{
		{Component: plugin.Component{Kind: "release"}, Code: plugin.DiagnosticInvalidDeclaration},
		{Component: plugin.Component{Kind: plugin.ComponentMCP}, Code: "preparation_failed"},
		{Component: plugin.Component{Kind: plugin.ComponentMCP, Name: "server"}, Code: plugin.DiagnosticInvalidDeclaration},
	} {
		if _, err := plugin.NewRelease(release.Digest(), plugin.Declaration{Name: "package", Diagnostics: []plugin.Diagnostic{invalid}}); err == nil {
			t.Fatalf("diagnostic outside the closed model was admitted: %+v", invalid)
		}
	}
}
