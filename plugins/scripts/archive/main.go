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
)

func archive(source, output, prefix string) (err error) {
	if !filepath.IsAbs(source) || !filepath.IsAbs(output) {
		return errors.New("plugin archive: source and output must be absolute")
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return err
	}
	output = filepath.Join(parent, filepath.Base(output))
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
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
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
		relative, err := filepath.Rel(source, path)
		if err != nil || !filepath.IsLocal(relative) {
			return errors.New("plugin archive: resource is outside source")
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, relative))
		header.Method = zip.Deflate
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		resource, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(resource, input)
		return errors.Join(copyErr, input.Close())
	})
}

func main() {
	source := flag.String("source", "", "absolute built plugin directory")
	output := flag.String("output", "", "absolute new ZIP path")
	prefix := flag.String("prefix", "", "optional local prefix for module source archives")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "plugin archive: unexpected arguments")
		os.Exit(1)
	}
	if err := archive(*source, *output, *prefix); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
