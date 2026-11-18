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
		return fmt.Errorf("usage: goapi catalog --repository path --out file")
	}
	if args[0] != "catalog" {
		return fmt.Errorf("unknown goapi command %q", args[0])
	}
	flags := flag.NewFlagSet("goapi catalog", flag.ContinueOnError)
	repository := flags.String("repository", "", "repository source root")
	out := flags.String("out", "", "catalog output file")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *repository == "" || *out == "" {
		return fmt.Errorf("repository and out are required and positional arguments are not accepted")
	}
	absolute, err := filepath.Abs(*repository)
	if err != nil {
		return err
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
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
