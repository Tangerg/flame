package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	repository := flag.String("repository", "", "Git source repository")
	revision := flag.String("revision", "", "frozen commit")
	output := flag.String("output", "", "file proxy directory")
	modulePath := flag.String("module", "", "module path")
	subdir := flag.String("subdir", "", "module source directory")
	version := flag.String("version", "", "immutable candidate version")
	flag.Parse()
	if *repository == "" || *revision == "" || *output == "" || *modulePath == "" || *version == "" {
		return errors.New("module proxy: repository, revision, output, module and version are required")
	}
	if err := module.Check(*modulePath, *version); err != nil {
		return err
	}
	escaped, err := module.EscapePath(*modulePath)
	if err != nil {
		return err
	}
	escapedVersion, err := module.EscapeVersion(*version)
	if err != nil {
		return err
	}
	directory := filepath.Join(*output, escaped, "@v")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	var archive bytes.Buffer
	if err := modzip.CreateFromVCS(&archive, module.Version{Path: *modulePath, Version: *version}, *repository, *revision, *subdir); err != nil {
		return err
	}
	source := "go.mod"
	if *subdir != "" {
		source = filepath.ToSlash(filepath.Join(*subdir, "go.mod"))
	}
	read := func(args ...string) ([]byte, error) {
		command := exec.Command("git", args...)
		command.Dir = *repository
		return command.Output()
	}
	mod, err := read("show", *revision+":"+source)
	if err != nil {
		return err
	}
	timestamp, err := read("show", "-s", "--format=%cI", *revision)
	if err != nil {
		return err
	}
	info, err := json.Marshal(struct{ Version, Time string }{*version, strings.TrimSpace(string(timestamp))})
	if err != nil {
		return err
	}
	for _, artifact := range []struct {
		suffix string
		data   []byte
	}{{".zip", archive.Bytes()}, {".mod", mod}, {".info", append(info, '\n')}} {
		name := filepath.Join(directory, escapedVersion+artifact.suffix)
		if err := publishImmutable(name, artifact.data); err != nil {
			return err
		}
	}
	if _, err := modzip.CheckZip(module.Version{Path: *modulePath, Version: *version}, filepath.Join(directory, escapedVersion+".zip")); err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var versions []string
	for _, entry := range entries {
		if version, found := strings.CutSuffix(entry.Name(), ".info"); found {
			versions = append(versions, version)
		}
	}
	slices.Sort(versions)
	return os.WriteFile(filepath.Join(directory, "list"), []byte(strings.Join(versions, "\n")+"\n"), 0o644)
}

// A version becomes visible atomically and can never replace different bytes.
func publishImmutable(name string, data []byte) error {
	staged, err := os.CreateTemp(filepath.Dir(name), ".module-*")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	if _, err := staged.Write(data); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Chmod(0644); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Link(staged.Name(), name); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	original, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if !bytes.Equal(original, data) {
		return fmt.Errorf("module proxy: immutable artifact differs: %s", name)
	}
	return nil
}
