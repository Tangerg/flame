package plugin

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestInstallationOwnsAtomicConfigurationAndReleaseTrust(t *testing.T) {
	release := Release{Digest: strings.Repeat("1", 64), Name: "review", Servers: []Server{{Name: "reviews", Type: "stdio", Command: "review"}}, Inputs: []Input{{ID: "key", Server: "reviews", Target: "env", Key: "API_KEY", Secret: true, Required: true}}, Requests: []RequestGrant{{Capability: "tools.invoke", Targets: []string{"reviews/record"}}}}
	installed, err := New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	release.Servers[0].Command = "modified"
	if installed.Snapshot().Selected.Servers[0].Command != "review" {
		t.Fatal("caller advanced the owned release")
	}
	if err = installed.Enable(true); !errors.Is(err, ErrUnapproved) {
		t.Fatal(err)
	}
	if err = installed.Approve(installed.Snapshot().Selected.Digest, installed.Snapshot().Selected.Requests); err != nil {
		t.Fatal(err)
	}
	key := "credential"
	before := installed.Snapshot()
	if err = installed.Configure(Configuration{Digest: release.Digest, Values: map[string]*string{"key": &key}, DisabledServers: []string{"reviews", "reviews"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, installed.Snapshot()) {
		t.Fatal("rejected transition changed installation facts")
	}
	if err = installed.Configure(Configuration{Digest: release.Digest, Values: map[string]*string{"key": &key}}); err != nil {
		t.Fatal(err)
	}
	if err = installed.Enable(true); err != nil {
		t.Fatal(err)
	}
	if err = installed.Configure(Configuration{Digest: release.Digest, Values: map[string]*string{"key": nil}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("enabled release lost its required input")
	}
	snapshot := installed.Snapshot()
	snapshot.Values["key"] = "changed"
	snapshot.Grants[0].Targets[0] = "reviews/other"
	if current := installed.Snapshot(); current.Grants[0].Targets[0] != "reviews/record" || current.Values["key"] != key {
		t.Fatal("projection advanced authority")
	}
	candidate := installed.Snapshot().Selected
	candidate.Digest = strings.Repeat("2", 64)
	if err = installed.Stage(candidate); err != nil {
		t.Fatal(err)
	}
	if installed.Snapshot().Selected.Digest == candidate.Digest {
		t.Fatal("staging activated code")
	}
	if err = installed.Select(candidate.Digest); err != nil {
		t.Fatal(err)
	}
	current := installed.Snapshot()
	if current.Enabled || current.ApprovedDigest != "" || len(current.Grants) > 0 || len(current.Values) > 0 || current.Staged != nil {
		t.Fatalf("new release inherited authority: %+v", current)
	}
}

func TestReleaseRequestsCannotCreateAnUndeclaredSource(t *testing.T) {
	release := Release{Digest: strings.Repeat("1", 64), Name: "review", Requests: []RequestGrant{{Capability: InvokeTools, Targets: []string{"missing/read"}}}}
	if err := release.Validate(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("undeclared source request = %v", err)
	}
}

func TestSelectedReleaseCannotAlsoBecomeStaged(t *testing.T) {
	release := Release{Digest: strings.Repeat("1", 64), Name: "review", Servers: []Server{{Name: "review", Type: Stdio, Command: "fixture"}}, Inputs: []Input{{ID: "key", Server: "review", Target: Environment, Key: "API_KEY", Secret: true}}}
	installation, err := New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	value := "configured-secret"
	if err := installation.Configure(Configuration{Digest: release.Digest, Values: map[string]*string{"key": &value}}); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(true); err != nil {
		t.Fatal(err)
	}
	before := installation.Snapshot()
	if err := installation.Stage(release); !errors.Is(err, ErrStale) {
		t.Fatalf("staging selected digest = %v", err)
	}
	if !reflect.DeepEqual(before, installation.Snapshot()) {
		t.Fatal("staging selected bytes changed installation configuration or authority")
	}
	before.Staged = &release
	if _, err := Restore(before); !errors.Is(err, ErrStale) {
		t.Fatalf("restored duplicate release positions = %v", err)
	}
}

func TestConfigurationCannotCrossAReleaseSelection(t *testing.T) {
	release := Release{
		Digest: strings.Repeat("1", 64), Name: "package",
		Servers: []Server{{Name: "server", Type: StreamableHTTP, URL: "https://first.example/mcp"}},
		Inputs:  []Input{{ID: "credential", Server: "server", Target: Authorization, Secret: true}},
		Skills:  []Skill{{Name: "review", Description: "Review changes"}},
	}
	installation, err := New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	draft := Configuration{
		Digest: release.Digest, Values: map[string]*string{"credential": new("Bearer reviewed-secret")},
		DisabledServers: []string{"server"}, DisabledSkills: []string{"review"},
	}
	next := release.Clone()
	next.Digest = strings.Repeat("2", 64)
	next.Servers[0].URL = "https://replacement.example/mcp"
	if err := installation.Stage(next); err != nil {
		t.Fatal(err)
	}
	if err := installation.Select(next.Digest); err != nil {
		t.Fatal(err)
	}
	before := installation.Snapshot()
	if err := installation.Configure(draft); !errors.Is(err, ErrStale) {
		t.Fatalf("configuration of a replaced release = %v, want stale release", err)
	}
	if !reflect.DeepEqual(before, installation.Snapshot()) {
		t.Fatal("stale configuration advanced replacement inputs or component enablement")
	}
	draft.Digest = next.Digest
	if err := installation.Configure(draft); err != nil {
		t.Fatalf("configuration of the selected release: %v", err)
	}
	current := installation.Snapshot()
	if current.Values["credential"] != *draft.Values["credential"] || !reflect.DeepEqual(current.DisabledServers, draft.DisabledServers) || !reflect.DeepEqual(current.DisabledSkills, draft.DisabledSkills) {
		t.Fatal("selected release did not accept its own configuration")
	}
}

func TestServerAuthorityOwnsOnlyCodeAndSourceGrants(t *testing.T) {
	release := Release{Digest: strings.Repeat("1", 64), Name: "review", Servers: []Server{{Name: "a", Type: "stdio", Command: "server"}, {Name: "b", Type: "stdio", Command: "server"}}, Inputs: []Input{{ID: "secret", Server: "a", Target: "env", Key: "API_KEY", Secret: true}}, Requests: []RequestGrant{{Capability: "tools.invoke", Targets: []string{"a/read", "a/write", "b/read"}}}}
	i, err := New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release.Digest, release.Requests); err != nil {
		t.Fatal(err)
	}
	a, b := i.ServerAuthority("a"), i.ServerAuthority("b")
	if err := i.Approve(release.Digest, []RequestGrant{{Capability: "tools.invoke", Targets: []string{"b/read", "a/write", "a/read"}}}); err != nil {
		t.Fatal(err)
	}
	secret := "rotated"
	if err := i.Configure(Configuration{Digest: release.Digest, Values: map[string]*string{"secret": &secret}, DisabledServers: []string{"b"}}); err != nil {
		t.Fatal(err)
	}
	if i.ServerAuthority("a") != a || i.ServerAuthority("b") != b {
		t.Fatal("configuration or grant ordering advanced tool authority")
	}
	if err := i.Approve(release.Digest, []RequestGrant{{Capability: "tools.invoke", Targets: []string{"a/read", "b/read"}}}); err != nil {
		t.Fatal(err)
	}
	if i.ServerAuthority("a") == a || i.ServerAuthority("b") != b {
		t.Fatal("source grant change was not isolated")
	}
	i.Revoke()
	if i.ServerAuthority("b") == b || i.Active() {
		t.Fatal("revocation retained authority")
	}
}

func TestRestoreRejectsInvalidPortableTransportSemantics(t *testing.T) {
	for _, server := range []Server{
		{Name: "a", Type: "stdio"},
		{Name: "a", Type: "stdio", Command: "server", URL: "https://example.test"},
		{Name: "a", Type: "stdio", Command: "../server"},
		{Name: "a", Type: "stdio", Command: "server", CWD: "${PLUGIN_DATA}/../escape"},
		{Name: "a", Type: "stdio", Command: "server", Env: map[string]string{"PLUGIN_ROOT": "/other"}},
		{Name: "a", Type: "stdio", Command: "server", Env: map[string]string{"LD_PRELOAD": "/other"}},
		{Name: "a", Type: "streamable-http", URL: "http://example.test/mcp"},
		{Name: "a", Type: "streamableHttp", URL: "https://example.test/mcp"},
		{Name: "a", Type: "streamable-http", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "secret"}},
		{Name: "a", Type: "streamable-http", URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "one\x01two"}},
		{Name: "a", Type: "streamable-http", URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "one", "x-test": "two"}},
	} {
		t.Run(string(server.Type)+"/"+server.Command+"/"+server.CWD+"/"+server.URL, func(t *testing.T) {
			_, err := New(uuid.NewString(), "/package", Release{Digest: strings.Repeat("1", 64), Name: "review", Servers: []Server{server}})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid transport admitted: %v", err)
			}
		})
	}
}

func TestPortableWorkingDirectoryUsesTheResourcePathContract(t *testing.T) {
	for _, suffix := range []string{"NUL.txt", "directory./file", "directory /file", "bad?file", "bad\x7ffile"} {
		t.Run(suffix, func(t *testing.T) {
			server := Server{Name: "backend", Type: Stdio, Command: "fixture", CWD: "${PLUGIN_DATA}/" + suffix}
			if err := server.Validate(); err == nil {
				t.Fatal("server declaration admitted a nonportable resource path")
			}
		})
	}
}

func TestHTTPInputConfigurationRejectsInvalidCredentialsAtomically(t *testing.T) {
	release := Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []Server{{Name: "server", Type: StreamableHTTP, URL: "https://example.test/mcp"}}, Inputs: []Input{{ID: "credential", Server: "server", Target: Authorization, Secret: true}}}
	i, err := New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	before := i.Snapshot()
	for _, value := range []string{"Bearer one\x01two", " Bearer secret", "Bearer secret "} {
		if err := i.Configure(Configuration{Digest: release.Digest, Values: map[string]*string{"credential": &value}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid HTTP credential was persisted: %v", err)
		}
		if !reflect.DeepEqual(before, i.Snapshot()) {
			t.Fatal("invalid credential advanced installation configuration")
		}
	}
}

func TestDisabledBackendDoesNotRequireItsInputs(t *testing.T) {
	release := Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []Server{{Name: "a", Type: "stdio", Command: "server"}, {Name: "b", Type: "stdio", Command: "server"}}, Inputs: []Input{{ID: "secret", Server: "b", Target: "env", Key: "API_KEY", Secret: true, Required: true}}}
	i, err := New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Configure(Configuration{Digest: release.Digest, DisabledServers: []string{"b"}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	if err := i.Enable(true); err != nil {
		t.Fatal(err)
	}
	before := i.Snapshot()
	if err := i.Configure(Configuration{Digest: release.Digest}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("enabled backend without required input: %v", err)
	}
	if !reflect.DeepEqual(before, i.Snapshot()) {
		t.Fatal("failed configuration advanced component enablement")
	}
}

func TestReleaseRejectsInvalidInputOwnership(t *testing.T) {
	base := Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []Server{{Name: "server", Type: StreamableHTTP, URL: "https://mcp.example/tools"}}}
	for _, inputs := range [][]Input{
		{{ID: "unknown", Server: "missing", Target: Header, Key: "X-Tenant"}},
		{{ID: "invalid", Server: "server", Target: "unsupported"}},
		{{ID: "credential", Server: "server", Target: Authorization}},
		{{ID: "reserved", Server: "server", Target: Header, Key: "Authorization", Secret: true}},
		{{ID: "first", Server: "server", Target: Header, Key: "X-Tenant"}, {ID: "second", Server: "server", Target: Header, Key: "x-tenant"}},
	} {
		release := base.Clone()
		release.Inputs = inputs
		if _, err := New(uuid.NewString(), "/package", release); !errors.Is(err, ErrInvalid) {
			t.Fatalf("admitted invalid inputs: %+v, %v", inputs, err)
		}
	}
	base.Requests = []RequestGrant{{Capability: "runtime.query", Targets: []string{"plan.get"}}}
	if _, err := New(uuid.NewString(), "/package", base); !errors.Is(err, ErrInvalid) {
		t.Fatalf("admitted an unavailable request: %v", err)
	}
}

func TestInputCannotCompeteWithStaticConfiguration(t *testing.T) {
	for _, test := range []struct {
		name   string
		server Server
		input  Input
	}{
		{"environment", Server{Name: "server", Type: Stdio, Command: "server", Env: map[string]string{"TENANT": "fixed"}}, Input{ID: "tenant", Server: "server", Target: Environment, Key: "TENANT"}},
		{"header", Server{Name: "server", Type: StreamableHTTP, URL: "https://example.test/mcp", Headers: map[string]string{"X-Tenant": "fixed"}}, Input{ID: "tenant", Server: "server", Target: Header, Key: "X-Tenant"}},
		{"header case", Server{Name: "server", Type: StreamableHTTP, URL: "https://example.test/mcp", Headers: map[string]string{"X-Tenant": "fixed"}}, Input{ID: "tenant", Server: "server", Target: Header, Key: "x-tenant"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			release := Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []Server{test.server}, Inputs: []Input{test.input}}
			if err := release.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("admitted two owners for a configuration binding: %v", err)
			}
		})
	}
}

func TestOptionalInputPresenceAdvancesSourceConfiguration(t *testing.T) {
	release := Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []Server{{Name: "server", Type: Stdio, Command: "server"}}, Inputs: []Input{{ID: "option", Server: "server", Target: Environment, Key: "OPTION"}}}
	installation, err := New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []*string{new(""), nil} {
		previous, err := Restore(installation.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		if err := installation.Configure(Configuration{Digest: release.Digest, Values: map[string]*string{"option": value}}); err != nil {
			t.Fatal(err)
		}
		if changed := installation.ChangedServers(previous); len(changed) != 1 || changed[0] != "server" {
			t.Fatalf("input presence did not change source configuration: %v", changed)
		}
		if installation.RetainsServerCredentials(previous, "server") {
			t.Fatal("changed requesting configuration retained its OAuth grant")
		}
		if installation.ServerAuthority("server") != previous.ServerAuthority("server") {
			t.Fatal("input presence changed standing tool permission")
		}
	}
}

func TestPortableEnvironmentBindingsHaveOneOwner(t *testing.T) {
	for _, test := range []struct {
		name   string
		env    map[string]string
		inputs []Input
	}{
		{"static case variants", map[string]string{"OPTION": "one", "option": "two"}, nil},
		{"input and static case variants", map[string]string{"OPTION": "one"}, []Input{{ID: "option", Server: "server", Target: Environment, Key: "option"}}},
		{"input case variants", nil, []Input{{ID: "first", Server: "server", Target: Environment, Key: "OPTION"}, {ID: "second", Server: "server", Target: Environment, Key: "option"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			release := Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []Server{{Name: "server", Type: Stdio, Command: "server", Env: test.env}}, Inputs: test.inputs}
			if err := release.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("portable environment admitted competing bindings: %v", err)
			}
		})
	}
}
