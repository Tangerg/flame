package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompiledBackendProjectionDoesNotRequireSDKSources(t *testing.T) {
	directory := t.TempDir()
	release := filepath.Join(directory, "node-release.json")
	expected := filepath.Join(directory, "expected.json")
	if err := os.WriteFile(release, []byte(`{"version":"24.22.1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runBackendProtocol([]string{"--node-release", release, "--out", expected}); err != nil {
		t.Fatal(err)
	}
	name := "flame-sdk"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable := filepath.Join(directory, name)
	// Trimming source paths and running outside the checkout prevents a compiled
	// tool from substituting files on disk for its own protocol version.
	build := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-trimpath", "-o", executable, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build standalone protocol tool: %v\n%s", err, output)
	}
	actual := filepath.Join(directory, "actual.json")
	command := exec.CommandContext(t.Context(), executable, "backend-protocol", "--node-release", release, "--out", actual)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("project without SDK sources: %v\n%s", err, output)
	}
	want, err := os.ReadFile(expected)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(actual)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("standalone protocol projection differs from its compiled SDK")
	}
	if err := runBackendProtocol([]string{"--source", directory, "--node-release", release, "--out", actual}); err == nil {
		t.Fatal("a second checkout became a source of protocol facts")
	}
}
