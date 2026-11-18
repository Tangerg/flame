package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContainsRequiresPhysicalTargets(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "src", "file.go")
	outside := filepath.Join(filepath.Dir(root), "outside", "file.go")

	for _, test := range []struct {
		name   string
		target string
		want   bool
	}{
		{name: "root", target: root, want: true},
		{name: "descendant", target: inside, want: true},
		{name: "sibling", target: outside, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ContainsPath(root, test.target)
			if err != nil {
				t.Fatalf("Contains: %v", err)
			}
			if got != test.want {
				t.Fatalf("Contains(%q, %q) = %v, want %v", root, test.target, got, test.want)
			}
		})
	}
}

func TestResolveFollowsSymlinkBeforeParentTraversal(t *testing.T) {
	root := t.TempDir()
	targetParent := t.TempDir()
	target := filepath.Join(targetParent, "nested")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	physicalParent, err := filepath.EvalSymlinks(targetParent)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"..", "../missing/child"} {
		t.Run(suffix, func(t *testing.T) {
			relative := "alias/" + suffix
			want := filepath.Join(physicalParent, "missing", "child")
			if suffix == ".." {
				want = physicalParent
			}
			for _, path := range []string{relative, root + string(filepath.Separator) + relative} {
				got, err := ResolvePath(root, path)
				if err != nil || got != want {
					t.Fatalf("ResolvePath(%q) = (%q, %v), want %q", path, got, err, want)
				}
			}
		})
	}
}
