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

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	sdk "github.com/Tangerg/scope/skills"
)

const ManifestSchema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
const MCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

func parse(ctx context.Context, root *os.Root, digest fingerprint.Digest) (plugin.Release, error) {
	body, err := read(ctx, root, "plugin.json", MaxManifestBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errResourceLimit) {
			return plugin.Release{}, fmt.Errorf("%w: manifest: %w", plugin.ErrInvalid, err)
		}
		return plugin.Release{}, fmt.Errorf("pluginpackage: read manifest: %w", err)
	}
	var envelope map[string]jsontext.Value
	if err = json.Unmarshal(body, &envelope); err != nil {
		return plugin.Release{}, fmt.Errorf("%w: manifest object: %w", plugin.ErrInvalid, err)
	}
	if envelope == nil {
		return plugin.Release{}, fmt.Errorf("%w: manifest must be a JSON object", plugin.ErrInvalid)
	}
	var unknown []plugin.Diagnostic
	known := []string{"$schema", "name", "version", "description", "author", "homepage", "repository", "license", "keywords", "extensions"}
	for _, key := range slices.Sorted(maps.Keys(envelope)) {
		raw := envelope[key]
		if !slices.Contains(known, key) {
			unknown = append(unknown, plugin.Diagnostic{Component: plugin.Component{Kind: plugin.ComponentManifestField, Name: key}, Code: plugin.DiagnosticUnknownField})
			continue
		}
		if key == "extensions" {
			continue
		}
		if key == "keywords" {
			var v []string
			if err = decodeDeclaration(raw, &v); err != nil {
				return plugin.Release{}, fmt.Errorf("%w: manifest keywords: %w", plugin.ErrInvalid, err)
			}
			if v == nil {
				return plugin.Release{}, fmt.Errorf("%w: invalid keywords", plugin.ErrInvalid)
			}
			continue
		}
		if key == "author" {
			var v map[string]jsontext.Value
			if err = json.Unmarshal(raw, &v); err != nil {
				return plugin.Release{}, fmt.Errorf("%w: manifest author: %w", plugin.ErrInvalid, err)
			}
			if v == nil {
				return plugin.Release{}, fmt.Errorf("%w: invalid author", plugin.ErrInvalid)
			}
			for field, value := range v {
				var text string
				if !slices.Contains([]string{"name", "email", "url"}, field) || string(value) == "null" {
					return plugin.Release{}, fmt.Errorf("%w: invalid author field %q", plugin.ErrInvalid, field)
				}
				if err := json.Unmarshal(value, &text); err != nil {
					return plugin.Release{}, fmt.Errorf("%w: author field %q: %w", plugin.ErrInvalid, field, err)
				}
			}
			continue
		}
		var text string
		if string(raw) == "null" {
			return plugin.Release{}, fmt.Errorf("%w: invalid manifest field %q", plugin.ErrInvalid, key)
		}
		if err := json.Unmarshal(raw, &text); err != nil {
			return plugin.Release{}, fmt.Errorf("%w: manifest field %q: %w", plugin.ErrInvalid, key, err)
		}
	}
	var schema, name, version, description string
	_ = json.Unmarshal(envelope["$schema"], &schema)
	_ = json.Unmarshal(envelope["name"], &name)
	_ = json.Unmarshal(envelope["version"], &version)
	_ = json.Unmarshal(envelope["description"], &description)
	if schema != ManifestSchema {
		return plugin.Release{}, fmt.Errorf("%w: unsupported manifest schema %q", plugin.ErrInvalid, schema)
	}
	release, err := plugin.NewBuilder(name, version, description)
	if err != nil {
		return plugin.Release{}, fmt.Errorf("manifest: %w", err)
	}
	for _, diagnostic := range unknown {
		release.Report(diagnostic)
	}
	if err = parseMCP(ctx, root, release); err != nil {
		return plugin.Release{}, err
	}
	if raw, found := envelope["extensions"]; found {
		parseExtensions(raw, release)
	}
	if err = parseSkills(ctx, root, release); err != nil {
		return plugin.Release{}, err
	}
	return release.Release(digest)
}

func parseExtensions(raw jsontext.Value, release *plugin.Builder) {
	var namespaces map[string]jsontext.Value
	if err := json.Unmarshal(raw, &namespaces); err != nil || namespaces == nil {
		report(release, plugin.Component{Kind: plugin.ComponentManifestField, Name: "extensions"}, plugin.DiagnosticInvalidDeclaration)
		return
	}
	raw, found := namespaces[plugin.Namespace]
	if !found {
		return
	}
	var members map[string]jsontext.Value
	var apiVersion int
	if err := json.Unmarshal(raw, &members); err != nil || members == nil || json.Unmarshal(members["apiVersion"], &apiVersion) != nil || apiVersion != plugin.APIVersion {
		report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
		return
	}
	for _, field := range slices.Sorted(maps.Keys(members)) {
		if !slices.Contains(extensionFields, field) {
			report(release, plugin.Component{Kind: plugin.ComponentExtensionField, Name: field}, plugin.DiagnosticUnknownField)
		}
	}
	admitContributions(members["inputs"], release, plugin.Component{Kind: plugin.ComponentExtensionField, Name: "inputs"}, func(w wireInput) error { return release.AdmitInput(w.domain()) })
	var contributes map[string]jsontext.Value
	if raw := members["contributes"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &contributes); err != nil || contributes == nil {
			report(release, plugin.Component{Kind: plugin.ComponentExtensionField, Name: "contributes"}, plugin.DiagnosticInvalidDeclaration)
		}
	}
	for _, component := range slices.Sorted(maps.Keys(contributes)) {
		if component != "themes" {
			report(release, plugin.Component{Kind: plugin.ComponentContribution, Name: component}, plugin.DiagnosticUnsupportedContribution)
			continue
		}
		admitContributions(contributes[component], release, plugin.Component{Kind: plugin.ComponentContribution, Name: component}, func(w wireTheme) error {
			theme, err := w.domain()
			if err != nil {
				return err
			}
			return release.AdmitTheme(theme)
		})
	}
}

func report(release *plugin.Builder, component plugin.Component, code plugin.DiagnosticCode) {
	release.Report(plugin.Diagnostic{Component: component, Code: code})
}

// admitContributions isolates each invalid member of one component. A member
// that decodes is validated once, against the members admitted before it.
func admitContributions[T any](raw jsontext.Value, release *plugin.Builder, component plugin.Component, admit func(T) error) {
	if len(raw) == 0 {
		return
	}
	var values []jsontext.Value
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		report(release, component, plugin.DiagnosticInvalidDeclaration)
		return
	}
	for _, raw := range values {
		var wire T
		if err := decodeDeclaration(raw, &wire); err != nil {
			report(release, component, plugin.DiagnosticInvalidDeclaration)
			continue
		}
		code, admitted := admission(admit(wire))
		if admitted {
			continue
		}
		report(release, component, code)
		if code == plugin.DiagnosticComponentLimit {
			return
		}
	}
}

// admission names the diagnostic for a builder's refusal. Capacity belongs to
// the builder alone: the first member it refuses for capacity ends the
// component, so the remaining members are reported once rather than each.
func admission(err error) (plugin.DiagnosticCode, bool) {
	if err == nil {
		return "", true
	}
	if refusal, ok := errors.AsType[*plugin.Refusal](err); ok {
		return refusal.Code, false
	}
	return plugin.DiagnosticInvalidDeclaration, false
}

func parseMCP(ctx context.Context, root *os.Root, release *plugin.Builder) error {
	body, err := read(ctx, root, "mcp.json", MaxManifestBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		report(release, plugin.Component{Kind: plugin.ComponentMCP}, plugin.DiagnosticUnavailableComponent)
		return nil
	}
	var envelope struct {
		Schema  string                    `json:"$schema"`
		Servers map[string]jsontext.Value `json:"mcpServers"`
	}
	decodeErr := json.Unmarshal(body, &envelope, json.RejectUnknownMembers(true))
	supported := decodeErr == nil && envelope.Schema == MCPSchema && envelope.Servers != nil
	if !supported {
		report(release, plugin.Component{Kind: plugin.ComponentMCP}, plugin.DiagnosticInvalidDeclaration)
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(envelope.Servers)) {
		server, err := portableServer(name, envelope.Servers[name])
		if err == nil {
			err = inspectServerFiles(root, server)
		}
		if err == nil {
			err = release.AdmitServer(server)
		}
		code, admitted := admission(err)
		switch {
		case admitted:
		case code == plugin.DiagnosticComponentLimit:
			report(release, plugin.Component{Kind: plugin.ComponentMCP}, code)
			return nil
		default:
			report(release, plugin.Component{Kind: plugin.ComponentMCPServer, Name: name}, code)
		}
	}
	return nil
}

// portableServer is the only translation from the portable mcp.json spelling
// into the MCP registry vocabulary the release declares.
func portableServer(name string, raw jsontext.Value) (plugin.Server, error) {
	var wire struct {
		Type    string            `json:"type"`
		Command string            `json:"command,omitempty"`
		Args    []string          `json:"args,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
		CWD     string            `json:"cwd,omitempty"`
		URL     string            `json:"url,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
	}
	if err := decodeDeclaration(raw, &wire); err != nil {
		return plugin.Server{}, fmt.Errorf("%w: server declaration: %w", plugin.ErrInvalid, err)
	}
	var members map[string]jsontext.Value
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		return plugin.Server{}, fmt.Errorf("%w: server declaration must be an object", plugin.ErrInvalid)
	}
	local, err := mcpserver.ParseServerName(name)
	if err != nil {
		return plugin.Server{}, fmt.Errorf("%w: server name: %w", plugin.ErrInvalid, err)
	}
	server := plugin.Server{Name: local}
	var allowed []string
	switch wire.Type {
	case "stdio":
		server.Transport = mcpserver.TransportStdio
		server.Command, server.Args, server.Env, server.Dir = wire.Command, wire.Args, wire.Env, wire.CWD
		allowed = []string{"type", "command", "args", "env", "cwd"}
	case "streamable-http":
		server.Transport = mcpserver.TransportStreamableHTTP
		server.URL, server.Headers = wire.URL, wire.Headers
		allowed = []string{"type", "url", "headers"}
	default:
		return plugin.Server{}, fmt.Errorf("%w: package server transport %q", plugin.ErrInvalid, wire.Type)
	}
	for key := range members {
		if !slices.Contains(allowed, key) {
			return plugin.Server{}, fmt.Errorf("%w: server member %q does not apply to %s", plugin.ErrInvalid, key, wire.Type)
		}
	}
	return server, nil
}

// inspectServerFiles requires a package-relative command and working
// directory to exist in the exact bytes being admitted.
func inspectServerFiles(root *os.Root, s plugin.Server) error {
	if packaged, found := s.PackagedCommand(); found {
		f, err := root.Open(packaged)
		if err != nil {
			return fmt.Errorf("pluginpackage: open server command: %w", err)
		}
		info, err := f.Stat()
		err = errors.Join(err, f.Close())
		if err != nil {
			return fmt.Errorf("pluginpackage: inspect server command: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: server command is not a regular file", plugin.ErrInvalid)
		}
	}
	workingDirectory, err := s.WorkingDirectory()
	if err != nil {
		return err
	}
	if workingDirectory.Base() == plugin.ReleaseBase {
		_, relative := locate(workingDirectory, "", "")
		info, err := root.Stat(relative)
		if err != nil {
			return fmt.Errorf("%w: server working directory: %w", plugin.ErrInvalid, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: server working directory is not a directory", plugin.ErrInvalid)
		}
	}
	return nil
}

func parseSkills(ctx context.Context, root *os.Root, release *plugin.Builder) error {
	dir, err := fs.ReadDir(root.FS(), "skills")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		report(release, plugin.Component{Kind: plugin.ComponentSkills}, plugin.DiagnosticUnavailableComponent)
		return ctx.Err()
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
			report(release, plugin.Component{Kind: plugin.ComponentSkill, Name: entry.Name()}, plugin.DiagnosticUnavailableComponent)
			continue
		}
		skill, err := promptsource.LoadSkillDocument(ctx, entry.Name(), content)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			report(release, plugin.Component{Kind: plugin.ComponentSkill, Name: entry.Name()}, plugin.DiagnosticInvalidDeclaration)
			continue
		}
		code, admitted := admission(release.AdmitSkill(plugin.Skill{Name: skill.Name, Description: skill.Description}))
		switch {
		case admitted:
		case code == plugin.DiagnosticComponentLimit:
			report(release, plugin.Component{Kind: plugin.ComponentSkills}, code)
			return nil
		default:
			report(release, plugin.Component{Kind: plugin.ComponentSkill, Name: entry.Name()}, code)
		}
	}
	return nil
}
