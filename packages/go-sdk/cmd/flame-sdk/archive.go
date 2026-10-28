package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

func archive(source, output, prefix string) (err error) {
	if !filepath.IsAbs(source) || !filepath.IsAbs(output) {
		return errors.New("plugin archive: source and output must be absolute")
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	parent, name := filepath.Split(output)
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	output = filepath.Join(parent, name)
	if relative, err := filepath.Rel(source, output); err != nil || filepath.IsLocal(relative) {
		return errors.New("plugin archive: output must be outside source")
	}
	if prefix != "" && !filepath.IsLocal(prefix) {
		return errors.New("plugin archive: invalid entry prefix")
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("plugin archive: source must be a directory")
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, file.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(output))
		}
	}()
	writer := zip.NewWriter(file)
	defer func() { err = errors.Join(err, writer.Close()) }()
	return fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("plugin archive: non-regular resource %q", path)
		}
		if !fs.ValidPath(path) || !filepath.IsLocal(path) {
			return errors.New("plugin archive: resource is outside source")
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, path))
		header.Method = zip.Deflate
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		resource, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		input, opened, err := filesystem.OpenFileAtExpected(root, path, info, 0)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(resource, input)
		return errors.Join(copyErr, filesystem.VerifyAtVersion(input, opened, root, path), input.Close())
	})
}

func runArchive(args []string) error {
	flags := flag.NewFlagSet("flame-sdk archive", flag.ContinueOnError)
	source := flags.String("source", "", "absolute built plugin directory")
	output := flags.String("output", "", "absolute new ZIP path")
	prefix := flags.String("prefix", "", "optional local prefix for module source archives")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("plugin archive: unexpected arguments")
	}
	return archive(*source, *output, *prefix)
}
