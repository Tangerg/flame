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

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	sdkplugin "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"
	sdk "github.com/Tangerg/scope/skills"
)

const manifestSchema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
const MCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

func parse(ctx context.Context, root *os.Root, digest identity.Digest) (plugin.Release, error) {
	builder, err := parseMetadata(ctx, root)
	if err != nil {
		return plugin.Release{}, err
	}
	return builder.Release(digest)
}

func parseMetadata(ctx context.Context, root *os.Root) (*plugin.Builder, error) {
	body, err := read(ctx, root, "plugin.json", maxManifestBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errResourceLimit) {
			return nil, fmt.Errorf("%w: manifest: %w", plugin.ErrInvalid, err)
		}
		return nil, fmt.Errorf("pluginpackage: read manifest: %w", err)
	}
	var envelope map[string]jsontext.Value
	if err = json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: manifest object: %w", plugin.ErrInvalid, err)
	}
	if envelope == nil {
		return nil, fmt.Errorf("%w: manifest must be a JSON object", plugin.ErrInvalid)
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
			if err = jsonvalue.UnmarshalNonNull(raw, &v); err != nil {
				return nil, fmt.Errorf("%w: manifest keywords: %w", plugin.ErrInvalid, err)
			}
			if v == nil {
				return nil, fmt.Errorf("%w: invalid keywords", plugin.ErrInvalid)
			}
			continue
		}
		if key == "author" {
			var v map[string]jsontext.Value
			if err = json.Unmarshal(raw, &v); err != nil {
				return nil, fmt.Errorf("%w: manifest author: %w", plugin.ErrInvalid, err)
			}
			if v == nil {
				return nil, fmt.Errorf("%w: invalid author", plugin.ErrInvalid)
			}
			for field, value := range v {
				var text string
				if !slices.Contains([]string{"name", "email", "url"}, field) || string(value) == "null" {
					return nil, fmt.Errorf("%w: invalid author field %q", plugin.ErrInvalid, field)
				}
				if err := json.Unmarshal(value, &text); err != nil {
					return nil, fmt.Errorf("%w: author field %q: %w", plugin.ErrInvalid, field, err)
				}
			}
			continue
		}
		var text string
		if string(raw) == "null" {
			return nil, fmt.Errorf("%w: invalid manifest field %q", plugin.ErrInvalid, key)
		}
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("%w: manifest field %q: %w", plugin.ErrInvalid, key, err)
		}
	}
	var schema, name, version, description string
	_ = json.Unmarshal(envelope["$schema"], &schema)
	_ = json.Unmarshal(envelope["name"], &name)
	_ = json.Unmarshal(envelope["version"], &version)
	_ = json.Unmarshal(envelope["description"], &description)
	if schema != manifestSchema {
		return nil, fmt.Errorf("%w: unsupported manifest schema %q", plugin.ErrInvalid, schema)
	}
	release, err := plugin.NewBuilder(name, version, description)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	for _, diagnostic := range unknown {
		release.Report(diagnostic)
	}
	nativeServers, err := parseMCP(ctx, root, release)
	if err != nil {
		return nil, err
	}
	if raw, found := envelope["extensions"]; found {
		if err := parseExtensions(raw, release, ctx, root, nativeServers); err != nil {
			return nil, err
		}
		if err := cancellation.Err(ctx); err != nil {
			return nil, err
		}
	}
	if err = parseSkills(ctx, root, release); err != nil {
		return nil, err
	}
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	return release, nil
}

func parseExtensions(raw jsontext.Value, release *plugin.Builder, ctx context.Context, root *os.Root, nativeServers []string) error {
	var namespaces map[string]jsontext.Value
	if err := json.Unmarshal(raw, &namespaces); err != nil || namespaces == nil {
		report(release, plugin.Component{Kind: plugin.ComponentManifestField, Name: "extensions"}, plugin.DiagnosticInvalidDeclaration)
		return nil
	}
	raw, found := namespaces[plugin.Namespace]
	if !found {
		return nil
	}
	var reference struct {
		Declaration string `json:"declaration"`
	}
	if err := json.Unmarshal(raw, &reference, json.RejectUnknownMembers(true)); err != nil || !plugin.ValidResourcePath(reference.Declaration) || !strings.HasPrefix(reference.Declaration, plugin.Namespace+"/") {
		report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
		return nil
	}
	body, err := read(ctx, root, reference.Declaration, maxManifestBytes)
	if err != nil {
		report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
		return nil
	}
	var members map[string]jsontext.Value
	var requiredProtocol string
	if err := json.Unmarshal(body, &members); err != nil || members == nil || json.Unmarshal(members["protocol"], &requiredProtocol) != nil || requiredProtocol != sdkplugin.DeclarationProtocol {
		report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
		return nil
	}
	for field := range members {
		if !slices.Contains(declarationFields, field) {
			report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
			return nil
		}
	}
	if raw, found := members["managedBackends"]; found {
		var values []wireManagedBackend
		if err := jsonvalue.UnmarshalNonNull(raw, &values); err != nil {
			return fmt.Errorf("%w: managed backend declarations: %w", plugin.ErrInvalid, err)
		}
		for _, value := range values {
			if slices.Contains(nativeServers, value.ID.String()) {
				return fmt.Errorf("%w: server %s has native and managed startup declarations", plugin.ErrInvalid, value.ID)
			}
			server := plugin.Server{Name: value.ID, Transport: mcpserver.TransportStdio, Managed: &plugin.ManagedBackend{Runtime: value.Runtime, Entry: value.Entry}}
			if err := inspectServerFiles(root, server); err != nil {
				return fmt.Errorf("%w: managed backend %s: %w", plugin.ErrInvalid, value.ID, err)
			}
			if err := release.AdmitServer(server); err != nil {
				return fmt.Errorf("%w: managed backend %s: %w", plugin.ErrInvalid, value.ID, err)
			}
		}
	}
	var contributes map[string]jsontext.Value
	if raw := members["contributes"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &contributes); err != nil || contributes == nil {
			report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
			return nil
		}
		for field := range contributes {
			if !slices.Contains(presentationFields, field) {
				report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
				return nil
			}
		}
	}
	admitContributions(members["contributions"], release, plugin.Component{Kind: plugin.ComponentContribution, Name: "contributions"}, func(w wireContribution) error {
		if !plugin.ValidResourcePath(w.Contract) || !strings.HasPrefix(w.Contract, plugin.Namespace+"/") {
			return plugin.ErrInvalid
		}
		body, err := read(ctx, root, w.Contract, maxManifestBytes)
		if err != nil {
			return err
		}
		contract, err := portable.ParseCapability(body)
		if err != nil {
			return err
		}
		value := plugin.Contribution{Background: w.Background, ID: w.ID, Server: w.Server, Export: w.Export, Contract: contract.Identity()}
		for _, required := range w.Requires {
			if !plugin.ValidResourcePath(required.Contract) || !strings.HasPrefix(required.Contract, plugin.Namespace+"/") {
				return plugin.ErrInvalid
			}
			body, err := read(ctx, root, required.Contract, maxManifestBytes)
			if err != nil {
				return err
			}
			dependency, err := portable.ParseCapability(body)
			if err != nil {
				return err
			}
			value.Requires = append(value.Requires, plugin.Requirement{ID: required.ID, Contract: dependency.Identity(), Optional: required.Optional})
		}
		return release.AdmitContribution(value)
	})
	readPoint := func(resource string) (*portable.Point, error) {
		if !plugin.ValidResourcePath(resource) || !strings.HasPrefix(resource, plugin.Namespace+"/") {
			return nil, plugin.ErrInvalid
		}
		body, err := read(ctx, root, resource, maxManifestBytes)
		if err != nil {
			return nil, err
		}
		return portable.ParsePoint(body)
	}
	admitContributions(members["points"], release, plugin.Component{Kind: plugin.ComponentContribution, Name: "points"}, func(w wirePoint) error {
		contract, err := readPoint(w.Contract)
		if err != nil {
			return err
		}
		return release.AdmitPoint(plugin.Point{ID: w.ID, Contract: contract.Identity()})
	})
	admitContributions(members["pointValues"], release, plugin.Component{Kind: plugin.ComponentContribution, Name: "pointValues"}, func(w wirePointValue) error {
		contract, err := readPoint(w.Contract)
		if err != nil {
			return err
		}
		value, err := jsonvalue.Parse(w.Value)
		if err != nil {
			return err
		}
		return release.AdmitPointValue(plugin.PointValue{ID: w.ID, Contract: contract.Identity(), Value: value})
	})
	admitContributions(members["inputs"], release, plugin.Component{Kind: plugin.ComponentExtensionField, Name: "inputs"}, func(w wireInput) error { return release.AdmitInput(w.domain()) })
	admitContributions(members["components"], release, plugin.Component{Kind: plugin.ComponentContribution, Name: "components"}, func(w wireComponent) error {
		var contribution plugin.ContributionID
		if w.Contribution != nil {
			if err := w.Contribution.Validate(); err != nil {
				return err
			}
			contribution = *w.Contribution
		}
		var material string
		if w.Material != nil {
			if *w.Material == "" {
				return plugin.ErrInvalid
			}
			material = *w.Material
		}
		var settingsIntent extension.SettingsIntent
		if w.SettingsIntent != nil {
			if *w.SettingsIntent == "" {
				return plugin.ErrInvalid
			}
			settingsIntent = *w.SettingsIntent
		}
		if !plugin.ValidResourcePath(w.Entry) || !strings.HasPrefix(w.Entry, plugin.Namespace+"/") {
			return plugin.ErrInvalid
		}
		if _, err := read(ctx, root, w.Entry, maxPackageBytes); err != nil {
			return err
		}
		var selector *plugin.RendererSelector
		if w.Selector != nil {
			selector = &plugin.RendererSelector{Kind: w.Selector.Type, Name: w.Selector.Name}
		}
		return release.AdmitComponent(plugin.ClientComponent{Selector: selector, ID: w.ID, Title: w.Title, Scope: w.Scope, Placement: w.Placement, Entry: w.Entry, Export: w.Export, Contribution: contribution, Material: material, SettingsIntent: settingsIntent})
	})
	for _, component := range slices.Sorted(maps.Keys(contributes)) {
		switch component {
		case "locales":
			admitContributions(contributes[component], release, plugin.Component{Kind: plugin.ComponentContribution, Name: component}, func(w wireLocale) error {
				value := plugin.Locale{Tag: w.Tag, Title: w.Title, Entry: w.Entry}
				if !plugin.ValidResourcePath(value.Entry) || !strings.HasPrefix(value.Entry, plugin.Namespace+"/") {
					return plugin.ErrInvalid
				}
				body, err := read(ctx, root, value.Entry, maxManifestBytes)
				if err != nil {
					return err
				}
				if _, err := decodeLocale(body); err != nil {
					return err
				}
				return release.AdmitLocale(value)
			})
		case "themes":
			admitContributions(contributes[component], release, plugin.Component{Kind: plugin.ComponentContribution, Name: component}, func(w wireTheme) error {
				theme, err := w.domain()
				if err != nil {
					return err
				}
				return release.AdmitTheme(theme)
			})
		default:
			report(release, plugin.Component{Kind: plugin.ComponentContribution, Name: component}, plugin.DiagnosticUnsupportedContribution)
		}
	}
	return nil
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
		if err := jsonvalue.UnmarshalNonNull(raw, &wire); err != nil {
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

func parseMCP(ctx context.Context, root *os.Root, release *plugin.Builder) ([]string, error) {
	body, err := read(ctx, root, "mcp.json", maxManifestBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		if cause := cancellation.Err(ctx); cause != nil {
			return nil, errors.Join(err, cause)
		}
		report(release, plugin.Component{Kind: plugin.ComponentMCP}, plugin.DiagnosticUnavailableComponent)
		return nil, nil
	}
	var envelope struct {
		Schema  string                    `json:"$schema"`
		Servers map[string]jsontext.Value `json:"mcpServers"`
	}
	decodeErr := json.Unmarshal(body, &envelope, json.RejectUnknownMembers(true))
	var names []string
	if decodeErr == nil {
		names = slices.Sorted(maps.Keys(envelope.Servers))
	}
	supported := decodeErr == nil && envelope.Schema == MCPSchema && envelope.Servers != nil
	if !supported {
		report(release, plugin.Component{Kind: plugin.ComponentMCP}, plugin.DiagnosticInvalidDeclaration)
		return names, nil
	}
	for _, name := range names {
		server, err := portableServer(name, envelope.Servers[name])
		if err == nil && server.Transport != mcpserver.TransportStdio {
			return nil, fmt.Errorf("%w: installed server %q requires a packaged process; configure remote MCP through the host registry", plugin.ErrInvalid, name)
		}
		if err == nil {
			if _, packaged := server.PackagedCommand(); !packaged {
				return nil, fmt.Errorf("%w: installed server %q requires a packaged executable", plugin.ErrInvalid, name)
			}
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
			return names, nil
		default:
			report(release, plugin.Component{Kind: plugin.ComponentMCPServer, Name: name}, code)
		}
	}
	return names, nil
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
	if err := jsonvalue.UnmarshalNonNull(raw, &wire); err != nil {
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
	packaged, found := s.PackagedCommand()
	if s.Managed != nil {
		packaged, found = s.Managed.Entry, true
		if !plugin.ValidResourcePath(packaged) {
			return plugin.ErrInvalid
		}
	}
	if found {
		f, _, err := filesystem.OpenFileAt(root, packaged, maxPackageBytes)
		if err != nil {
			return fmt.Errorf("pluginpackage: open server command: %w", packageInputError(err))
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("pluginpackage: inspect server command: %w", err)
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
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	dir, err := fs.ReadDir(root.FS(), "skills")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		report(release, plugin.Component{Kind: plugin.ComponentSkills}, plugin.DiagnosticUnavailableComponent)
		if cause := cancellation.Err(ctx); cause != nil {
			return errors.Join(err, cause)
		}
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
			if cause := cancellation.Err(ctx); cause != nil {
				return errors.Join(err, cause)
			}
			report(release, plugin.Component{Kind: plugin.ComponentSkill, Name: entry.Name()}, plugin.DiagnosticUnavailableComponent)
			continue
		}
		skill, err := skilldocument.Load(ctx, entry.Name(), content)
		if err != nil {
			if cause := cancellation.Err(ctx); cause != nil {
				return errors.Join(err, cause)
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
