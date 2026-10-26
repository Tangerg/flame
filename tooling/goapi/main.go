package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fail(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: goapi <catalog|consumers> --repository path")
	}
	command := args[0]
	flags := flag.NewFlagSet("goapi "+command, flag.ContinueOnError)
	repository := flags.String("repository", "", "repository source root")
	out := flags.String("out", "", "catalog output file")
	manifest := flags.String("manifest", "", "Runtime manifest containing generated Go method names")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *repository == "" {
		return fmt.Errorf("repository is required and positional arguments are not accepted")
	}
	absolute, err := filepath.Abs(*repository)
	if err != nil {
		return err
	}
	switch command {
	case "catalog":
		if *out == "" || *manifest != "" {
			return fmt.Errorf("catalog requires out and does not accept manifest")
		}
		api, err := loadPublicGoAPI(absolute)
		if err != nil {
			return err
		}
		data, err := json.Marshal(api, jsontext.WithIndent("  "), json.Deterministic(true))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
			return err
		}
		return os.WriteFile(*out, append(data, '\n'), 0644)
	case "consumers":
		if *manifest == "" || *out != "" {
			return fmt.Errorf("consumers requires manifest and writes to stdout")
		}
		methods, err := manifestGoNames(*manifest)
		if err != nil {
			return err
		}
		return json.MarshalWrite(os.Stdout, collect(absolute, methods), json.Deterministic(true))
	default:
		return fmt.Errorf("unknown goapi command %q", command)
	}
}

func manifestGoNames(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Methods []struct {
			Name   string `json:"name"`
			GoName string `json:"goName"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	methods := map[string]string{}
	names := map[string]bool{}
	for _, method := range manifest.Methods {
		if method.Name == "" || names[method.Name] {
			return nil, fmt.Errorf("manifest has empty or duplicate method %q", method.Name)
		}
		names[method.Name] = true
		if method.GoName == "" {
			continue
		}
		if _, exists := methods[method.GoName]; exists {
			return nil, fmt.Errorf("manifest has duplicate Go method %q", method.GoName)
		}
		methods[method.GoName] = method.Name
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("manifest has no Go method projections")
	}
	return methods, nil
}
