package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	commit, err := archiveModule(&archive, module.Version{Path: *modulePath, Version: *version}, *repository, *revision, *subdir)
	if err != nil {
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
	mod, err := read("show", commit+":"+source)
	if err != nil {
		return err
	}
	timestamp, err := read("show", "-s", "--format=%cI", commit)
	if err != nil {
		return err
	}
	info, err := versionInfo(filepath.Join(directory, escapedVersion+".info"), *version, strings.TrimSpace(string(timestamp)))
	if err != nil {
		return err
	}
	for _, artifact := range []struct {
		suffix string
		data   []byte
	}{{".zip", archive.Bytes()}, {".mod", mod}, {".info", info}} {
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

func archiveModule(writer io.Writer, version module.Version, repository, revision, subdir string) (string, error) {
	resolve := exec.Command("git", "rev-parse", revision+"^{commit}")
	resolve.Dir = repository
	frozen, err := resolve.Output()
	if err != nil {
		return "", fmt.Errorf("module proxy: resolve frozen source: %w", err)
	}
	commit := strings.TrimSpace(string(frozen))
	command := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	command.Dir = repository
	data, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("module proxy: locate Git source store: %w", err)
	}
	common := strings.TrimSpace(string(data))
	if !filepath.IsAbs(common) || filepath.Base(common) != ".git" {
		return "", errors.New("module proxy: source must belong to a standard Git checkout")
	}
	// x/mod recognizes the primary checkout, while linked worktrees identify a
	// private Git directory. The shared store owns the same immutable commit.
	return commit, modzip.CreateFromVCS(writer, version, filepath.Dir(common), commit, subdir)
}

// The archive and manifest must still match this frozen source byte for byte.
// An already-published version owns its recorded timestamp even when a later
// commit changes only another module in the repository.
func versionInfo(name, version, timestamp string) ([]byte, error) {
	existing, err := os.ReadFile(name)
	if err == nil {
		var info struct{ Version, Time string }
		if err := json.Unmarshal(existing, &info); err != nil {
			return nil, err
		}
		if info.Version != version {
			return nil, fmt.Errorf("module proxy: metadata version differs: %s", name)
		}
		if _, err := time.Parse(time.RFC3339, info.Time); err != nil {
			return nil, fmt.Errorf("module proxy: invalid published timestamp: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		return nil, err
	}
	info, err := json.Marshal(struct{ Version, Time string }{version, timestamp})
	return append(info, '\n'), err
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
