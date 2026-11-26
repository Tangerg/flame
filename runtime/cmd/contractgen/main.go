package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

const (
	samplesDirectory = "samples"
	schemaBundleFile = "schema.json"
)

var (
	streamEventType  = reflect.TypeFor[protocol.StreamEvent]()
	runtimeEventType = reflect.TypeFor[protocol.RuntimeEvent]()
)

func main() {
	out := flag.String("out", ".", "directory for generated artifacts; empty writes only requested projections")
	validator := flag.String("validator", "", "generated Go wire validator file")
	ts := flag.String("ts", "", "directory the TypeScript protocol binding is written to; skipped when empty")
	clientValues := flag.String("client-values", "", "file for consumer-neutral client scalar contracts; skipped when empty")
	toolCategoryFixture := flag.String("tool-category-fixture", "", "file for the built-in tool categories used by client fixtures")
	localBinding := flag.String("go-local", "", "local typed Go binding output directory")
	remoteBinding := flag.String("go-client", "", "remote typed Go client output directory")
	sampleSource := flag.String("samples", "", "directory of canonical Runtime JSON samples to refresh and copy; required with -ts")
	flag.Parse()

	err := writeGoBindings(*localBinding, *remoteBinding)
	if err == nil {
		err = run(*out, *validator, *ts, *sampleSource, *clientValues)
	}
	if err == nil && *toolCategoryFixture != "" {
		err = writeToolCategoryFixture(*toolCategoryFixture)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
}

func run(dir, validator, tsDir, sampleDir, clientValuesFile string) error {
	if tsDir != "" && sampleDir == "" {
		return fmt.Errorf("canonical sample directory is required with TypeScript output")
	}
	registry, shapes := delivery.Contract(), dispatch.WireShapes()
	walked := walkWireTypes(registry, shapes)
	built, err := build(walked)
	if err != nil {
		return err
	}
	if validator != "" {
		if err := os.WriteFile(validator, []byte(newValidators(registry, shapes, walked)), filesystem.SharedFileMode); err != nil {
			return fmt.Errorf("write %s: %w", validator, err)
		}
	}
	if dir != "" {
		if err := writeContractArtifacts(dir, registry, shapes, walked, built); err != nil {
			return err
		}
	}
	if clientValuesFile != "" {
		if err := writeClientValues(clientValuesFile, walked); err != nil {
			return err
		}
	}
	if tsDir == "" {
		return nil
	}
	return writeTypeScriptBinding(tsDir, sampleDir, registry, shapes, walked, built)
}

func writeContractArtifacts(dir string, registry *delivery.Registry, shapes *dispatch.Shapes, walked *schemaSet, built manifest) error {
	if err := os.MkdirAll(dir, filesystem.SharedDirectoryMode); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	for _, artifact := range []struct {
		name    string
		content any
	}{
		{"manifest.json", built},
		{schemaBundleFile, newBundle(walked)},
		{"openrpc.json", newOpenRPC(registry, shapes, walked)},
	} {
		if err := writeJSON(filepath.Join(dir, artifact.name), artifact.content); err != nil {
			return err
		}
	}
	path := filepath.Join(dir, "API_REFERENCE.md")
	if err := os.WriteFile(path, []byte(reference(built)), filesystem.SharedFileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeClientValues(file string, walked *schemaSet) error {
	values, err := newClientValues(walked)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), filesystem.SharedDirectoryMode); err != nil {
		return fmt.Errorf("create client scalar directory: %w", err)
	}
	if err := os.WriteFile(file, []byte(values), filesystem.SharedFileMode); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}

func writeTypeScriptBinding(tsDir, sampleDir string, registry *delivery.Registry, shapes *dispatch.Shapes, walked *schemaSet, built manifest) error {
	generated := filepath.Join(tsDir, "src", "generated")
	testing := filepath.Join(tsDir, "testing", "generated")
	for _, directory := range []string{generated, filepath.Join(testing, samplesDirectory)} {
		if err := os.MkdirAll(directory, filesystem.SharedDirectoryMode); err != nil {
			return err
		}
	}
	for _, artifact := range []struct{ name, content string }{
		{tsFileName, newTypeScript(walked, built.Notifications)},
		{tsValidatorFileName, newWireChecks(registry, shapes, walked)},
		{tsMethodsFileName, newWireMethods(registry, walked)},
		{tsSamplesFileName, newWireSamples()},
	} {
		directory := generated
		if artifact.name == tsSamplesFileName {
			directory = testing
		}
		path := filepath.Join(directory, artifact.name)
		if err := os.WriteFile(path, []byte(artifact.content), filesystem.SharedFileMode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return writeSamples(sampleDir, filepath.Join(testing, samplesDirectory))
}

func writeJSON(path string, content any) error {
	encoded, err := json.Marshal(content, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), filesystem.SharedFileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
