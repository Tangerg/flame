package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGeneratedSamplesReplaceThePreviousProjection(t *testing.T) {
	directory := t.TempDir()
	source := os.DirFS(samplesDir)
	if err := os.WriteFile(filepath.Join(directory, "retired.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSamples(source, directory); err != nil {
		t.Fatal(err)
	}
	want, err := fs.Glob(source, "*.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := fs.Glob(os.DirFS(directory), "*.json")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("generated samples = %v, want current source files %v", got, want)
	}
	for _, name := range want {
		expected, err := fs.ReadFile(source, name)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(actual, expected) {
			t.Fatalf("generated sample %q differs from its source", name)
		}
	}
}
