package pluginpackage

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type archiveEntry struct {
	name string
	body string
	mode fs.FileMode
}

func writeArchive(t *testing.T, entries []archiveEntry) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "package.zip")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	defer func() {
		if err := errors.Join(writer.Close(), file.Close()); err != nil {
			t.Fatal(err)
		}
	}()
	for _, entry := range entries {
		header := zip.FileHeader{Name: entry.name}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		output, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(output, entry.body); err != nil {
			t.Fatal(err)
		}
	}
	return name
}

func TestArchivesRejectEscapesAndAmbiguousDestinations(t *testing.T) {
	for _, names := range [][]string{
		{"plugin.json", "../escaped"},
		{"plugin.json", "A/file", "a/other"},
		{"plugin.json", "same", "same"},
		{"plugin.json", "link"},
		{"plugin.json", "a/", "a/"},
		{"plugin.json", "NUL.txt"},
		{"plugin.json", "COM¹.txt"},
		{"plugin.json", "nested/LPT².txt"},
		{"plugin.json", "directory./file"},
		{"plugin.json", "Café/file", "Cafe\u0301/other"},
		{"plugin.json", "same", "same/file"},
		{"plugin.json", "same/file", "same"},
		{"plugin.json", "same", "same/"},
		{"plugin.json", "same/", "same"},
		{"plugin.json", "parent/same", "parent/same/nested/file"},
		{"plugin.json", "parent/same/nested/file", "parent/same"},
	} {
		t.Run(strings.Join(names, ","), func(t *testing.T) {
			r := testReleases(t)
			entries := make([]archiveEntry, 0, len(names))
			for _, name := range names {
				entry := archiveEntry{name: name, body: "value"}
				if name == "plugin.json" {
					entry.body = portableManifest
				}
				if name == "link" {
					entry.mode = os.ModeSymlink | 0700
				}
				if strings.HasSuffix(name, "/") {
					entry.body = ""
				}
				entries = append(entries, entry)
			}
			if _, err := publishPackage(t.Context(), r, writeArchive(t, entries)); !errors.Is(err, plugin.ErrInvalid) {
				t.Fatalf("unsafe archive lost its validation category: %v", err)
			}
		})
	}
}

func TestArchiveDirectoryDeclarationsDoNotChangeReleaseIdentity(t *testing.T) {
	r := testReleases(t)
	expected, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{
		"plugin.json":         portableManifest,
		"nested/resource.txt": "value",
	}))
	if err != nil {
		t.Fatal(err)
	}
	manifest := archiveEntry{name: "plugin.json", body: portableManifest}
	resource := archiveEntry{name: "nested/resource.txt", body: "value"}
	directory := archiveEntry{name: "nested/"}
	for _, test := range []struct {
		name    string
		entries []archiveEntry
	}{
		{"implicit parent", []archiveEntry{manifest, resource}},
		{"declared parent before child", []archiveEntry{manifest, directory, resource}},
		{"declared parent after child", []archiveEntry{manifest, resource, directory}},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual, err := publishPackage(t.Context(), r, writeArchive(t, test.entries))
			if err != nil {
				t.Fatal(err)
			}
			if actual.Digest() != expected.Digest() {
				t.Fatalf("archive changed the admitted tree: got %s, want %s", actual.Digest(), expected.Digest())
			}
		})
	}
}

func TestArchiveAdmissionCountsImplicitDirectories(t *testing.T) {
	r := testReleases(t)
	entries := []archiveEntry{{name: "plugin.json", body: portableManifest}}
	for index := range maxFiles / 2 {
		entries = append(entries, archiveEntry{name: fmt.Sprintf("directory-%d/notes.txt", index), body: "bounded"})
	}
	if _, err := publishPackage(t.Context(), r, writeArchive(t, entries)); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("tree entry limit lost its validation category: %v", err)
	}
	remaining, err := os.ReadDir(r.directory)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("retained failed staging: %v, %v", remaining, err)
	}
}

func TestConflictingArchiveCannotAdvanceInstallation(t *testing.T) {
	for _, entries := range [][]archiveEntry{
		{{name: "plugin.json", body: portableManifest}, {name: "parent", body: "file"}, {name: "parent/child", body: "child"}},
		{{name: "plugin.json", body: portableManifest}, {name: "parent/child", body: "child"}, {name: "parent", body: "file"}},
	} {
		t.Run(entries[1].name, func(t *testing.T) {
			releases, store, _ := testReleaseStore(t)
			owner, err := plugins.New(t.Context(), store, releases.catalog, releases, &recordedConnections{}, passthroughDependencies{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := owner.Install(t.Context(), writePackage(t, map[string]string{"plugin.json": portableManifest}))
			if err != nil {
				t.Fatal(err)
			}
			archive := writeArchive(t, entries)
			if _, err := owner.Install(t.Context(), archive); !errors.Is(err, plugin.ErrInvalid) {
				t.Fatalf("conflicting installation lost its admission category: %v", err)
			}
			if _, err := owner.Stage(t.Context(), installed.View.ID, archive); !errors.Is(err, plugin.ErrInvalid) {
				t.Fatalf("conflicting update lost its admission category: %v", err)
			}
			current, err := store.List(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(current) != 1 {
				t.Fatalf("conflicting archive changed the installation set: %+v", current)
			}
			remaining := current[0]
			if remaining.Installation.ID() != installed.View.ID ||
				remaining.Installation.Enabled() != installed.View.Enabled ||
				remaining.Selected.Digest() != installed.Selected.Digest() || remaining.Staged != nil {
				t.Fatalf("conflicting archive changed durable installation state: %+v", current)
			}
			admitted, err := releases.catalog.Digests(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(admitted) != 1 || admitted[0] != installed.Selected.Digest() {
				t.Fatalf("conflicting archive left published release content: %v", admitted)
			}
		})
	}
}
