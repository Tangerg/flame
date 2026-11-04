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
	"github.com/Tangerg/frame/portable"
	"github.com/Tangerg/frame/portable/contractgen"
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
	data, err := io.ReadAll(io.LimitReader(reader, int64(portable.ProfileLimits().MaxDescriptorBytes)+1))
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

// Frame owns Schema and language projections. Flame adds only the composition
// and target policy omitted from Frame's transport-neutral service identity.
func generateFeature(capability *contract.Capability, directory, tsDirectory string, goOutput contractgen.GoOutput) error {
	stage, err := os.MkdirTemp("", "flame-feature-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	neutral, err := value.EncodeJSON(capability.Service().Descriptor())
	if err != nil {
		return err
	}
	source := filepath.Join(stage, "service.json")
	if err := os.WriteFile(source, neutral.Bytes(), 0o600); err != nil {
		return err
	}
	var artifacts []featureArtifact
	var generatedDirectory, generatedTS string
	if directory != "" {
		generatedDirectory = filepath.Join(stage, "contracts")
		for _, name := range []string{"schema.json", "methods.generated.json", "capability.generated.json", "API_REFERENCE.md"} {
			artifacts = append(artifacts, featureArtifact{filepath.Join(generatedDirectory, name), filepath.Join(directory, name)})
		}
	}
	if tsDirectory != "" {
		generatedTS = filepath.Join(stage, "typescript")
		artifacts = append(artifacts, featureArtifact{filepath.Join(generatedTS, "methods.generated.ts"), filepath.Join(tsDirectory, "methods.generated.ts")})
	}
	generatedGo := goOutput
	if goOutput.Path != "" {
		generatedGo.Path = filepath.Join(stage, "methods.generated.go")
		artifacts = append(artifacts, featureArtifact{generatedGo.Path, goOutput.Path})
	}
	if err := contractgen.Generate(source, generatedDirectory, generatedTS, generatedGo); err != nil {
		return err
	}
	descriptor, err := value.EncodeJSON(capability.Descriptor())
	if err != nil {
		return err
	}
	if generatedDirectory != "" {
		pretty, err := json.Marshal(capability.Descriptor(), json.Deterministic(true), jsontext.WithIndent("  "))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(generatedDirectory, "capability.generated.json"), append(pretty, '\n'), 0o600); err != nil {
			return err
		}
	}
	if generatedTS != "" {
		if err := writeFeatureTypeScript(capability, filepath.Join(generatedTS, "methods.generated.ts")); err != nil {
			return err
		}
	}
	if generatedGo.Path != "" {
		if err := writeFeatureGo(capability, descriptor, generatedGo.Path); err != nil {
			return err
		}
	}
	return publishFeatureArtifacts(artifacts)
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

func writeFeatureGo(capability *contract.Capability, descriptor value.JSON, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	originalDescriptor, err := json.Marshal(capability.Service().Descriptor(), json.Deterministic(true))
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
