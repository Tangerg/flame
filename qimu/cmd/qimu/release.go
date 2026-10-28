package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
)

func runPackage(args []string, output io.Writer) (err error) {
	flags := flag.NewFlagSet("qimu package", flag.ContinueOnError)
	source := flags.String("source", "", "release source directory")
	manifestPath := flags.String("manifest", "", "source manifest; defaults to source/manifest.json")
	dependenciesPath := flags.String("dependencies", "", "JSON array of exact locked dependency releases")
	destination := flags.String("out", "", "new ZIP artifact path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *source == "" || *destination == "" {
		return errors.New("source and out are required")
	}
	if *manifestPath == "" {
		*manifestPath = filepath.Join(*source, distribution.ManifestName)
	}
	data, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	var manifest distribution.Manifest
	if err := json.Unmarshal(data, &manifest, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	var dependencies []distribution.LockedRelease
	if *dependenciesPath != "" {
		data, err := os.ReadFile(*dependenciesPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &dependencies, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(*destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		err = errors.Join(err, file.Close())
		if !complete {
			err = errors.Join(err, os.Remove(*destination))
		}
	}()
	digest := sha256.New()
	lock, err := distribution.Build(context.Background(), *source, manifest, dependencies, io.MultiWriter(file, digest))
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	complete = true
	return json.MarshalWrite(output, struct {
		ArchiveDigest string            `json:"archiveDigest"`
		Lock          distribution.Lock `json:"lock"`
	}{hex.EncodeToString(digest.Sum(nil)), lock})
}

func runVerify(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("qimu verify", flag.ContinueOnError)
	archive := flags.String("archive", "", "release ZIP file")
	expected := flags.String("digest", "", "expected SHA-256 digest of the ZIP bytes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *archive == "" {
		return errors.New("archive and digest are required")
	}
	digest, err := qimu.ParseDigest(*expected)
	if err != nil {
		return err
	}
	release, err := distribution.VerifyArchive(context.Background(), *archive, digest, distribution.DefaultLimits())
	if err != nil {
		return err
	}
	return json.MarshalWrite(output, release)
}
