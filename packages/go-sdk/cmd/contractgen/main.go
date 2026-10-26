package main

import (
	json "encoding/json/v2"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/contractgen"
	"github.com/Tangerg/flame/packages/go-sdk/plugin"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("contractgen", flag.ContinueOnError)
	feature := flags.String("feature", "", "capability descriptor file, or - for stdin")
	model := flags.String("model-contract", "", "model role: context, input or selection; writes descriptor to stdout")
	out := flags.String("out", ".", "artifact directory; empty writes only requested projections")
	ts := flags.String("ts", "", "TypeScript projection directory")
	goOut := flags.String("go-out", "", "Go projection output file")
	goPackage := flags.String("go-package", "", "Go projection package")
	goPrefix := flags.String("go-prefix", "", "Go projection exported prefix")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if (*feature == "") == (*model == "") {
		return fmt.Errorf("exactly one of feature or model-contract is required")
	}
	if *model != "" {
		var incompatible string
		flags.Visit(func(f *flag.Flag) {
			if f.Name != "model-contract" {
				incompatible = f.Name
			}
		})
		if incompatible != "" {
			return fmt.Errorf("model-contract cannot be combined with %s", incompatible)
		}
		return writeModelContract(*model)
	}
	return runFeature(*feature, *out, *ts, featureGoOutput{Path: *goOut, Package: *goPackage, Prefix: *goPrefix})
}

func writeModelContract(role string) error {
	var contract *capability.Contract
	var err error
	switch role {
	case "context":
		contract, err = plugin.ModelContextContract()
	case "input":
		contract, err = plugin.ModelInputContract()
	case "selection":
		contract, err = plugin.ModelSelectionContract()
	default:
		return fmt.Errorf("unknown model contract %q", role)
	}
	if err != nil {
		return err
	}
	return json.MarshalWrite(os.Stdout, contract.Descriptor())
}

type featureGoOutput struct{ Path, Package, Prefix string }

func runFeature(source, directory, tsDirectory string, goOutputs ...featureGoOutput) error {
	type artifact struct {
		path string
		body []byte
	}
	var outputs []artifact
	appendOutput := func(path string, body []byte, err error) error {
		if err != nil {
			return err
		}
		outputs = append(outputs, artifact{path, body})
		return nil
	}
	var input []byte
	var err error
	if source == "-" {
		input, err = io.ReadAll(os.Stdin)
	} else {
		input, err = os.ReadFile(source)
	}
	if err != nil {
		return err
	}
	feature, err := contractgen.BuildFeature(input)
	if err != nil {
		return err
	}
	if directory != "" {
		for _, output := range []struct {
			name   string
			encode func() ([]byte, error)
		}{
			{"schema.json", feature.SchemaJSON},
			{"methods.generated.json", feature.MethodsJSON},
			{"capability.generated.json", feature.CapabilityJSON},
		} {
			body, err := output.encode()
			if err := appendOutput(filepath.Join(directory, output.name), body, err); err != nil {
				return err
			}
		}
		outputs = append(outputs, artifact{filepath.Join(directory, "API_REFERENCE.md"), []byte(feature.Reference())})
	}
	if tsDirectory != "" {
		outputs = append(outputs, artifact{filepath.Join(tsDirectory, "methods.generated.ts"), []byte(feature.TypeScript())})
	}
	for _, output := range goOutputs {
		if output.Path == "" {
			continue
		}
		body, err := feature.Go(contractgen.GoOptions{Package: output.Package, Prefix: output.Prefix})
		if err := appendOutput(output.Path, body, err); err != nil {
			return err
		}
	}
	// Admit every target before writing; a hard link is the same source despite a different path.
	targets := map[string]bool{}
	for _, output := range outputs {
		target, err := filepath.Abs(output.path)
		if err != nil {
			return err
		}
		if targets[target] {
			return fmt.Errorf("feature contracts: duplicate output %s", target)
		}
		targets[target] = true
		if source != "-" {
			inputPath, err := filepath.Abs(source)
			if err != nil {
				return err
			}
			original, err := os.Stat(source)
			if err != nil {
				return err
			}
			existing, err := os.Stat(target)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if inputPath == target || err == nil && os.SameFile(original, existing) {
				return fmt.Errorf("feature contracts: output would overwrite descriptor input %s", source)
			}
		}
	}
	for _, output := range outputs {
		if err := os.MkdirAll(filepath.Dir(output.path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(output.path, output.body, 0644); err != nil {
			return err
		}
	}
	return nil
}
