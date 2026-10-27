package main

import (
	"bytes"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
)

func authorSource(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "plugin.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"example.documents","version":"1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "document.txt"), []byte("original document"), 0600); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestCommandPublishesOneNeutralIdentityWithoutExecutingCode(t *testing.T) {
	source := authorSource(t)
	archive := filepath.Join(t.TempDir(), "plugin.zip")
	var output bytes.Buffer
	if err := run([]string{"--source", source, "--archive", archive}, &output); err != nil {
		t.Fatal(err)
	}
	var identity struct {
		Release qimu.Digest `json:"release"`
	}
	if err := json.Unmarshal(output.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := distribution.VerifyArchive(t.Context(), archive, qimu.Hash(data), distribution.DefaultLimits())
	if err != nil || verified.Digest != identity.Release || len(verified.Manifest.Units) != 0 {
		t.Fatalf("neutral release projection: %+v, %v", verified, err)
	}
	if err := run([]string{"--source", source, "--archive", archive}, &output); err == nil {
		t.Fatal("a completed release was overwritten")
	}
	actual, err := os.ReadFile(archive)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("original archive changed after refusal: %v", err)
	}
}

func TestCommandRejectsOutputAliasedIntoItsSource(t *testing.T) {
	source := authorSource(t)
	alias := filepath.Join(t.TempDir(), "source-alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(alias, "self.zip")
	var output bytes.Buffer
	if err := run([]string{"--source", source, "--archive", archive}, &output); err == nil {
		t.Fatal("archive could become one of its own source artifacts")
	}
	if _, err := os.Stat(filepath.Join(source, "self.zip")); !os.IsNotExist(err) {
		t.Fatalf("refused output left an artifact: %v", err)
	}
}
