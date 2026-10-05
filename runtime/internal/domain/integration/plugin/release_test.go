package plugin

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

const (
	stdio = mcpserver.TransportStdio
	http  = mcpserver.TransportStreamableHTTP
)

func TestReleaseIsAdmittedOnceAndImmutable(t *testing.T) {
	declaration := Declaration{Name: "review", Servers: []Server{{Name: serverName("reviews"), Transport: stdio, Command: "review", Env: map[string]string{"MODE": "strict"}}}}
	release, err := NewRelease(testDigest("1"), declaration)
	if err != nil {
		t.Fatal(err)
	}
	declaration.Servers[0].Command = "modified"
	declaration.Servers[0].Env["MODE"] = "lenient"
	projection := release.Declaration()
	projection.Servers[0].Args = append(projection.Servers[0].Args, "--unsafe")
	if current := release.Declaration().Servers[0]; current.Command != "review" || current.Env["MODE"] != "strict" || len(current.Args) != 0 {
		t.Fatalf("a caller advanced an admitted release: %+v", current)
	}
	if _, err := NewRelease(fingerprint.Digest{}, Declaration{Name: "review"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("release without a digest = %v", err)
	}
	if _, err := NewRelease(testDigest("1"), Declaration{Name: "Review"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("release with an invalid package name = %v", err)
	}
	if _, err := NewRelease(testDigest("1"), Declaration{Name: "review", Diagnostics: []Diagnostic{{Component: Component{Kind: ComponentMCP, Name: "named"}, Code: DiagnosticInvalidDeclaration}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("release with an invalid finding = %v", err)
	}
}

func TestBuilderIsolatesAnInvalidContribution(t *testing.T) {
	builder, err := NewBuilder("review", "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.AdmitServer(Server{Name: serverName("reviews"), Transport: stdio, Command: "review"}); err != nil {
		t.Fatal(err)
	}
	if err := builder.AdmitServer(Server{Name: serverName("reviews"), Transport: stdio, Command: "other"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate server = %v", err)
	}
	if err := builder.AdmitInput(Input{ID: "key", Server: serverName("missing"), Target: Environment, Key: "API_KEY"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("input of an undeclared server = %v", err)
	}
	if err := builder.AdmitInput(Input{ID: "key", Server: serverName("reviews"), Target: Environment, Key: "API_KEY", Secret: true}); err != nil {
		t.Fatal(err)
	}
	release, err := builder.Release(testDigest("1"))
	if err != nil {
		t.Fatal(err)
	}
	want := Declaration{Name: "review", Version: "1.0.0", Servers: []Server{{Name: serverName("reviews"), Transport: stdio, Command: "review"}}, Inputs: []Input{{ID: "key", Server: serverName("reviews"), Target: Environment, Key: "API_KEY", Secret: true}}}
	if got := release.Declaration(); !reflect.DeepEqual(got, want) {
		t.Fatalf("admitted declaration = %+v, want %+v", got, want)
	}
	again, err := NewRelease(release.Digest(), release.Declaration())
	if err != nil || !reflect.DeepEqual(again, release) {
		t.Fatalf("admitted declaration did not readmit identically: %v", err)
	}
}

func TestReleaseRejectsInvalidPortableServers(t *testing.T) {
	for _, server := range []Server{
		{Name: serverName("a"), Transport: stdio},
		{Name: serverName("a"), Transport: stdio, Command: "server", URL: "https://example.test"},
		{Name: serverName("a"), Transport: stdio, Command: "../server"},
		{Name: serverName("a"), Transport: stdio, Command: "server", Dir: "${PLUGIN_DATA}/../escape"},
		{Name: serverName("a"), Transport: stdio, Command: "server", Env: map[string]string{"PLUGIN_ROOT": "/other"}},
		{Name: serverName("a"), Transport: stdio, Command: "server", Env: map[string]string{"plugin_data": "/other"}},
		{Name: serverName("a"), Transport: stdio, Command: "server", Env: map[string]string{"LD_PRELOAD": "/other"}},
		{Name: serverName("a"), Transport: http, URL: "http://example.test/mcp"},
		{Name: serverName("a"), Transport: "streamable-http", URL: "https://example.test/mcp"},
		{Name: serverName("a"), Transport: http, URL: "https://user@example.test/mcp"},
		{Name: serverName("a"), Transport: http, URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "secret"}},
		{Name: serverName("a"), Transport: http, URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "one\x01two"}},
		{Name: serverName("a"), Transport: http, URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "one", "x-test": "two"}},
	} {
		t.Run(string(server.Transport)+"/"+server.Command+"/"+server.Dir+"/"+server.URL, func(t *testing.T) {
			if _, err := NewRelease(testDigest("1"), Declaration{Name: "review", Servers: []Server{server}}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid server admitted: %v", err)
			}
		})
	}
	if _, err := NewRelease(testDigest("1"), Declaration{Name: "review", Servers: []Server{{Name: serverName("local"), Transport: http, URL: "http://127.0.0.1:8080/mcp"}}}); err != nil {
		t.Fatalf("loopback HTTP server refused: %v", err)
	}
	public := Server{Name: serverName("public"), Transport: http, URL: "https://example.test/mcp", Headers: map[string]string{"X-Api-Key": "public client id"}}
	if _, err := NewRelease(testDigest("1"), Declaration{Name: "review", Servers: []Server{public}}); err != nil {
		t.Fatalf("static header in public package content refused by its name: %v", err)
	}
}

func TestPortableWorkingDirectoryUsesTheResourcePathContract(t *testing.T) {
	for _, suffix := range []string{"NUL.txt", "directory./file", "directory /file", "bad?file", "bad\x7ffile"} {
		t.Run(suffix, func(t *testing.T) {
			server := Server{Name: serverName("backend"), Transport: stdio, Command: "fixture", Dir: "${PLUGIN_DATA}/" + suffix}
			if _, err := NewRelease(testDigest("1"), Declaration{Name: "review", Servers: []Server{server}}); err == nil {
				t.Fatal("server declaration admitted a nonportable resource path")
			}
		})
	}
}

func TestReleaseRejectsInvalidInputOwnership(t *testing.T) {
	servers := []Server{{Name: serverName("server"), Transport: http, URL: "https://mcp.example/tools"}}
	for _, inputs := range [][]Input{
		{{ID: "unknown", Server: serverName("missing"), Target: Header, Key: "X-Tenant"}},
		{{ID: "invalid", Server: serverName("server"), Target: "unsupported"}},
		{{ID: "credential", Server: serverName("server"), Target: Authorization}},
		{{ID: "reserved", Server: serverName("server"), Target: Header, Key: "Authorization", Secret: true}},
		{{ID: "tenant", Server: serverName("server"), Target: Header, Key: "X-Tenant"}},
		{{ID: "env", Server: serverName("server"), Target: Environment, Key: "TENANT"}},
		{{ID: "first", Server: serverName("server"), Target: Header, Key: "X-Tenant", Secret: true}, {ID: "second", Server: serverName("server"), Target: Header, Key: "x-tenant", Secret: true}},
	} {
		if _, err := NewRelease(testDigest("1"), Declaration{Name: "package", Servers: servers, Inputs: inputs}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("admitted invalid inputs: %+v, %v", inputs, err)
		}
	}
}

func TestHeaderInputsAreAlwaysSecret(t *testing.T) {
	servers := []Server{{Name: serverName("server"), Transport: http, URL: "https://mcp.example/tools"}}
	for _, input := range []Input{
		{ID: "tenant", Server: serverName("server"), Target: Header, Key: "X-Tenant"},
		{ID: "credential", Server: serverName("server"), Target: Authorization},
	} {
		if _, err := NewRelease(testDigest("1"), Declaration{Name: "package", Servers: servers, Inputs: []Input{input}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("admitted a non-secret %s input: %v", input.Target, err)
		}
		input.Secret = true
		if _, err := NewRelease(testDigest("1"), Declaration{Name: "package", Servers: servers, Inputs: []Input{input}}); err != nil {
			t.Fatalf("refused a secret %s input: %v", input.Target, err)
		}
	}
}

func TestInputCannotCompeteWithStaticConfiguration(t *testing.T) {
	for _, test := range []struct {
		name   string
		server Server
		input  Input
	}{
		{"environment", Server{Name: serverName("server"), Transport: stdio, Command: "server", Env: map[string]string{"TENANT": "fixed"}}, Input{ID: "tenant", Server: serverName("server"), Target: Environment, Key: "TENANT"}},
		{"header", Server{Name: serverName("server"), Transport: http, URL: "https://example.test/mcp", Headers: map[string]string{"X-Tenant": "fixed"}}, Input{ID: "tenant", Server: serverName("server"), Target: Header, Key: "X-Tenant", Secret: true}},
		{"header case", Server{Name: serverName("server"), Transport: http, URL: "https://example.test/mcp", Headers: map[string]string{"X-Tenant": "fixed"}}, Input{ID: "tenant", Server: serverName("server"), Target: Header, Key: "x-tenant", Secret: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRelease(testDigest("1"), Declaration{Name: "package", Servers: []Server{test.server}, Inputs: []Input{test.input}}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("admitted two owners for a configuration binding: %v", err)
			}
		})
	}
}

func TestPortableEnvironmentBindingsHaveOneOwner(t *testing.T) {
	for _, test := range []struct {
		name   string
		env    map[string]string
		inputs []Input
	}{
		{"static case variants", map[string]string{"OPTION": "one", "option": "two"}, nil},
		{"input and static case variants", map[string]string{"OPTION": "one"}, []Input{{ID: "option", Server: serverName("server"), Target: Environment, Key: "option"}}},
		{"input case variants", nil, []Input{{ID: "first", Server: serverName("server"), Target: Environment, Key: "OPTION"}, {ID: "second", Server: serverName("server"), Target: Environment, Key: "option"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			declaration := Declaration{Name: "package", Servers: []Server{{Name: serverName("server"), Transport: stdio, Command: "server", Env: test.env}}, Inputs: test.inputs}
			if _, err := NewRelease(testDigest("1"), declaration); !errors.Is(err, ErrInvalid) {
				t.Fatalf("portable environment admitted competing bindings: %v", err)
			}
		})
	}
}
