package document

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/control"
	"github.com/Tangerg/flame/qimu/distribution"
)

//go:embed assets/preview.js assets/theme.css
var assets embed.FS

func buildPreview(ctx context.Context, store *distribution.Store, contracts Contracts, authorize func(qimu.Digest)) (distribution.Release, error) {
	directory, err := os.MkdirTemp("", "qimu-document-package-")
	if err != nil {
		return distribution.Release{}, err
	}
	defer os.RemoveAll(directory)
	module, err := assets.ReadFile("assets/preview.js")
	if err != nil {
		return distribution.Release{}, err
	}
	ref, err := json.Marshal(contracts.Previews.Ref())
	if err != nil {
		return distribution.Release{}, err
	}
	module = append(append([]byte("const point = Object.freeze({ ref: "), ref...), append([]byte(" });\n"), module...)...)
	if err := os.WriteFile(filepath.Join(directory, "preview.js"), module, 0600); err != nil {
		return distribution.Release{}, err
	}
	theme, err := assets.ReadFile("assets/theme.css")
	if err != nil {
		return distribution.Release{}, err
	}
	if err := os.WriteFile(filepath.Join(directory, "theme.css"), theme, 0600); err != nil {
		return distribution.Release{}, err
	}
	manifest := distribution.Manifest{SchemaVersion: 1, Package: "example.document-preview", Version: "1", Artifacts: []distribution.Artifact{{Path: "preview.js"}, {Path: "theme.css"}}, Contracts: []qimu.ContractRef{contracts.Previews.Ref(), contracts.Theme.Ref()}, Units: []distribution.Unit{{ID: "preview", Profile: qimu.BrowserJS, Exports: []qimu.ExportDeclaration{{Name: "preview", Contract: contracts.Previews.Ref()}}, Module: &distribution.Module{Entry: "preview.js"}}, {ID: "theme", Profile: qimu.StaticResource, Exports: []qimu.ExportDeclaration{{Name: "theme", Contract: contracts.Theme.Ref()}}, Resources: []string{"theme.css"}}}}
	var archive bytes.Buffer
	if _, err := distribution.Build(ctx, directory, manifest, nil, &archive); err != nil {
		return distribution.Release{}, err
	}
	path := filepath.Join(directory, "preview.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		return distribution.Release{}, err
	}
	authorize(qimu.Hash(archive.Bytes()))
	return store.InstallArchive(ctx, path, qimu.Hash(archive.Bytes()))
}

func previewDefinition(release distribution.Release) (*qimu.Plugin, error) {
	units := make([]qimu.Unit, 0, len(release.Manifest.Units))
	for _, unit := range release.Manifest.Units {
		if len(unit.Imports) != 0 {
			return nil, errors.New("document: built-in preview has no imports")
		}
		units = append(units, qimu.Unit{Declaration: qimu.UnitDeclaration{ID: unit.ID, Profile: unit.Profile, Exports: unit.Exports}})
	}
	return qimu.Define(qimu.Definition{Revision: release.Digest, Units: units})
}

type moduleSource struct{ store *distribution.Store }

func (s moduleSource) Module(ctx context.Context, binding qimu.UnitBinding) (control.Artifact, error) {
	release, err := s.store.Inspect(ctx, binding.Source.Release)
	if err != nil {
		return control.Artifact{}, err
	}
	for _, unit := range release.Manifest.Units {
		if unit.ID == binding.Source.UnitID && unit.Profile == qimu.BrowserJS && unit.Module != nil {
			for _, artifact := range release.Manifest.Artifacts {
				if artifact.Path == unit.Module.Entry {
					return control.Artifact{Path: "/qimu/artifacts/" + string(release.Digest) + "/" + artifact.Path, Digest: artifact.Digest}, nil
				}
			}
		}
	}
	return control.Artifact{}, qimu.ErrUnavailable
}

func (a *App) artifact(ctx context.Context, path string) ([]byte, string, error) {
	release, name, found := strings.Cut(path, "/")
	if !found || qimu.Digest(release) != a.preview.Digest {
		return nil, "", qimu.ErrUnavailable
	}
	lease, err := a.Content.Acquire(ctx, a.preview.Digest)
	if err != nil {
		return nil, "", err
	}
	defer lease.Close()
	data, err := lease.Read(ctx, name)
	if err != nil {
		return nil, "", err
	}
	contentType := "application/octet-stream"
	if strings.HasSuffix(name, ".js") {
		contentType = "text/javascript; charset=utf-8"
	} else if strings.HasSuffix(name, ".css") {
		contentType = "text/css; charset=utf-8"
	}
	return data, contentType, nil
}
