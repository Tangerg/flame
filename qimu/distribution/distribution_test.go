package distribution

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu"
)

func packageFixture(t *testing.T) (string, Manifest, []byte, Lock) {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "theme.css"), []byte("body { color: blue; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 1, Package: "test.theme", Version: "1.0.0", Artifacts: []Artifact{{Path: "theme.css"}}, Units: []Unit{{ID: "theme", Profile: "static-resource", Resources: []string{"theme.css"}}}}
	var archive bytes.Buffer
	lock, err := Build(context.Background(), directory, manifest, nil, &archive)
	if err != nil {
		t.Fatal(err)
	}
	return directory, manifest, archive.Bytes(), lock
}

func storeFixture(t *testing.T, retained func(context.Context, qimu.Digest) (bool, error)) *Store {
	t.Helper()
	if retained == nil {
		retained = func(context.Context, qimu.Digest) (bool, error) { return false, nil }
	}
	store, err := Open(Config{Directory: t.TempDir(), Admission: qimu.NewAdmission(), Trust: func(context.Context, Source, Manifest, Lock) error { return nil }, Retained: retained})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Close(ctx); err != nil {
			t.Error(err)
		}
		if err := removeSealed(store.directory); err != nil {
			t.Error(err)
		}
	})
	return store
}

func installArchive(t *testing.T, store *Store, data []byte) Release {
	t.Helper()
	file := filepath.Join(t.TempDir(), "plugin.zip")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	release, err := store.InstallArchive(context.Background(), file, qimu.Hash(data))
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func TestBuildDeterministicAndResourceNeedsNoMethods(t *testing.T) {
	directory, manifest, archive, lock := packageFixture(t)
	var second bytes.Buffer
	other, err := Build(context.Background(), directory, manifest, nil, &second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(archive, second.Bytes()) || lock.Release != other.Release {
		t.Fatal("same release produced different bytes")
	}
	store := storeFixture(t, nil)
	release := installArchive(t, store, archive)
	if release.Digest != lock.Release || len(release.Manifest.Units[0].Exports) != 0 {
		t.Fatal("resource release changed its declaration")
	}
	lease, err := store.Acquire(context.Background(), release.Digest)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	data, err := lease.Read(context.Background(), "theme.css")
	if err != nil || string(data) != "body { color: blue; }\n" {
		t.Fatalf("resource: %q %v", data, err)
	}
}

func TestArchivePathsAliasesAndSymlinksAreRefusedBeforeTrust(t *testing.T) {
	_, _, valid, _ := packageFixture(t)
	for _, name := range []string{"../escape", "/absolute", "a\\b", "A/theme.css", "con.txt"} {
		t.Run(name, func(t *testing.T) {
			store := storeFixture(t, nil)
			data := rewriteArchive(t, valid, func(writer *zip.Writer) {
				file, err := writer.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = file.Write([]byte("bad"))
			})
			file := filepath.Join(t.TempDir(), "bad.zip")
			_ = os.WriteFile(file, data, 0600)
			if _, err := store.InstallArchive(context.Background(), file, qimu.Hash(data)); err == nil {
				t.Fatal("unsafe or undeclared archive was accepted")
			}
		})
	}
	store := storeFixture(t, nil)
	data := rewriteArchive(t, valid, func(writer *zip.Writer) {
		header := &zip.FileHeader{Name: "link"}
		header.SetMode(os.ModeSymlink | 0777)
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write([]byte("/tmp/outside"))
	})
	file := filepath.Join(t.TempDir(), "symlink.zip")
	_ = os.WriteFile(file, data, 0600)
	if _, err := store.InstallArchive(context.Background(), file, qimu.Hash(data)); err == nil {
		t.Fatal("symlink archive accepted")
	}
}

func TestArchiveDigestLimitsAndTrust(t *testing.T) {
	_, _, data, _ := packageFixture(t)
	store := storeFixture(t, nil)
	file := filepath.Join(t.TempDir(), "plugin.zip")
	_ = os.WriteFile(file, data, 0600)
	if _, err := store.InstallArchive(context.Background(), file, qimu.Hash([]byte("different"))); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("digest: %v", err)
	}
	store.limits.ExpandedBytes = 4
	if _, err := store.InstallArchive(context.Background(), file, qimu.Hash(data)); !errors.Is(err, ErrCapacity) {
		t.Fatalf("size: %v", err)
	}
	store.limits = DefaultLimits()
	store.trust = func(context.Context, Source, Manifest, Lock) error { return errors.New("publisher is not approved") }
	if _, err := store.InstallArchive(context.Background(), file, qimu.Hash(data)); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("trust: %v", err)
	}
	entries, err := os.ReadDir(store.directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected staging remained: %v %v", entries, err)
	}
}

func TestDirectorySnapshotCannotFollowLaterSourceChanges(t *testing.T) {
	_, _, data, _ := packageFixture(t)
	root := t.TempDir()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, entry.Name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := storeFixture(t, nil)
	release, err := store.InstallDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "theme.css"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := store.Acquire(context.Background(), release.Digest)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	actual, err := lease.Read(context.Background(), "theme.css")
	if err != nil || string(actual) != "body { color: blue; }\n" {
		t.Fatalf("source replaced release: %q %v", actual, err)
	}
	if _, err := store.InstallDirectory(context.Background(), root); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("drift source accepted: %v", err)
	}
}

func TestReadAndDurableProtectionPreventCollection(t *testing.T) {
	_, _, data, _ := packageFixture(t)
	retained := true
	store := storeFixture(t, func(context.Context, qimu.Digest) (bool, error) { return retained, nil })
	release := installArchive(t, store, data)
	lease, err := store.Acquire(context.Background(), release.Digest)
	if err != nil {
		t.Fatal(err)
	}
	retained = false
	if err := store.Remove(context.Background(), release.Digest); !errors.Is(err, qimu.ErrInUse) {
		t.Fatalf("live read removed: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	retained = true
	if err := store.Remove(context.Background(), release.Digest); !errors.Is(err, qimu.ErrInUse) {
		t.Fatalf("durable protection removed: %v", err)
	}
	retained = false
	if err := store.Remove(context.Background(), release.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.Read(context.Background(), "theme.css"); !errors.Is(err, qimu.ErrClosed) {
		t.Fatalf("closed lease read: %v", err)
	}
}

func TestCachedByteDriftAndManifestAliasesAreRefused(t *testing.T) {
	_, manifest, data, _ := packageFixture(t)
	store := storeFixture(t, nil)
	release := installArchive(t, store, data)
	file := filepath.Join(store.directory, string(release.Digest), "theme.css")
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("changed cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acquire(context.Background(), release.Digest); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("cache drift: %v", err)
	}
	manifest.Artifacts = []Artifact{{Path: "A/one", Digest: qimu.Hash(nil)}, {Path: "a/two", Digest: qimu.Hash(nil)}}
	manifest.Units[0].Resources = nil
	if err := manifest.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("parent alias: %v", err)
	}
}

func TestManifestDuplicateKeysAndWrongLockAreRefused(t *testing.T) {
	if _, err := ParseManifest([]byte(`{"schemaVersion":1,"schemaVersion":1,"package":"p","version":"1","artifacts":[],"units":[]}`)); err == nil {
		t.Fatal("duplicate member accepted")
	}
	_, _, data, _ := packageFixture(t)
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	var lock Lock
	for _, entry := range archive.File {
		reader, _ := entry.Open()
		body, _ := io.ReadAll(reader)
		_ = reader.Close()
		if entry.Name == ManifestName {
			manifest, err = ParseManifest(body)
		}
		if entry.Name == LockName {
			lock, err = ParseLock(body)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	lock.Release = qimu.Hash([]byte("another release"))
	if err := lock.Verify(manifest); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("wrong lock: %v", err)
	}
	body, _ := json.Marshal(manifest)
	if _, err := ParseManifest(body); err != nil {
		t.Fatal(err)
	}
}

func rewriteArchive(t *testing.T, data []byte, appendFiles func(*zip.Writer)) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	for _, entry := range reader.File {
		source, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		header := entry.FileHeader
		target, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(target, source)
		_ = source.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	appendFiles(writer)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}
