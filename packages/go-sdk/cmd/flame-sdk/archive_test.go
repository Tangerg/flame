package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestArchivePreservesPackageRootAndExecutableMode(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "bin", "backend"), []byte("executable"), 0755); err != nil {
		t.Fatal(err)
	}
	var previous []byte
	for _, name := range []string{"first.zip", "second.zip"} {
		output := filepath.Join(root, name)
		if err := archive(source, output, ""); err != nil {
			t.Fatal(err)
		}
		contents, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && !bytes.Equal(contents, previous) {
			t.Fatal("unchanged package bytes produced another archive")
		}
		previous = contents
		reader, err := zip.OpenReader(output)
		if err != nil {
			t.Fatal(err)
		}
		if len(reader.File) != 1 || reader.File[0].Name != "bin/backend" || reader.File[0].Mode().Perm() != 0755 {
			t.Fatalf("archive resources: %+v", reader.File)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive(source, filepath.Join(source, "self.zip"), ""); err == nil {
		t.Fatal("archive admitted its own output as source material")
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	if err := archive(source, filepath.Join(alias, "self.zip"), ""); err == nil {
		t.Fatal("archive admitted its own output through a filesystem alias")
	}
	if err := archive(source, filepath.Join(root, "first.zip"), ""); err == nil {
		t.Fatal("archive replaced an existing file")
	}
}

func TestArchiveRefusesInvalidSourceWithoutLeavingARelease(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "plain")
	if err := os.WriteFile(plain, []byte("not a package directory"), 0644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(plain, filepath.Join(source, "linked")); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{plain, source} {
		output := filepath.Join(root, "release.zip")
		if err := archive(input, output, ""); err == nil {
			t.Fatal("invalid source produced a release")
		}
		if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed archive left a release: %v", err)
		}
	}
}

func TestArchiveResolvesPhysicalOutputParent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	physical := filepath.Join(root, "physical")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "entry.js"), []byte("export default 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(physical, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(physical, "child"), alias); err != nil {
		t.Fatal(err)
	}
	output := alias + string(filepath.Separator) + ".." + string(filepath.Separator) + "release.zip"
	if err := archive(source, output, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(physical, "release.zip")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "release.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive published at the lexical parent: %v", err)
	}
}
