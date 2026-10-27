// Package native demonstrates an authored document plugin using Qimu's public
// contracts. It does not use the document workbench's built-in implementations.
package native

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"html"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/examples/document"
)

const maxSourceBytes = 1 << 20

//go:embed plugins.go
var pluginSource []byte

type Configuration struct {
	SourceDirectory string               `json:"sourceDirectory"`
	WorkDirectory   string               `json:"workDirectory"`
	Template        document.DocumentRef `json:"template"`
}

func configuration(setup *qimu.Setup) (Configuration, error) {
	var value Configuration
	if err := json.Unmarshal(setup.Configuration(), &value); err != nil {
		return value, err
	}
	if value.SourceDirectory == "" || value.WorkDirectory == "" {
		return value, errors.New("authoring: source and work directories are required")
	}
	return value, nil
}

// Plugin uses the bytes of the authored implementation as its release identity.
// Contract names and identities come from their existing document owner.
func Plugin(contracts document.Contracts) (*qimu.Plugin, error) {
	return qimu.Define(qimu.Definition{Revision: qimu.Hash(pluginSource), Units: []qimu.Unit{
		{
			Declaration: qimu.UnitDeclaration{
				ID: "files", Profile: qimu.NativeGo,
				Exports: []qimu.ExportDeclaration{{Name: "read", Contract: contracts.Files.Ref()}},
			},
			Setup: func(_ context.Context, setup *qimu.Setup) ([]qimu.ExportValue, error) {
				config, err := configuration(setup)
				if err != nil {
					return nil, err
				}
				root, err := os.OpenRoot(config.SourceDirectory)
				if err != nil {
					return nil, err
				}
				if _, err := setup.Cleanup("authored file root", root.Close); err != nil {
					return nil, errors.Join(err, root.Close())
				}
				read := document.FileReader(func(ctx context.Context, ref document.DocumentRef) (document.Document, error) {
					return readSource(ctx, root, ref)
				})
				return []qimu.ExportValue{qimu.Provide("read", contracts.Files, read)}, nil
			},
		},
		{
			Declaration: qimu.UnitDeclaration{
				ID: "converter", Profile: qimu.NativeGo,
				Imports: []qimu.ImportDeclaration{{Slot: "files", Contract: contracts.Files.Ref()}},
				Exports: []qimu.ExportDeclaration{{Name: "uppercase", Contract: contracts.Converters.Ref()}},
			},
			Setup: func(ctx context.Context, setup *qimu.Setup) ([]qimu.ExportValue, error) {
				config, err := configuration(setup)
				if err != nil {
					return nil, err
				}
				files, err := qimu.Import(setup, "files", contracts.Files)
				if err != nil {
					return nil, err
				}
				workspace, err := os.MkdirTemp(config.WorkDirectory, "converter-")
				if err != nil {
					return nil, err
				}
				if _, err := setup.Cleanup("authored converter workspace", func() error { return os.RemoveAll(workspace) }); err != nil {
					return nil, errors.Join(err, os.RemoveAll(workspace))
				}
				// Loading a missing or stale template fails after a real resource
				// was acquired. Its original Setup still owns the workspace.
				template, err := files(ctx, config.Template)
				if err != nil {
					return nil, err
				}
				if strings.Count(template.Text, "{{content}}") != 1 {
					return nil, errors.New("authoring: template requires one content slot")
				}
				preparedTemplate := filepath.Join(workspace, "template.html")
				if err := os.WriteFile(preparedTemplate, []byte(template.Text), 0600); err != nil {
					return nil, err
				}
				convert := document.Converter(func(ctx context.Context, ref document.DocumentRef) (document.Converted, error) {
					source, err := files(ctx, ref)
					if err != nil {
						return document.Converted{}, err
					}
					compiled, err := os.ReadFile(preparedTemplate)
					if err != nil {
						return document.Converted{}, err
					}
					content := html.EscapeString(strings.ToUpper(source.Text))
					output := strings.Replace(string(compiled), "{{content}}", content, 1)
					if err := context.Cause(ctx); err != nil {
						return document.Converted{}, err
					}
					return document.Converted{Source: ref, HTML: output, Title: strings.TrimSpace(strings.SplitN(source.Text, "\n", 2)[0])}, nil
				})
				return []qimu.ExportValue{qimu.Contribute("uppercase", contracts.Converters, document.ConverterMetadata{Format: "text", Priority: 50}, convert)}, nil
			},
		},
		{
			Declaration: qimu.UnitDeclaration{
				ID: "validation", Profile: qimu.NativeGo,
				Exports: []qimu.ExportDeclaration{
					{Name: "nonempty", Contract: contracts.Validators.Ref()},
					{Name: "title", Contract: contracts.Validators.Ref()},
				},
			},
			Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
				nonempty := document.Validator(func(ctx context.Context, value document.Converted) ([]document.Diagnostic, error) {
					if err := context.Cause(ctx); err != nil {
						return nil, err
					}
					if strings.TrimSpace(value.HTML) == "" {
						return []document.Diagnostic{{Rule: "nonempty", Message: "conversion produced no output"}}, nil
					}
					return nil, nil
				})
				title := document.Validator(func(ctx context.Context, value document.Converted) ([]document.Diagnostic, error) {
					if err := context.Cause(ctx); err != nil {
						return nil, err
					}
					if len([]rune(value.Title)) < 8 {
						return []document.Diagnostic{{Rule: "title", Message: "title is shorter than eight characters"}}, nil
					}
					return nil, nil
				})
				return []qimu.ExportValue{
					qimu.Contribute("nonempty", contracts.Validators, document.ValidatorMetadata{Name: "nonempty", Order: 10}, nonempty),
					qimu.Contribute("title", contracts.Validators, document.ValidatorMetadata{Name: "title", Order: 20}, title),
				}, nil
			},
		},
	}})
}

func readSource(ctx context.Context, root *os.Root, ref document.DocumentRef) (_ document.Document, err error) {
	if !fs.ValidPath(ref.Path) || ref.Path == "." || ref.Revision.Validate() != nil {
		return document.Document{}, errors.New("authoring: exact source identity is required")
	}
	if err := context.Cause(ctx); err != nil {
		return document.Document{}, err
	}
	file, err := root.Open(ref.Path)
	if err != nil {
		return document.Document{}, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return document.Document{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSourceBytes {
		return document.Document{}, errors.New("authoring: source must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSourceBytes+1))
	if err != nil {
		return document.Document{}, err
	}
	if len(data) > maxSourceBytes {
		return document.Document{}, errors.New("authoring: source byte budget exceeded")
	}
	if qimu.Hash(data) != ref.Revision {
		return document.Document{}, document.ErrSourceChanged
	}
	if err := context.Cause(ctx); err != nil {
		return document.Document{}, err
	}
	return document.Document{Ref: ref, Text: string(data)}, nil
}
