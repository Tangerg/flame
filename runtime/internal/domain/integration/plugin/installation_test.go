package plugin

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/google/uuid"
)

func TestInstallationOwnsAtomicConfigurationAndReleaseTrust(t *testing.T) {
	release := testRelease(t, "1", Declaration{Name: "review", Servers: []Server{{Name: serverName("reviews"), Transport: stdio, Command: "review"}}, Inputs: []Input{{ID: "key", Server: serverName("reviews"), Target: Environment, Key: "API_KEY", Secret: true, Required: true}}})
	installed, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err = installed.Enable(release); !errors.Is(err, ErrUnapproved) {
		t.Fatal(err)
	}
	if err = installed.Approve(release); err != nil {
		t.Fatal(err)
	}
	before := installed.Snapshot()
	if err = installed.Configure(release, Configuration{Values: map[string]ValueChange{"key": SetValue("credential")}, Servers: map[mcpserver.ServerName]ComponentChange{serverName("missing"): DisableComponent}}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err = installed.Configure(release, Configuration{Values: map[string]ValueChange{"key": {}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero value change = %v", err)
	}
	if !reflect.DeepEqual(before, installed.Snapshot()) {
		t.Fatal("rejected transition changed installation facts")
	}
	if err = installed.Configure(release, Configuration{Values: map[string]ValueChange{"key": SetValue("credential")}}); err != nil {
		t.Fatal(err)
	}
	if err = installed.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err = installed.Configure(release, Configuration{Values: map[string]ValueChange{"key": ClearValue()}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("enabled release lost its required input")
	}
	snapshot := installed.Snapshot()
	snapshot.Values["key"] = "changed"
	if current := installed.Snapshot(); current.Values["key"] != "credential" {
		t.Fatal("projection advanced configuration")
	}
}

func TestInstallationStateTransitionsAreClosed(t *testing.T) {
	release := testRelease(t, "1", Declaration{Name: "review", Servers: []Server{{Name: serverName("reviews"), Transport: stdio, Command: "review"}}})
	installation, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	state := func(want State) {
		t.Helper()
		if got := installation.State(); got != want {
			t.Fatalf("state = %q, want %q", got, want)
		}
	}
	state(Unapproved)
	if err := installation.Enable(release); !errors.Is(err, ErrUnapproved) {
		t.Fatalf("enable unapproved = %v", err)
	}
	installation.Disable()
	state(Unapproved)
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	state(Approved)
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	state(Enabled)
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	state(Enabled)
	installation.Disable()
	state(Approved)
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	installation.Revoke()
	state(Unapproved)
	if installation.Active() {
		t.Fatal("revoked installation is active")
	}
	record := installation.Snapshot()
	for _, invalid := range []State{"", "active", "approved,enabled"} {
		record.State = invalid
		if _, err := Restore(record); !errors.Is(err, ErrInvalid) {
			t.Fatalf("restored state %q = %v", invalid, err)
		}
	}
}

func TestTransitionsRefuseAReleaseTheyDoNotName(t *testing.T) {
	declaration := Declaration{Name: "review", Servers: []Server{{Name: serverName("reviews"), Transport: stdio, Command: "review"}}, Inputs: []Input{{ID: "mode", Server: serverName("reviews"), Target: Environment, Key: "MODE"}}}
	release := testRelease(t, "1", declaration)
	other := testRelease(t, "2", declaration)
	installation, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	before := installation.Snapshot()
	for transition, apply := range map[string]func() error{
		"approve": func() error { return installation.Approve(other) },
		"enable":  func() error { return installation.Enable(other) },
		"configure": func() error {
			return installation.Configure(other, Configuration{Values: map[string]ValueChange{"mode": SetValue("strict")}})
		},
		"stage": func() error { return installation.Stage(other, release) },
		"source": func() error {
			_, err := installation.ServerSource(other, serverName("reviews"))
			return err
		},
	} {
		if err := apply(); !errors.Is(err, ErrStale) {
			t.Fatalf("%s with an unselected release = %v", transition, err)
		}
	}
	if _, err := installation.ServerIDs(other); !errors.Is(err, ErrStale) {
		t.Fatalf("server identities of an unselected release = %v", err)
	}
	if !reflect.DeepEqual(before, installation.Snapshot()) {
		t.Fatal("a refused transition advanced the installation")
	}
}

func TestSelectedReleaseCannotAlsoBecomeStaged(t *testing.T) {
	release := testRelease(t, "1", Declaration{Name: "review", Servers: []Server{{Name: serverName("review"), Transport: stdio, Command: "fixture"}}})
	installation, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	before := installation.Snapshot()
	if err := installation.Stage(release, release); !errors.Is(err, ErrStale) {
		t.Fatalf("staging selected digest = %v", err)
	}
	if !reflect.DeepEqual(before, installation.Snapshot()) {
		t.Fatal("staging selected bytes changed installation configuration or authority")
	}
	selected := release.Digest()
	before.Staged = &selected
	if _, err := Restore(before); !errors.Is(err, ErrStale) {
		t.Fatalf("restored duplicate release positions = %v", err)
	}
	other := testRelease(t, "2", Declaration{Name: "other"})
	if err := installation.Stage(release, other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("staging another package = %v", err)
	}
}

func TestSelectRequiresRenewedApprovalOfTheNewDigest(t *testing.T) {
	declaration := Declaration{Name: "review", Servers: []Server{{Name: serverName("a"), Transport: stdio, Command: "./server"}}}
	release := testRelease(t, "1", declaration)
	installation := approvedInstallation(t, release, nil)
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	next := declaration
	next.Version = "2"
	candidate := testRelease(t, "2", next)
	selectCandidate(t, installation, release, candidate)
	if installation.State() != Unapproved || installation.Active() || installation.ServerEnabled(serverName("a")) {
		t.Fatalf("selected code inherited approval: %q", installation.State())
	}
	if err := installation.Enable(candidate); !errors.Is(err, ErrUnapproved) {
		t.Fatalf("enable unreviewed release = %v", err)
	}
	if err := installation.Approve(candidate); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(candidate); err != nil {
		t.Fatal(err)
	}
}

func TestSelectRetainsConfigurationWhoseMeaningIsUnchanged(t *testing.T) {
	declaration := Declaration{
		Name: "review",
		Servers: []Server{
			{Name: serverName("a"), Transport: stdio, Command: "./server"},
			{Name: serverName("b"), Transport: http, URL: "https://b.example/mcp"},
		},
		Inputs: []Input{
			{ID: "mode", Server: serverName("a"), Target: Environment, Key: "MODE"},
			{ID: "token", Server: serverName("b"), Target: Authorization, Secret: true},
			{ID: "region", Server: serverName("b"), Target: Header, Key: "X-Region", Secret: true},
			{ID: "zone", Server: serverName("b"), Target: Header, Key: "X-Zone", Secret: true},
		},
		Skills: []Skill{{Name: "review", Description: "Review"}, {Name: "audit", Description: "Audit"}},
	}
	release := testRelease(t, "1", declaration)
	installation := approvedInstallation(t, release, map[string]string{"mode": "strict", "token": "Bearer secret", "region": "eu", "zone": "z1"})
	if err := installation.Configure(release, Configuration{Servers: map[mcpserver.ServerName]ComponentChange{serverName("b"): DisableComponent}, Skills: map[string]ComponentChange{"audit": DisableComponent}}); err != nil {
		t.Fatal(err)
	}

	next := declaration
	next.Version = "2"
	next.Inputs = []Input{
		{ID: "mode", Server: serverName("a"), Target: Environment, Key: "MODE", Required: true},
		{ID: "token", Server: serverName("b"), Target: Authorization, Secret: true},
		{ID: "region", Server: serverName("b"), Target: Header, Key: "X-Area", Secret: true},
		{ID: "zone", Server: serverName("b"), Target: Header, Key: "X-Zone", Secret: true},
	}
	next.Skills = []Skill{{Name: "review", Description: "Review"}}
	candidate := testRelease(t, "2", next)
	selectCandidate(t, installation, release, candidate)

	current := installation.Snapshot()
	if want := map[string]string{"mode": "strict", "token": "Bearer secret", "zone": "z1"}; !reflect.DeepEqual(current.Values, want) {
		t.Fatalf("retained values = %+v, want %+v", current.Values, want)
	}
	if !reflect.DeepEqual(current.DisabledServers, []mcpserver.ServerName{serverName("b")}) || len(current.DisabledSkills) != 0 {
		t.Fatalf("retained disabled components = %v, %v", current.DisabledServers, current.DisabledSkills)
	}
	if current.Selected != candidate.Digest() || current.Staged != nil || current.State != Unapproved {
		t.Fatalf("selection = %s staged %v state %q", current.Selected, current.Staged, current.State)
	}
}

func TestCredentialsFollowTheirRecipientAcrossReleases(t *testing.T) {
	base := Declaration{
		Name: "review",
		Servers: []Server{
			{Name: serverName("local"), Transport: stdio, Command: "./server"},
			{Name: serverName("remote"), Transport: http, URL: "https://a.example/mcp", Headers: map[string]string{"X-Client": "flame"}},
		},
		Inputs: []Input{
			{ID: "api-key", Server: serverName("local"), Target: Environment, Key: "API_KEY", Secret: true},
			{ID: "mode", Server: serverName("local"), Target: Environment, Key: "MODE"},
			{ID: "token", Server: serverName("remote"), Target: Authorization, Secret: true},
		},
	}
	values := map[string]string{"api-key": "process secret", "mode": "strict", "token": "Bearer secret"}
	for _, test := range []struct {
		name   string
		change func(*Declaration)
		want   map[string]string
	}{
		{"unchanged endpoint", func(*Declaration) {}, map[string]string{"mode": "strict", "token": "Bearer secret"}},
		{"changed URL", func(d *Declaration) {
			d.Servers[1].URL = "https://elsewhere.example/mcp"
		}, map[string]string{"mode": "strict"}},
		{"changed static header", func(d *Declaration) {
			d.Servers[1].Headers = map[string]string{"X-Client": "other"}
		}, map[string]string{"mode": "strict"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			release := testRelease(t, "1", base)
			installation := approvedInstallation(t, release, values)
			next := base
			next.Version = "2"
			next.Servers = []Server{base.Servers[0].clone(), base.Servers[1].clone()}
			test.change(&next)
			candidate := testRelease(t, "2", next)
			previous, err := installation.ServerSource(release, serverName("remote"))
			if err != nil {
				t.Fatal(err)
			}
			selectCandidate(t, installation, release, candidate)
			if current := installation.Snapshot().Values; !reflect.DeepEqual(current, test.want) {
				t.Fatalf("retained values = %+v, want %+v", current, test.want)
			}
			current, err := installation.ServerSource(candidate, serverName("remote"))
			if err != nil {
				t.Fatal(err)
			}
			retained := previous.ID(serverName("remote")) == current.ID(serverName("remote")) &&
				oauth(previous).Fingerprint() == oauth(current).Fingerprint()
			if _, wantRetained := test.want["token"]; retained != wantRetained {
				t.Fatalf("OAuth credential retained = %v, want %v", retained, wantRetained)
			}
		})
	}
}

func oauth(source mcpserver.Source) mcpserver.OAuthTarget {
	return mcpserver.OAuthTarget{Source: source, Name: serverName("remote"), URL: "https://a.example/mcp"}
}

func TestServerAuthorityIsTheSelectedCodeOfOneServer(t *testing.T) {
	declaration := Declaration{Name: "review", Servers: []Server{{Name: serverName("a"), Transport: stdio, Command: "server"}, {Name: serverName("b"), Transport: stdio, Command: "server"}}, Inputs: []Input{{ID: "secret", Server: serverName("a"), Target: Environment, Key: "API_KEY", Secret: true}}}
	release := testRelease(t, "1", declaration)
	i, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	a, b := i.ServerAuthority(serverName("a")), i.ServerAuthority(serverName("b"))
	if a == b {
		t.Fatal("sibling servers share tool authority")
	}
	if err := i.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := i.Configure(release, Configuration{Values: map[string]ValueChange{"secret": SetValue("rotated")}, Servers: map[mcpserver.ServerName]ComponentChange{serverName("b"): DisableComponent}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Enable(release); err != nil {
		t.Fatal(err)
	}
	if i.ServerAuthority(serverName("a")) != a || i.ServerAuthority(serverName("b")) != b {
		t.Fatal("approval, configuration or enablement advanced tool authority")
	}
	next := declaration
	next.Version = "2"
	candidate := testRelease(t, "2", next)
	selectCandidate(t, i, release, candidate)
	if i.ServerAuthority(serverName("a")) == a || i.ServerAuthority(serverName("b")) == b {
		t.Fatal("a release change kept standing tool authority")
	}
}

func TestHTTPInputConfigurationRejectsInvalidCredentialsAtomically(t *testing.T) {
	release := testRelease(t, "1", Declaration{Name: "package", Servers: []Server{{Name: serverName("server"), Transport: http, URL: "https://example.test/mcp"}}, Inputs: []Input{{ID: "credential", Server: serverName("server"), Target: Authorization, Secret: true}}})
	i, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	before := i.Snapshot()
	for _, value := range []string{"Bearer one\x01two", " Bearer secret", "Bearer secret "} {
		if err := i.Configure(release, Configuration{Values: map[string]ValueChange{"credential": SetValue(value)}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid HTTP credential was persisted: %v", err)
		}
		if !reflect.DeepEqual(before, i.Snapshot()) {
			t.Fatal("invalid credential advanced installation configuration")
		}
	}
}

func TestConfigurationIsADelta(t *testing.T) {
	release := testRelease(t, "1", Declaration{
		Name:    "package",
		Servers: []Server{{Name: serverName("a"), Transport: stdio, Command: "server"}, {Name: serverName("b"), Transport: stdio, Command: "server"}},
		Inputs:  []Input{{ID: "one", Server: serverName("a"), Target: Environment, Key: "ONE"}, {ID: "two", Server: serverName("a"), Target: Environment, Key: "TWO"}},
		Skills:  []Skill{{Name: "review", Description: "Review"}},
	})
	i := approvedInstallation(t, release, map[string]string{"one": "1", "two": "2"})
	if err := i.Configure(release, Configuration{Servers: map[mcpserver.ServerName]ComponentChange{serverName("a"): DisableComponent, serverName("b"): DisableComponent}, Skills: map[string]ComponentChange{"review": DisableComponent}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Configure(release, Configuration{Values: map[string]ValueChange{"two": ClearValue()}, Servers: map[mcpserver.ServerName]ComponentChange{serverName("a"): EnableComponent}}); err != nil {
		t.Fatal(err)
	}
	current := i.Snapshot()
	if !reflect.DeepEqual(current.Values, map[string]string{"one": "1"}) {
		t.Fatalf("values = %v", current.Values)
	}
	if !reflect.DeepEqual(current.DisabledServers, []mcpserver.ServerName{serverName("b")}) || !reflect.DeepEqual(current.DisabledSkills, []string{"review"}) {
		t.Fatalf("disabled = %v, %v", current.DisabledServers, current.DisabledSkills)
	}
	if err := i.Configure(release, Configuration{Skills: map[string]ComponentChange{"review": "toggle"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown component change = %v", err)
	}
}

func TestDisabledBackendDoesNotRequireItsInputs(t *testing.T) {
	release := testRelease(t, "1", Declaration{Name: "package", Servers: []Server{{Name: serverName("a"), Transport: stdio, Command: "server"}, {Name: serverName("b"), Transport: stdio, Command: "server"}}, Inputs: []Input{{ID: "secret", Server: serverName("b"), Target: Environment, Key: "API_KEY", Secret: true, Required: true}}})
	i, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Configure(release, Configuration{Servers: map[mcpserver.ServerName]ComponentChange{serverName("b"): DisableComponent}}); err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := i.Enable(release); err != nil {
		t.Fatal(err)
	}
	before := i.Snapshot()
	if err := i.Configure(release, Configuration{Servers: map[mcpserver.ServerName]ComponentChange{serverName("b"): EnableComponent}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("enabled backend without required input: %v", err)
	}
	if !reflect.DeepEqual(before, i.Snapshot()) {
		t.Fatal("failed configuration advanced component enablement")
	}
}

func TestOptionalInputPresenceAdvancesSourceConfiguration(t *testing.T) {
	release := testRelease(t, "1", Declaration{Name: "package", Servers: []Server{{Name: serverName("server"), Transport: stdio, Command: "server"}}, Inputs: []Input{{ID: "option", Server: serverName("server"), Target: Environment, Key: "OPTION"}}})
	installation, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []ValueChange{SetValue(""), ClearValue()} {
		previous, err := Restore(installation.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		if err := installation.Configure(release, Configuration{Values: map[string]ValueChange{"option": change}}); err != nil {
			t.Fatal(err)
		}
		changed, err := installation.ChangedServers(previous, release, release)
		if err != nil || len(changed) != 1 || changed[0] != serverName("server") {
			t.Fatalf("input presence did not change source configuration: %v, %v", changed, err)
		}
		if installation.ServerAuthority(serverName("server")) != previous.ServerAuthority(serverName("server")) {
			t.Fatal("input presence changed standing tool permission")
		}
	}
}

func approvedInstallation(t *testing.T, release Release, values map[string]string) *Installation {
	t.Helper()
	installation, err := New(testInstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]ValueChange{}
	for id, value := range values {
		changes[id] = SetValue(value)
	}
	if err := installation.Configure(release, Configuration{Values: changes}); err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	return installation
}

func selectCandidate(t *testing.T, installation *Installation, selected, candidate Release) {
	t.Helper()
	if err := installation.Stage(selected, candidate); err != nil {
		t.Fatal(err)
	}
	if err := installation.Select(selected, selected); !errors.Is(err, ErrStale) {
		t.Fatalf("selecting an unstaged release = %v", err)
	}
	if err := installation.Select(selected, candidate); err != nil {
		t.Fatal(err)
	}
}

func testRelease(t *testing.T, seed string, declaration Declaration) Release {
	t.Helper()
	release, err := NewRelease(testDigest(seed), declaration)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func testDigest(seed string) fingerprint.Digest { return fingerprint.Strings(seed) }

func serverName(raw string) mcpserver.ServerName {
	parsed, err := mcpserver.ParseServerName(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}

func testInstallationID(t *testing.T) resourceid.InstallationID {
	t.Helper()
	id, err := resourceid.ParseInstallation(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return id
}
