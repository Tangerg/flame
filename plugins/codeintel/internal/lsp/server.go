package lsp

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

type ServerSpec struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Languages   map[string]string `json:"languages"`
	RootMarkers []string          `json:"rootMarkers"`
}

func validateLanguageRoute(extension, language string) error {
	if !validServerField(language) || extension != strings.ToLower(extension) {
		return fmt.Errorf("invalid document language route %q", extension)
	}
	if len(extension) < 2 || extension[0] != '.' || !validServerField(extension) || strings.ContainsAny(extension, `/\\`) {
		return fmt.Errorf("file extension %q is invalid", extension)
	}
	return nil
}

func (s ServerSpec) validate() error {
	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "name", value: s.Name},
		{name: "command", value: s.Command},
	} {
		if !validServerField(required.value) {
			return fmt.Errorf("%s %q is invalid", required.name, required.value)
		}
	}
	if len(s.Languages) == 0 {
		return fmt.Errorf("server requires document languages")
	}
	for extension, language := range s.Languages {
		if err := validateLanguageRoute(extension, language); err != nil {
			return err
		}
	}
	for _, marker := range s.RootMarkers {
		if !validServerField(marker) || marker == "." || marker == ".." || strings.ContainsAny(marker, `/\\`) {
			return fmt.Errorf("root marker %q must be a direct child name", marker)
		}
	}
	return nil
}

func validServerField(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}

func DefaultServers() []ServerSpec {
	return []ServerSpec{
		{
			Name:        "gopls",
			Command:     "gopls",
			Languages:   map[string]string{".go": "go"},
			RootMarkers: []string{"go.mod", "go.work"},
		},
		{
			Name:        "typescript",
			Command:     "typescript-language-server",
			Args:        []string{"--stdio"},
			Languages:   map[string]string{".ts": "typescript", ".mts": "typescript", ".cts": "typescript", ".tsx": "typescriptreact"},
			RootMarkers: []string{"tsconfig.json", "jsconfig.json", "package.json"},
		},
	}
}

type serverTable struct {
	specs []ServerSpec
	byExt map[string]ServerSpec
}

func newServerTable(specs []ServerSpec) (*serverTable, error) {
	specs = slices.Clone(specs)
	byName := make(map[string]struct{}, len(specs))
	for i := range specs {
		if err := specs[i].validate(); err != nil {
			return nil, fmt.Errorf("lsp: server %d: %w", i+1, err)
		}
		name := specs[i].Name
		if _, duplicate := byName[name]; duplicate {
			return nil, fmt.Errorf("lsp: server name %q is configured more than once", name)
		}
		byName[name] = struct{}{}
		specs[i].Args = slices.Clone(specs[i].Args)
		specs[i].Languages = maps.Clone(specs[i].Languages)
		specs[i].RootMarkers = slices.Clone(specs[i].RootMarkers)
	}
	byExt := make(map[string]ServerSpec, len(specs))
	for _, spec := range specs {
		for ext := range spec.Languages {
			if owner, duplicate := byExt[ext]; duplicate {
				return nil, fmt.Errorf(
					"lsp: file extension %q is assigned to both %q and %q",
					ext,
					owner.Name,
					spec.Name,
				)
			}
			byExt[ext] = spec
		}
	}
	return &serverTable{specs: specs, byExt: byExt}, nil
}

func (s *serverTable) forFile(path string) (ServerSpec, bool) {
	spec, ok := s.byExt[strings.ToLower(filepath.Ext(path))]
	return spec, ok
}

func (s *serverTable) forRoot(root string) []ServerSpec {
	var out []ServerSpec
	for _, spec := range s.specs {
		for _, marker := range spec.RootMarkers {
			if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
				out = append(out, spec)
				break
			}
		}
	}
	return out
}
