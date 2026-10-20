package localruntime

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveProductRoot(t *testing.T) {
	home := t.TempDir()
	configured := filepath.Join(t.TempDir(), " product root ")
	for _, test := range []struct {
		name, home, configured, want string
	}{
		{name: "default", home: home, want: filepath.Join(home, ".flame")},
		{name: "configured", home: home, configured: configured, want: configured},
		{name: "configured without home", configured: configured, want: configured},
		{name: "configured with relative home", home: "relative", configured: configured, want: configured},
		{name: "missing home"},
		{name: "relative home", home: "relative"},
		{name: "relative configuration", home: home, configured: "relative"},
		{name: "whitespace configuration", home: home, configured: " "},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, err := ResolveProductRoot(test.home, test.configured)
			if test.want == "" {
				if err == nil || root != "" {
					t.Fatalf("invalid product root resolved as %q: %v", root, err)
				}
				return
			}
			if err != nil || root != test.want {
				t.Fatalf("product root = %q, error = %v, want %q", root, err, test.want)
			}
		})
	}
}

func TestDataDirectoryOwnsLocalDeploymentLayout(t *testing.T) {
	home := t.TempDir()
	root, err := ResolveProductRoot(home, "")
	if err != nil {
		t.Fatal(err)
	}
	directory, err := DataDirectoryUnder(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".flame", "runtime")
	if directory.Path() != want {
		t.Fatalf("path = %q, want %q", directory.Path(), want)
	}
	if directory.DatabasePath() != filepath.Join(want, "flame.db") {
		t.Fatalf("database path = %q", directory.DatabasePath())
	}
	if directory.LocalTokenPath() != filepath.Join(want, "local-token") {
		t.Fatalf("token path = %q", directory.LocalTokenPath())
	}
}

func TestDataDirectoryRejectsUnownedPaths(t *testing.T) {
	for _, path := range []string{"", "relative"} {
		if _, err := DataDirectoryAt(path); !errors.Is(err, ErrInvalidDataDirectory) {
			t.Fatalf("DataDirectoryAt(%q) error = %v", path, err)
		}
		if _, err := DataDirectoryUnder(path); !errors.Is(err, ErrInvalidDataDirectory) {
			t.Fatalf("DataDirectoryUnder(%q) error = %v", path, err)
		}
	}
}
