package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
)

func run(args []string, output io.Writer) (err error) {
	flags := flag.NewFlagSet("pluginpack", flag.ContinueOnError)
	source := flags.String("source", "", "absolute completed plugin build directory")
	archive := flags.String("archive", "", "optional new ZIP output outside the source")
	osName := flags.String("os", "", "native target operating system")
	arch := flags.String("arch", "", "native target architecture")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*source) {
		return errors.New("pluginpack: an absolute source directory is required")
	}
	physicalSource, err := filepath.EvalSymlinks(*source)
	if err != nil {
		return err
	}
	writer := io.Discard
	var out *archiveOutput
	if *archive != "" {
		out, err = createArchive(physicalSource, *archive)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, out.close()) }()
		writer = out.file
	}
	release, err := pluginpackage.BuildRelease(context.Background(), *source, pluginpackage.BuildTarget{OS: *osName, Arch: *arch}, writer)
	if err != nil {
		return err
	}
	if out != nil {
		if err := out.commit(); err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(struct {
		Package string `json:"package"`
		Version string `json:"version"`
		Release string `json:"release"`
	}{release.Manifest.Package, release.Manifest.Version, release.Digest.String()})
	if err != nil {
		return err
	}
	_, err = output.Write(append(encoded, '\n'))
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type archiveOutput struct {
	path     string
	file     *os.File
	complete bool
}

func createArchive(physicalSource, path string) (*archiveOutput, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("pluginpack: archive must be absolute")
	}
	physicalParent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(physicalSource, filepath.Join(physicalParent, filepath.Base(path)))
	if err != nil || filepath.IsLocal(relative) {
		return nil, errors.New("pluginpack: archive must be outside source")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, filesystem.PrivateFileMode)
	if err != nil {
		return nil, err
	}
	return &archiveOutput{path: path, file: file}, nil
}

func (a *archiveOutput) commit() error {
	if err := a.file.Sync(); err != nil {
		return err
	}
	a.complete = true
	return nil
}

func (a *archiveOutput) close() error {
	err := a.file.Close()
	if !a.complete {
		err = errors.Join(err, os.Remove(a.path))
	}
	return err
}
