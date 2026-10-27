package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
)

func TestPackageAndVerifyPreserveTheExactArtifactAndRefusePartialOutput(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "theme.css"), []byte("body { color: blue }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := distribution.Manifest{
		SchemaVersion: 1, Package: "example.theme", Version: "1.0.0",
		Artifacts: []distribution.Artifact{{Path: "theme.css"}},
		Units:     []distribution.Unit{{ID: "theme", Profile: qimu.StaticResource, Resources: []string{"theme.css"}}},
	}
	writeManifest := func() {
		t.Helper()
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, distribution.ManifestName), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest()
	archive := filepath.Join(t.TempDir(), "theme.zip")
	var output bytes.Buffer
	if err := runPackage([]string{"--source", source, "--out", archive}, &output); err != nil {
		t.Fatal(err)
	}
	var report struct {
		ArchiveDigest qimu.Digest       `json:"archiveDigest"`
		Lock          distribution.Lock `json:"lock"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil || report.ArchiveDigest != qimu.Hash(data) {
		t.Fatalf("archive receipt: %v, %v", report.ArchiveDigest, err)
	}
	output.Reset()
	if err := runVerify([]string{"--archive", archive, "--digest", report.ArchiveDigest.String()}, &output); err != nil {
		t.Fatal(err)
	}
	var verified distribution.Release
	if err := json.Unmarshal(output.Bytes(), &verified); err != nil || verified.Digest != report.Lock.Release {
		t.Fatalf("verified identity: %v, %v", verified.Digest, err)
	}
	if err := runPackage([]string{"--source", source, "--out", archive}, &output); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrote an existing artifact: %v", err)
	}
	if actual, err := os.ReadFile(archive); err != nil || !bytes.Equal(actual, data) {
		t.Fatal("rejected publication changed the original archive")
	}
	if err := os.WriteFile(archive, append(data, 'x'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runVerify([]string{"--archive", archive, "--digest", report.ArchiveDigest.String()}, &output); err == nil {
		t.Fatal("changed bytes retained the original archive identity")
	}
	manifest.Artifacts = append(manifest.Artifacts, distribution.Artifact{Path: "missing.css"})
	writeManifest()
	incomplete := filepath.Join(t.TempDir(), "incomplete.zip")
	if err := runPackage([]string{"--source", source, "--out", incomplete}, &output); err == nil {
		t.Fatal("missing declared artifact was accepted")
	}
	if _, err := os.Stat(incomplete); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed publication left an apparent archive: %v", err)
	}
}
