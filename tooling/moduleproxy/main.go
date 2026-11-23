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

const (
	goModFile     = "go.mod"
	gitExecutable = "git"
)

// The proxy builds before any candidate module exists, so it owns its published modes instead of importing them.
const (
	publishedDirectoryMode = 0o755
	publishedFileMode      = 0o644
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type publication struct {
	repository, revision, treeTime, output, modulePath, subdir, version string
}

func run() error {
	var p publication
	flag.StringVar(&p.repository, "repository", "", "Git source repository")
	flag.StringVar(&p.revision, "revision", "", "frozen Git source")
	flag.StringVar(&p.treeTime, "tree-time", "", "publication time of a frozen module directory tree")
	flag.StringVar(&p.output, "output", "", "file proxy directory")
	flag.StringVar(&p.modulePath, "module", "", "module path")
	flag.StringVar(&p.subdir, "subdir", "", "module source directory")
	flag.StringVar(&p.version, "version", "", "immutable candidate version")
	flag.Parse()
	if p.repository == "" || p.revision == "" || p.output == "" || p.modulePath == "" || p.version == "" {
		return errors.New("module proxy: repository, revision, output, module and version are required")
	}
	if err := module.Check(p.modulePath, p.version); err != nil {
		return err
	}
	escaped, err := module.EscapePath(p.modulePath)
	if err != nil {
		return err
	}
	directory := filepath.Join(p.output, escaped, "@v")
	if err := os.MkdirAll(directory, publishedDirectoryMode); err != nil {
		return err
	}
	if err := p.publishArtifacts(directory); err != nil {
		return err
	}
	return writeVersionList(directory)
}

func (p publication) publishArtifacts(directory string) error {
	escapedVersion, err := module.EscapeVersion(p.version)
	if err != nil {
		return err
	}
	var archive bytes.Buffer
	frozen, err := archiveModule(&archive, module.Version{Path: p.modulePath, Version: p.version}, p.repository, p.revision, p.subdir)
	if err != nil {
		return err
	}
	source := goModFile
	if p.subdir != "" {
		source = filepath.ToSlash(filepath.Join(p.subdir, goModFile))
	}
	read := func(args ...string) ([]byte, error) {
		command := exec.Command(gitExecutable, args...)
		command.Dir = p.repository
		return command.Output()
	}
	mod, err := read("show", frozen+":"+source)
	if err != nil {
		return err
	}
	timestamp, err := publicationTime(p.subdir, p.treeTime, frozen, read)
	if err != nil {
		return err
	}
	info, err := versionInfo(filepath.Join(directory, escapedVersion+".info"), p.version, timestamp, p.treeTime == "")
	if err != nil {
		return err
	}
	for _, artifact := range []struct {
		suffix string
		data   []byte
	}{{".zip", archive.Bytes()}, {".mod", mod}, {".info", info}} {
		if err := publishImmutable(filepath.Join(directory, escapedVersion+artifact.suffix), artifact.data); err != nil {
			return err
		}
	}
	_, err = modzip.CheckZip(module.Version{Path: p.modulePath, Version: p.version}, filepath.Join(directory, escapedVersion+".zip"))
	return err
}

func writeVersionList(directory string) error {
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
	return os.WriteFile(filepath.Join(directory, "list"), []byte(strings.Join(versions, "\n")+"\n"), publishedFileMode)
}

func publicationTime(subdir, treeTime, source string, read func(...string) ([]byte, error)) (string, error) {
	if treeTime != "" {
		if subdir != "" {
			return "", errors.New("module proxy: a tree pin already names the module directory")
		}
		if _, err := time.Parse(time.RFC3339, treeTime); err != nil {
			return "", err
		}
		return treeTime, nil
	}
	value, err := read("show", "-s", "--format=%cI", source)
	return strings.TrimSpace(string(value)), err
}

func archiveModule(writer io.Writer, version module.Version, repository, revision, subdir string) (string, error) {
	resolve := exec.Command(gitExecutable, "rev-parse", revision+"^{}")
	resolve.Dir = repository
	frozen, err := resolve.Output()
	if err != nil {
		return "", fmt.Errorf("module proxy: resolve frozen source: %w", err)
	}
	source := strings.TrimSpace(string(frozen))
	command := exec.Command(gitExecutable, "rev-parse", "--path-format=absolute", "--git-common-dir")
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
	return source, modzip.CreateFromVCS(writer, version, filepath.Dir(common), source, subdir)
}

// Field names are fixed by the GOPROXY protocol's $version.info document.
type proxyVersionInfo struct {
	Version string `json:"Version"`
	Time    string `json:"Time"`
}

func versionInfo(name, version, timestamp string, usePublishedTime bool) ([]byte, error) {
	existing, err := os.ReadFile(name)
	if err == nil {
		var info proxyVersionInfo
		if err := json.Unmarshal(existing, &info); err != nil {
			return nil, err
		}
		if info.Version != version {
			return nil, fmt.Errorf("module proxy: metadata version differs: %s", name)
		}
		if _, err := time.Parse(time.RFC3339, info.Time); err != nil {
			return nil, fmt.Errorf("module proxy: invalid published timestamp: %w", err)
		}
		if !usePublishedTime && info.Time != timestamp {
			return nil, fmt.Errorf("module proxy: immutable publication time differs: %s", name)
		}
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		return nil, err
	}
	info, err := json.Marshal(proxyVersionInfo{Version: version, Time: timestamp})
	return append(info, '\n'), err
}

func publishImmutable(name string, data []byte) error {
	staged, err := os.CreateTemp(filepath.Dir(name), ".module-*")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	if _, err := staged.Write(data); err != nil {
		return errors.Join(err, staged.Close())
	}
	if err := staged.Chmod(publishedFileMode); err != nil {
		return errors.Join(err, staged.Close())
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
