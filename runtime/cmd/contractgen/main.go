// Command contractgen writes the machine-readable projection of the Contract
// Registry.
//
// It lives outside the runtime ring architecture: it reads the delivery ring's
// method and shape specs together with the application ring's system invariants,
// which no runtime component may do. A code generator is not in the runtime's
// import graph, so the layering rule that constrains that graph does not apply to
// it.
//
// Run it through `go generate ./...`. CI's drift gate reruns it and fails on a
// worktree diff: that is the only mechanism that notices
// when the code and the published contract stop agreeing.
package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/contract/samples"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

// The two union types the manifest projects extra policy from. Kept as vars so
// the projection compares identity rather than matching on a name string.
var (
	streamEventType  = reflect.TypeFor[protocol.StreamEvent]()
	runtimeEventType = reflect.TypeFor[protocol.RuntimeEvent]()
)

func main() {
	out := flag.String("out", ".", "directory for generated artifacts; empty writes only requested projections")
	validators := flag.String("validators", "", "directory the generated Go wire validator is written to; skipped when empty")
	ts := flag.String("ts", "", "directory the TypeScript protocol binding is written to; skipped when empty")
	localBinding := flag.String("go-local", "", "local typed Go binding output directory")
	remoteBinding := flag.String("go-client", "", "remote typed Go client output directory")
	flag.Parse()

	err := writeGoBindings(*localBinding, *remoteBinding)
	if err == nil {
		err = run(*out, *validators, *ts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
}

func run(dir, validatorDir, tsDir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	// Built once: every artifact must describe the same registry snapshot, and a
	// second build would let them disagree if anything about it were not pure.
	registry, shapes := delivery.Contract(), dispatch.WireShapes()
	walked, err := walkWireTypes(registry, shapes)
	if err != nil {
		return err
	}
	built, err := build(walked)
	if err != nil {
		return err
	}
	// Generated validators are exported protocol methods, so they must exist on
	// disk before public-Go-API discovery loads that package. Loading first made a
	// shape gain require two generator runs: the first wrote ValidateWire, while
	// only the second published that method in go-api.json.
	if validatorDir != "" {
		path := filepath.Join(validatorDir, validatorFile)
		if err := os.WriteFile(path, []byte(newValidators(registry, shapes, walked)), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	for _, artifact := range []struct {
		name    string
		content any
	}{
		{"manifest.json", built},
		{"schema.json", newBundle(walked)},
		{"openrpc.json", newOpenRPC(registry, shapes, walked)},
	} {
		if err := writeJSON(filepath.Join(dir, artifact.name), artifact.content); err != nil {
			return err
		}
	}
	path := filepath.Join(dir, "API_REFERENCE.md")
	if err := os.WriteFile(path, []byte(reference(built)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if tsDir == "" {
		return nil
	}
	generated := filepath.Join(tsDir, "src", "generated")
	testing := filepath.Join(tsDir, "testing", "generated")
	for _, directory := range []string{generated, filepath.Join(testing, "samples")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	// What a TypeScript client needs to hold the protocol: the shapes, the checks that say an
	// arriving frame really is one of them, the method surface that carries them, and
	// which hand-written fixture proves which shape.
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
		path = filepath.Join(directory, artifact.name)
		if err := os.WriteFile(path, []byte(artifact.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return writeSamples(filepath.Join(testing, "samples"))
}

func writeSamples(directory string) error {
	files, err := fs.Glob(samples.Files, "*.json")
	if err != nil {
		return err
	}
	retained := make(map[string]bool, len(files))
	for _, name := range files {
		encoded, err := samples.Files.ReadFile(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, name), encoded, 0o644); err != nil {
			return err
		}
		retained[name] = true
	}
	previous, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range previous {
		if !retained[filepath.Base(path)] {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeJSON writes an artifact indented with a trailing newline, so a reviewer
// reads a diff rather than one long line — the drift gate's output is meant to be
// read by a person, not just compared.
func writeJSON(path string, content any) error {
	encoded, err := json.Marshal(content, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
