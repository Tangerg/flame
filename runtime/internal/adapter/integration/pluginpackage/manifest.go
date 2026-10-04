package pluginpackage

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	sdk "github.com/Tangerg/scope/skills"
)

const ManifestSchema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
const MCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

func parse(ctx context.Context, root *os.Root, digest string) (plugin.Release, error) {
	release := plugin.Release{Digest: digest}
	body, err := read(ctx, root, "plugin.json", MaxManifestBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errResourceLimit) {
			return release, errors.Join(plugin.ErrInvalid, err)
		}
		return release, err
	}
	var envelope map[string]jsontext.Value
	if err = json.Unmarshal(body, &envelope); err != nil {
		return release, fmt.Errorf("%w: manifest object: %w", plugin.ErrInvalid, err)
	}
	if envelope == nil {
		return release, fmt.Errorf("%w: manifest must be a JSON object", plugin.ErrInvalid)
	}
	known := []string{"$schema", "name", "version", "description", "author", "homepage", "repository", "license", "keywords", "extensions"}
	for _, key := range slices.Sorted(maps.Keys(envelope)) {
		raw := envelope[key]
		if !slices.Contains(known, key) {
			release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "manifest", Code: "unknown_field:" + key})
			continue
		}
		if key == "extensions" {
			continue
		}
		if key == "keywords" {
			var v []string
			if err = decodeDeclaration(raw, &v); err != nil {
				return release, fmt.Errorf("%w: manifest keywords: %w", plugin.ErrInvalid, err)
			}
			if v == nil {
				return release, fmt.Errorf("%w: invalid keywords", plugin.ErrInvalid)
			}
			continue
		}
		if key == "author" {
			var v map[string]jsontext.Value
			if err = json.Unmarshal(raw, &v); err != nil {
				return release, fmt.Errorf("%w: manifest author: %w", plugin.ErrInvalid, err)
			}
			if v == nil {
				return release, fmt.Errorf("%w: invalid author", plugin.ErrInvalid)
			}
			for field, value := range v {
				var text string
				if !slices.Contains([]string{"name", "email", "url"}, field) || string(value) == "null" {
					return release, fmt.Errorf("%w: invalid author field %q", plugin.ErrInvalid, field)
				}
				if err := json.Unmarshal(value, &text); err != nil {
					return release, fmt.Errorf("%w: author field %q: %w", plugin.ErrInvalid, field, err)
				}
			}
			continue
		}
		var text string
		if string(raw) == "null" {
			return release, fmt.Errorf("%w: invalid manifest field %q", plugin.ErrInvalid, key)
		}
		if err := json.Unmarshal(raw, &text); err != nil {
			return release, fmt.Errorf("%w: manifest field %q: %w", plugin.ErrInvalid, key, err)
		}
	}
	var schema string
	_ = json.Unmarshal(envelope["$schema"], &schema)
	_ = json.Unmarshal(envelope["name"], &release.Name)
	_ = json.Unmarshal(envelope["version"], &release.Version)
	_ = json.Unmarshal(envelope["description"], &release.Description)
	if schema != ManifestSchema || !plugin.ValidName(release.Name) {
		return release, fmt.Errorf("%w: unsupported or invalid manifest", plugin.ErrInvalid)
	}
	if err = parseMCP(ctx, root, &release); err != nil {
		return release, err
	}
	if raw, found := envelope["extensions"]; found {
		var namespaces map[string]jsontext.Value
		if err = json.Unmarshal(raw, &namespaces); err != nil || namespaces == nil {
			release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "manifest", Code: "ignored_extensions"})
		} else if raw, found := namespaces[plugin.Namespace]; found {
			var wire wireExtension
			err = json.Unmarshal(raw, &wire, json.RejectUnknownMembers(true))
			if err != nil || wire.APIVersion != plugin.APIVersion {
				release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "extension", Code: "invalid_extension"})
			} else {
				admitContributions(wire.Requests, plugin.MaxRequests, &release, "request", func(r *plugin.Release, w wireRequestGrant) { r.Requests = append(r.Requests, w.domain()) })
				admitContributions(wire.Inputs, plugin.MaxInputs, &release, "input", func(r *plugin.Release, w wireInput) { r.Inputs = append(r.Inputs, w.domain()) })
				var contributes map[string]jsontext.Value
				if len(wire.Contributes) > 0 {
					if err := json.Unmarshal(wire.Contributes, &contributes); err != nil || contributes == nil {
						release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "contributes", Code: "invalid_declaration"})
					}
				}
				for _, component := range slices.Sorted(maps.Keys(contributes)) {
					raw := contributes[component]
					if component != "themes" {
						release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: component, Code: "unsupported_contribution"})
						continue
					}
					admitContributions(raw, plugin.MaxThemes, &release, "theme", func(r *plugin.Release, w wireTheme) { r.Themes = append(r.Themes, w.domain()) })
				}
			}
		}
	}
	if err = parseSkills(ctx, root, &release); err != nil {
		return release, err
	}
	return release, nil
}
func admitContributions[T any](raw jsontext.Value, limit int, release *plugin.Release, component string, appendValue func(*plugin.Release, T)) {
	if len(raw) == 0 {
		return
	}
	var values []jsontext.Value
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: component, Code: "invalid_declaration"})
		return
	}
	if len(values) > limit {
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: component, Code: "component_limit"})
		return
	}
	for _, raw := range values {
		admitContribution(raw, release, component, appendValue)
	}
}

func admitContribution[T any](raw jsontext.Value, release *plugin.Release, component string, appendValue func(*plugin.Release, T)) {
	var wire T
	if err := decodeDeclaration(raw, &wire); err != nil {
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: component, Code: "invalid_declaration"})
		return
	}
	candidate := release.Clone()
	appendValue(&candidate, wire)
	if err := candidate.Validate(); err != nil {
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: component, Code: "invalid_dependencies"})
		return
	}
	*release = candidate
}

func parseMCP(ctx context.Context, root *os.Root, release *plugin.Release) error {
	body, err := read(ctx, root, "mcp.json", MaxManifestBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "mcp", Code: "unavailable_component"})
		return nil
	}
	var envelope struct {
		Schema  string                    `json:"$schema"`
		Servers map[string]jsontext.Value `json:"mcpServers"`
	}
	if err = json.Unmarshal(body, &envelope, json.RejectUnknownMembers(true)); err != nil || envelope.Schema != MCPSchema || envelope.Servers == nil || len(envelope.Servers) > plugin.MaxServers {
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "mcp", Code: "invalid_envelope"})
		return nil
	}
	names := make([]string, 0, len(envelope.Servers))
	for name := range envelope.Servers {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		var wire struct {
			Type    string            `json:"type"`
			Command string            `json:"command,omitempty"`
			Args    []string          `json:"args,omitempty"`
			Env     map[string]string `json:"env,omitempty"`
			CWD     string            `json:"cwd,omitempty"`
			URL     string            `json:"url,omitempty"`
			Headers map[string]string `json:"headers,omitempty"`
		}
		err = decodeDeclaration(envelope.Servers[name], &wire)
		var members map[string]jsontext.Value
		if decodeErr := json.Unmarshal(envelope.Servers[name], &members); decodeErr != nil || members == nil {
			err = plugin.ErrInvalid
		}
		allowed := []string{"type", "command", "args", "env", "cwd"}
		if wire.Type == string(plugin.StreamableHTTP) {
			allowed = []string{"type", "url", "headers"}
		}
		for key := range members {
			if !slices.Contains(allowed, key) {
				err = plugin.ErrInvalid
			}
		}
		server := plugin.Server{Name: name, Type: plugin.Transport(wire.Type), Command: wire.Command, Args: wire.Args, Env: wire.Env, CWD: wire.CWD, URL: wire.URL, Headers: wire.Headers}
		if err != nil || validateServer(root, server) != nil {
			release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "mcp:" + name, Code: "invalid_server"})
			continue
		}
		release.Servers = append(release.Servers, server)
	}
	return nil
}
func validateServer(root *os.Root, s plugin.Server) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if strings.HasPrefix(s.Command, "./") {
		f, err := root.Open(s.Command[2:])
		if err != nil {
			return err
		}
		info, err := f.Stat()
		err = errors.Join(err, f.Close())
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return plugin.ErrInvalid
		}
	}
	if s.CWD != "" && !strings.HasPrefix(s.CWD, "${PLUGIN_DATA}") {
		relative := strings.TrimPrefix(strings.TrimPrefix(s.CWD, "${PLUGIN_ROOT}"), "./")
		relative = strings.TrimPrefix(relative, "/")
		if relative == "" {
			relative = "."
		}
		info, err := root.Stat(relative)
		if err != nil || !info.IsDir() {
			return errors.Join(plugin.ErrInvalid, err)
		}
	}
	return nil
}

func parseSkills(ctx context.Context, root *os.Root, release *plugin.Release) error {
	dir, err := fs.ReadDir(root.FS(), "skills")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "skills", Code: "invalid_component"})
		return ctx.Err()
	}
	if len(dir) > domainskills.MaxSkillsPerSource {
		release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "skills", Code: "source_limit"})
		return nil
	}
	for _, entry := range dir {
		if !entry.IsDir() {
			continue
		}
		content, err := read(ctx, root, path.Join("skills", entry.Name(), sdk.SkillFile), domainskills.MaxAuthoredSkillDocumentBytes)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "skill:" + entry.Name(), Code: "invalid_skill"})
			continue
		}
		skill, err := promptsource.LoadSkillDocument(ctx, entry.Name(), content)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			release.Diagnostics = append(release.Diagnostics, plugin.Diagnostic{Component: "skill:" + entry.Name(), Code: "invalid_skill"})
			continue
		}
		release.Skills = append(release.Skills, plugin.Skill{Name: skill.Name, Description: skill.Description})
	}
	return nil
}
