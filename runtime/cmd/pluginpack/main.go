// Command pluginpack builds a Flame consumer release through its original
// metadata interpretation and Qimu's neutral distribution contract.
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
	var writer io.Writer = io.Discard
	var file *os.File
	complete := false
	if *archive != "" {
		if !filepath.IsAbs(*archive) {
			return errors.New("pluginpack: archive must be absolute")
		}
		physicalParent, err := filepath.EvalSymlinks(filepath.Dir(*archive))
		if err != nil {
			return err
		}
		relative, relErr := filepath.Rel(physicalSource, filepath.Join(physicalParent, filepath.Base(*archive)))
		if relErr != nil || filepath.IsLocal(relative) {
			return errors.New("pluginpack: archive must be outside source")
		}
		file, err = os.OpenFile(*archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer func() {
			err = errors.Join(err, file.Close())
			if !complete {
				err = errors.Join(err, os.Remove(*archive))
			}
		}()
		writer = file
	}
	release, err := pluginpackage.BuildRelease(context.Background(), *source, pluginpackage.BuildTarget{OS: *osName, Arch: *arch}, writer)
	if err != nil {
		return err
	}
	if file != nil {
		if err := file.Sync(); err != nil {
			return err
		}
	}
	complete = true
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
