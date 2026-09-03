package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildIDFromFileUsesSHA256ContentDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame")
	content := []byte("runtime executable")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write executable fixture: %v", err)
	}

	got, err := buildIDFromFile(path)
	if err != nil {
		t.Fatalf("buildIDFromFile: %v", err)
	}
	sum := sha256.Sum256(content)
	want := "sha256:" + hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("BuildID = %q, want %q", got, want)
	}
}

func TestBuildIDFromFileReportsReadFailure(t *testing.T) {
	_, err := buildIDFromFile(filepath.Join(t.TempDir(), "missing"))
	if err == nil || !strings.Contains(err.Error(), "build identity") {
		t.Fatalf("buildIDFromFile error = %v, want contextual read failure", err)
	}
}

func TestBuildIDFromOpenedFileRejectsGrowthAfterOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("before and after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := buildIDFromOpenedFile(path, file, before); err == nil || !strings.Contains(err.Error(), "changed while hashing") {
		t.Fatalf("growing executable error = %v", err)
	}
}

func TestBuildIDFromOpenedFileRejectsPathReplacement(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "flame")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(directory, "replacement")
	if err := os.WriteFile(replacement, []byte("after!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, err := buildIDFromOpenedFile(path, file, before); err == nil || !strings.Contains(err.Error(), "changed while hashing") {
		t.Fatalf("replaced executable error = %v", err)
	}
}
