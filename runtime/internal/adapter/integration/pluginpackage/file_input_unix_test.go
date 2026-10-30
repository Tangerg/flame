//go:build unix

package pluginpackage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"golang.org/x/sys/unix"
)

func TestPackagedCommandRefusesFIFOWithoutBlocking(t *testing.T) {
	directory := t.TempDir()
	pipe := filepath.Join(directory, "backend")
	if err := unix.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	server := plugin.Server{Name: testsupport.ServerName("backend"), Transport: mcpserver.TransportStdio, Command: "./backend"}
	err = fifoFailure(t, pipe, func() error { return inspectServerFiles(root, server) })
	if !errors.Is(err, plugin.ErrInvalid) || !errors.Is(err, filesystem.ErrNotRegular) {
		t.Fatalf("special packaged command lost its admission failure: %v", err)
	}
}

func TestPackageSourceRefusesFIFOWithoutBlocking(t *testing.T) {
	releases := testReleases(t)
	pipe := filepath.Join(t.TempDir(), "package.zip")
	if err := unix.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	err := fifoFailure(t, pipe, func() error {
		candidate, err := releases.Materialize(t.Context(), pipe)
		if candidate != nil {
			err = errors.Join(err, candidate.Discard())
		}
		return err
	})
	if !errors.Is(err, plugin.ErrInvalid) || !errors.Is(err, filesystem.ErrNotRegular) {
		t.Fatalf("special package source lost its admission failure: %v", err)
	}
}

func TestVerifiedResourceRefusesReplacementFIFOWithoutBlocking(t *testing.T) {
	releases := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "notes.txt": "admitted resource"})
	release, err := publishPackage(t.Context(), releases, source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	root, err := releases.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	pipe := filepath.Join(root, "notes.txt")
	if err := os.Remove(pipe); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	err = fifoFailure(t, pipe, func() error {
		content, err := releases.readResource(t.Context(), installation, "notes.txt", maxPackageBytes)
		if len(content) != 0 {
			return errors.New("special node returned resource content")
		}
		return err
	})
	if !errors.Is(err, distribution.ErrInvalid) {
		t.Fatalf("replacement node lost its file-kind failure: %v", err)
	}
}

// A failed blocking implementation is unblocked before the test returns, so
// regression verification cannot strand a reader or its temporary directory.
func fifoFailure(t *testing.T, pipe string, operation func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- operation() }()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Error("package input blocked on a FIFO")
	}
	file, err := unix.Open(pipe, unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Close(file); err != nil {
			t.Error(err)
		}
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("package input did not retire after the FIFO was unblocked")
		return nil
	}
}
