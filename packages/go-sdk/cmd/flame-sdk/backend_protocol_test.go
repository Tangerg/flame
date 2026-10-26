package main

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
)

func TestBackendProjectionIncludesProtocolFactsAcrossTheSourcePackage(t *testing.T) {
	directory := t.TempDir()
	release := filepath.Join(directory, "node-release.json")
	output := filepath.Join(directory, "protocol.json")
	if err := os.WriteFile(release, []byte(`{"version":"24.22.1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runBackendProtocol([]string{"--node-release", release, "--out", output}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Constants map[string]string `json:"constants"`
	}
	if err := json.Unmarshal(data, &projection); err != nil {
		t.Fatal(err)
	}
	if got := projection.Constants["ToolContextMetadataKey"]; got != pluginsdk.ToolContextMetadataKey {
		t.Fatalf("tool-context metadata lost its source owner: %q", got)
	}
	if err := runBackendProtocol([]string{"--source", directory, "--node-release", release, "--out", output}); err == nil {
		t.Fatal("a second checkout became a source of protocol facts")
	}
}
