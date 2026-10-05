package mcpserver

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func TestServerNameOwnsCanonicalLocalName(t *testing.T) {
	valid := []string{
		"a",
		"github",
		"html.to_design-v2",
		strings.Repeat("a", MaximumServerNameCharacters),
	}
	for _, raw := range valid {
		name, err := ParseServerName(raw)
		if err != nil {
			t.Errorf("ParseServerName(%q) error = %v", raw, err)
			continue
		}
		if name.String() != raw || name.Validate() != nil {
			t.Errorf("ParseServerName(%q) = %q, Validate = %v", raw, name.String(), name.Validate())
		}
	}

	invalid := []string{
		"",
		"GitHub",
		" github",
		"github ",
		"github/server",
		"installation/8ad9abf5-3a7d-4d0b-bef9-6ef92c20e746/files",
		"服务",
		strings.Repeat("a", MaximumServerNameCharacters+1),
	}
	for _, raw := range invalid {
		if _, err := ParseServerName(raw); !errors.Is(err, ErrInvalidServerName) {
			t.Errorf("ParseServerName(%q) error = %v, want ErrInvalidServerName", raw, err)
		}
	}

	var zero ServerName
	if !errors.Is(zero.Validate(), ErrInvalidServerName) {
		t.Fatalf("zero ServerName Validate error = %v", zero.Validate())
	}
}

func TestOriginIsAnExplicitClosedChoice(t *testing.T) {
	if !errors.Is(Origin{}.Validate(), ErrInvalidOrigin) {
		t.Fatal("zero origin validated")
	}
	if _, err := InstallationOrigin(resourceid.InstallationID{}); !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("InstallationOrigin(zero) error = %v", err)
	}
	if _, found := UserOrigin().Installation(); found || UserOrigin().Kind() != OriginUser {
		t.Fatal("user origin reported an installation")
	}
	installation := testInstallationID(t)
	origin, err := InstallationOrigin(installation)
	if err != nil {
		t.Fatal(err)
	}
	if got, found := origin.Installation(); !found || got != installation || origin.Kind() != OriginInstallation {
		t.Fatalf("installation origin = %v, %v", got, found)
	}
}

func TestIDSeparatesEqualNamesAcrossOrigins(t *testing.T) {
	name := testMCPServerName("files")
	if _, err := NewID(Origin{}, name); !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("NewID(zero origin) error = %v", err)
	}
	if _, err := NewID(UserOrigin(), ServerName{}); !errors.Is(err, ErrInvalidServerName) {
		t.Fatalf("NewID(zero name) error = %v", err)
	}
	if !errors.Is(ID{}.Validate(), ErrInvalidOrigin) {
		t.Fatal("zero ID validated")
	}
	origin, err := InstallationOrigin(testInstallationID(t))
	if err != nil {
		t.Fatal(err)
	}
	user, err := NewID(UserOrigin(), name)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := NewID(origin, name)
	if err != nil {
		t.Fatal(err)
	}
	if user == installed || user.Compare(installed) >= 0 || installed.Compare(user) <= 0 {
		t.Fatalf("user and installation IDs with one name collapsed: %v, %v", user, installed)
	}
}

func TestSourceBindsReleaseOnlyToInstallations(t *testing.T) {
	release, authority, recipient := fingerprint.Strings("release"), fingerprint.Strings("authority"), fingerprint.Strings("recipient")
	installation := testInstallationID(t)
	for name, build := range map[string]func() (Source, error){
		"missing installation": func() (Source, error) {
			return InstallationSource(resourceid.InstallationID{}, release, authority, recipient)
		},
		"missing release": func() (Source, error) {
			return InstallationSource(installation, fingerprint.Digest{}, authority, recipient)
		},
		"missing authority": func() (Source, error) {
			return InstallationSource(installation, release, fingerprint.Digest{}, recipient)
		},
		"missing recipient": func() (Source, error) {
			return InstallationSource(installation, release, authority, fingerprint.Digest{})
		},
	} {
		if _, err := build(); !errors.Is(err, ErrInvalidOrigin) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	if _, _, found := UserSource().Release(); found {
		t.Fatal("user source reported a release")
	}
	source, err := InstallationSource(installation, release, authority, recipient)
	if err != nil {
		t.Fatal(err)
	}
	gotRelease, gotAuthority, found := source.Release()
	if !found || gotRelease != release || gotAuthority != authority {
		t.Fatalf("installation source release = %v, %v, %v", gotRelease, gotAuthority, found)
	}
	server := Server{Source: source, Name: testMCPServerName("files"), Transport: TransportStdio, Command: "files"}
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
	if id := server.ID(); id.Origin() != source.Origin() || id.Name() != server.Name {
		t.Fatalf("server ID = %v", id)
	}
	server.Source = Source{}
	if err := server.Validate(); !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("server without source error = %v", err)
	}
}

func testInstallationID(t *testing.T) resourceid.InstallationID {
	t.Helper()
	id, err := resourceid.ParseInstallation("8ad9abf5-3a7d-4d0b-bef9-6ef92c20e746")
	if err != nil {
		t.Fatal(err)
	}
	return id
}
