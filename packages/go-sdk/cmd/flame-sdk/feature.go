package main

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/frame"
	"github.com/Tangerg/frame/contractgen"
	"github.com/Tangerg/frame/value"
)

func runFeature(args []string) error {
	flags := flag.NewFlagSet("flame-sdk feature", flag.ContinueOnError)
	source := flags.String("feature", "", "published Flame capability descriptor, or - for stdin")
	out := flags.String("out", ".", "artifact directory; empty writes only requested projections")
	ts := flags.String("ts", "", "TypeScript projection directory")
	goOut := flags.String("go-out", "", "Go projection file")
	goPackage := flags.String("go-package", "", "Go projection package")
	goPrefix := flags.String("go-prefix", "", "Go exported prefix")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *source == "" {
		return errors.New("one feature descriptor is required")
	}
	reader := os.Stdin
	if *source != "-" {
		file, err := os.Open(*source)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, int64(frame.CurrentPortableProfile().Limits.MaxDescriptorBytes)+1))
	if err != nil {
		return err
	}
	capability, err := contract.ParseCapability(data)
	if err != nil {
		return err
	}
	return generateFeature(capability, *out, *ts, contractgen.GoOutput{Path: *goOut, Package: *goPackage, Prefix: *goPrefix})
}

type featureArtifact struct{ source, target string }

// Frame owns schema and language projections. Flame supplies readable definition
// labels and restores the original contract with its composition and target policy.
func generateFeature(capability *contract.Capability, directory, tsDirectory string, goOutput contractgen.GoOutput) error {
	stage, err := os.MkdirTemp("", "flame-feature-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	projection, err := projectFeatureNames(capability.Service().Descriptor())
	if err != nil {
		return err
	}
	neutral, err := value.EncodeJSON(projection)
	if err != nil {
		return err
	}
	source := filepath.Join(stage, "service.json")
	if err := os.WriteFile(source, neutral.Bytes(), 0o600); err != nil {
		return err
	}
	staged := stageFeatureOutputs(stage, directory, tsDirectory, goOutput)
	if err := contractgen.Generate(staged.generateConfig(source)); err != nil {
		return err
	}
	if err := staged.writeCapabilityProjections(capability, projection); err != nil {
		return err
	}
	return publishFeatureArtifacts(staged.artifacts)
}

// stagedFeature generates every requested output in a scratch directory first,
// so a failed generation publishes nothing.
type stagedFeature struct {
	directory, typescript string
	goOutput              contractgen.GoOutput
	artifacts             []featureArtifact
}

func stageFeatureOutputs(stage, directory, tsDirectory string, goOutput contractgen.GoOutput) stagedFeature {
	staged := stagedFeature{goOutput: goOutput}
	if directory != "" {
		staged.directory = filepath.Join(stage, "contracts")
		for _, name := range []string{"schema.json", "methods.generated.json", "capability.generated.json", "API_REFERENCE.md"} {
			staged.artifacts = append(staged.artifacts, featureArtifact{filepath.Join(staged.directory, name), filepath.Join(directory, name)})
		}
	}
	if tsDirectory != "" {
		staged.typescript = filepath.Join(stage, "typescript")
		staged.artifacts = append(staged.artifacts, featureArtifact{filepath.Join(staged.typescript, "methods.generated.ts"), filepath.Join(tsDirectory, "methods.generated.ts")})
	}
	if goOutput.Path != "" {
		staged.goOutput.Path = filepath.Join(stage, "methods.generated.go")
		staged.artifacts = append(staged.artifacts, featureArtifact{staged.goOutput.Path, goOutput.Path})
	}
	return staged
}

func (s stagedFeature) generateConfig(source string) contractgen.GenerateConfig {
	config := contractgen.GenerateConfig{Source: source, ArtifactDirectory: s.directory, TypeScriptDirectory: s.typescript}
	if s.goOutput.Path != "" {
		config.Go = []contractgen.GoOutput{s.goOutput}
	}
	return config
}

func (s stagedFeature) writeCapabilityProjections(capability *contract.Capability, projection frame.CapabilityDescriptor) error {
	descriptor, err := value.EncodeJSON(capability.Descriptor())
	if err != nil {
		return err
	}
	if s.directory != "" {
		pretty, err := json.Marshal(capability.Descriptor(), json.Deterministic(true), jsontext.WithIndent("  "))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(s.directory, "capability.generated.json"), append(pretty, '\n'), 0o600); err != nil {
			return err
		}
	}
	if s.typescript != "" {
		if err := writeFeatureTypeScript(capability, filepath.Join(s.typescript, "methods.generated.ts")); err != nil {
			return err
		}
	}
	if s.goOutput.Path != "" {
		return writeFeatureGo(projection, descriptor, s.goOutput.Path)
	}
	return nil
}

func writeFeatureTypeScript(capability *contract.Capability, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	d := capability.Descriptor()
	metadata, err := json.Marshal(struct {
		Protocol    string               `json:"protocol"`
		Reference   string               `json:"reference"`
		Composition contract.Composition `json:"composition"`
		Scope       string               `json:"scope"`
		Contract    string               `json:"contract"`
	}{d.Protocol, d.Reference, d.Composition, d.Scope, capability.Identity().Digest().String()}, json.Deterministic(true))
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	changed := 0
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "export const CAPABILITY = "):
			lines[i] = "export const CAPABILITY = " + string(metadata) + " as const;"
			changed++
		case strings.HasPrefix(line, "export const CAPABILITY_JSON = "):
			lines[i] = "export const CAPABILITY_JSON = " + contractgen.TypeScriptStringLiteral(capability.Identity().String()) + ";"
			changed++
		}
	}
	if changed != 2 {
		return errors.New("feature contracts: Frame TypeScript identity projection is missing")
	}
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return err
	}
	return nil
}

func writeFeatureGo(projection frame.CapabilityDescriptor, descriptor value.JSON, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	originalDescriptor, err := json.Marshal(projection, json.Deterministic(true))
	if err != nil {
		return err
	}
	original := []byte(strconv.Quote(string(originalDescriptor)))
	if bytes.Count(data, original) != 1 {
		return errors.New("feature contracts: Frame Go identity projection is missing")
	}
	data = bytes.Replace(data, original, []byte(strconv.Quote(descriptor.String())), 1)
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return err
	}
	return nil
}

func publishFeatureArtifacts(artifacts []featureArtifact) error {
	seen := map[string]bool{}
	for _, output := range artifacts {
		absolute, err := filepath.Abs(output.target)
		if err != nil {
			return err
		}
		if seen[absolute] {
			return fmt.Errorf("feature contracts: duplicate output %s", output.target)
		}
		seen[absolute] = true
		if info, err := os.Lstat(output.target); err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("feature contracts: output is not a regular file: %s", output.target)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, output := range artifacts {
		body, err := os.ReadFile(output.source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(output.target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(output.target, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
