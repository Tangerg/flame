package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

type selectedExport string

func (f selectedExport) ChooseFileDestination(string) (string, error) { return string(f), nil }

func TestStreamedExportPublishesOnlyCompleteBytesFromTheOriginalTransfer(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "saved")
	if err := os.WriteFile(dest, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	var s exportTransfer
	id, err := s.begin("saved", 2, selectedExport(dest))
	if err != nil {
		t.Fatal(err)
	}
	data := base64.StdEncoding.EncodeToString([]byte("a"))
	if s.appendChunk("wrong", 0, data) == nil || s.appendChunk(id, 1, data) == nil {
		t.Fatal("accepted another chunk identity")
	}
	if err := s.appendChunk(id, 0, data); err != nil {
		t.Fatal(err)
	}
	if s.finish(id) == nil {
		t.Fatal("published incomplete export")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "old" {
		t.Fatalf("old destination = %q, %v", got, err)
	}
	id, err = s.begin("saved", 2, selectedExport(dest))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.appendChunk(id, 0, data); err != nil {
		t.Fatal(err)
	}
	if err := s.appendChunk(id, 1, data); err != nil {
		t.Fatal(err)
	}
	if err := s.finish(id); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(dest)
	if err != nil || string(got) != "aa" {
		t.Fatalf("destination = %q, %v", got, err)
	}
}

type exportPicker func() (string, error)

func (f exportPicker) ChooseFileDestination(string) (string, error) { return f() }

func TestNavigationRetiresAnOpenNativeExportPicker(t *testing.T) {
	var transfer exportTransfer
	started, release := make(chan struct{}), make(chan struct{})
	destination := filepath.Join(t.TempDir(), "stale")
	result := make(chan string, 1)
	go func() {
		id, err := transfer.begin("stale", 1, exportPicker(func() (string, error) { close(started); <-release; return destination, nil }))
		if err != nil {
			t.Errorf("retired picker: %v", err)
		}
		result <- id
	}()
	<-started
	transfer.abort()
	close(release)
	if id := <-result; id != "" {
		t.Fatal("retired document retained a write handle")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".flame-export-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("retired picker staged %v: %v", matches, err)
	}
}
