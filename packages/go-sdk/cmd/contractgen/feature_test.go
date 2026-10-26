package main

import (
	json "encoding/json/v2"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestFeatureConsumerGenerationLeavesPublisherArtifactsUntouched(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	method, err := pluginsdk.NewMethod[struct{}, string](pluginsdk.MethodConfig{Name: "acme/read", Kind: capability.Query, Target: "installation", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	data, err := featureDescriptor(method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(directory, "capability.generated.json")
	if err := os.WriteFile(source, data, 0644); err != nil {
		t.Fatal(err)
	}
	projection := filepath.Join(directory, "consumer", "contract.generated.go")
	if err := runFeature(source, "", "", featureGoOutput{Path: projection, Package: "consumer"}); err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(source)
	if err != nil || string(retained) != string(data) {
		t.Fatalf("publisher descriptor changed: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), projection, nil, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 2 {
		t.Fatalf("consumer generated publisher artifacts: %v, %v", entries, err)
	}
}

func TestFeatureGenerationCannotOverwriteDescriptorSource(t *testing.T) {
	for _, name := range []string{"methods.generated.json", "capability.generated.json"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			source := filepath.Join(directory, name)
			data := []byte(`{"authored":"retained"}`)
			if err := os.WriteFile(source, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := runFeature(source, directory, ""); err == nil {
				t.Fatal("generator overwrote its descriptor input")
			}
			retained, err := os.ReadFile(source)
			if err != nil || string(retained) != string(data) {
				t.Fatalf("source changed: %s, %v", retained, err)
			}
		})
	}
}

func featureDescriptor(methods ...capability.MethodDescriptor) ([]byte, error) {
	return json.Marshal(capability.Descriptor{Protocol: capability.Protocol, Reference: "acme/reviews", Composition: capability.Collection, Scope: "installation", Methods: methods})
}

func TestCommandRequiresOneExplicitGenerationMode(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--feature", "missing", "--model-contract", "context"},
		{"--model-contract", "context", "--out", "."},
		{"--model-contract", "input", "--go-prefix", "X"},
		{"--model-contract", "unknown"}, {"--model-contract", "selection", "extra"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("accepted ambiguous command: %v", args)
		}
	}
}

func TestFeatureGenerationRefusesSourceHardLinkBeforeWritingAnyProjection(t *testing.T) {
	method, err := pluginsdk.NewMethod[struct{}, string](pluginsdk.MethodConfig{Name: "acme/read", Kind: capability.Query, Target: "installation", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	data, err := featureDescriptor(method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source.json")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "aliased.go")
	if err := os.Link(source, target); err != nil {
		t.Fatal(err)
	}
	outputs := filepath.Join(directory, "projections")
	if err := runFeature(source, outputs, "", featureGoOutput{Path: target, Package: "consumer"}); err == nil {
		t.Fatal("overwrote source through hard link")
	}
	retained, err := os.ReadFile(source)
	if err != nil || string(retained) != string(data) {
		t.Fatalf("source changed: %v", err)
	}
	if _, err := os.Stat(outputs); !os.IsNotExist(err) {
		t.Fatalf("wrote outputs before admitting all targets: %v", err)
	}
}
