package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestResolveFollowsSymlinkBeforeMissingSuffix(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	got, err := ResolvePath(root, filepath.Join("alias", "new", "file.txt"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	physicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	want := filepath.Join(physicalTarget, "new", "file.txt")
	if got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

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

func TestRelativePathRequiresExplicitAbsoluteRoot(t *testing.T) {
	if _, err := ResolvePath("", "relative.txt"); err == nil {
		t.Fatal("Resolve accepted a relative path without a root")
	}
	if _, err := ResolvePath("relative-root", "relative.txt"); err == nil {
		t.Fatal("Resolve accepted a relative root")
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

func TestResolveContinuesPhysicalTraversalAfterMissingParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "nested")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ path, want string }{
		{"missing/../alias/new", filepath.Join(physical, "nested", "new")},
		{"missing/../alias/../new", filepath.Join(physical, "new")},
	} {
		t.Run(test.path, func(t *testing.T) {
			got, err := ResolvePath(root, test.path)
			if err != nil || got != test.want {
				t.Fatalf("ResolvePath(%q) = (%q, %v), want %q", test.path, got, err, test.want)
			}
			if contained, err := ContainsPath(root, got); err != nil || contained {
				t.Fatalf("physical target was confined by its lexical alias: %t, %v", contained, err)
			}
		})
	}
}

func TestResolveRejectsTraversalThroughExistingFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"file/../new", "file/child", "file/."} {
		_, err := ResolvePath(root, path)
		var pathError *os.PathError
		if !errors.As(err, &pathError) || !errors.Is(err, syscall.ENOTDIR) {
			t.Fatalf("traversal through a regular file lost its filesystem cause: %q: %v", path, err)
		}
		if pathError.Path != root+string(filepath.Separator)+path {
			t.Fatalf("filesystem error changed the refused path: %q", pathError.Path)
		}
	}
}
