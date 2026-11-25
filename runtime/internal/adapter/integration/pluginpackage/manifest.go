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
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framevalue "github.com/Tangerg/frame/value"
)

const jsonNull = "null"

const manifestSchema = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
const MCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

func parse(ctx context.Context, root *os.Root, digest framevalue.Digest) (plugin.Release, error) {
	builder, err := parseMetadata(ctx, root)
	if err != nil {
		return plugin.Release{}, err
	}
	return builder.Release(digest)
}

const extensionsMember = "extensions"

var authorFields = []string{"name", "email", "url"}

func manifestMemberRule(key string) (func(jsontext.Value) error, bool) {
	switch key {
	case "$schema", "name", "version", "description", "homepage", "repository", "license":
		return func(raw jsontext.Value) error { return requireText(raw, "manifest field", key) }, true
	case "keywords":
		return validateKeywords, true
	case "author":
		return validateAuthor, true
	case extensionsMember:
		return func(jsontext.Value) error { return nil }, true
	default:
		return nil, false
	}
}

func validateKeywords(raw jsontext.Value) error {
	var keywords []string
	if err := framevalue.DecodeValue(raw, &keywords, "plugin declaration"); err != nil {
		return fmt.Errorf("%w: manifest keywords: %w", plugin.ErrInvalid, err)
	}
	if keywords == nil {
		return fmt.Errorf("%w: invalid keywords", plugin.ErrInvalid)
	}
	return nil
}

func validateAuthor(raw jsontext.Value) error {
	var author map[string]jsontext.Value
	if err := json.Unmarshal(raw, &author); err != nil {
		return fmt.Errorf("%w: manifest author: %w", plugin.ErrInvalid, err)
	}
	if author == nil {
		return fmt.Errorf("%w: invalid author", plugin.ErrInvalid)
	}
	for field, value := range author {
		if !slices.Contains(authorFields, field) {
			return fmt.Errorf("%w: invalid author field %q", plugin.ErrInvalid, field)
		}
		if err := requireText(value, "author field", field); err != nil {
			return err
		}
	}
	return nil
}

func requireText(raw jsontext.Value, kind, name string) error {
	var text string
	if string(raw) == jsonNull {
		return fmt.Errorf("%w: invalid %s %q", plugin.ErrInvalid, kind, name)
	}
	if err := json.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("%w: %s %q: %w", plugin.ErrInvalid, kind, name, err)
	}
	return nil
}

func admitManifestFields(body []byte) (map[string]jsontext.Value, []plugin.Diagnostic, error) {
	var envelope map[string]jsontext.Value
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, fmt.Errorf("%w: manifest object: %w", plugin.ErrInvalid, err)
	}
	if envelope == nil {
		return nil, nil, fmt.Errorf("%w: manifest must be a JSON object", plugin.ErrInvalid)
	}
	var unknown []plugin.Diagnostic
	for _, key := range slices.Sorted(maps.Keys(envelope)) {
		validate, known := manifestMemberRule(key)
		if !known {
			unknown = append(unknown, plugin.Diagnostic{Component: plugin.Component{Kind: plugin.ComponentManifestField, Name: key}, Code: plugin.DiagnosticUnknownField})
			continue
		}
		if err := validate(envelope[key]); err != nil {
			return nil, nil, err
		}
	}
	return envelope, unknown, nil
}

func parseMetadata(ctx context.Context, root *os.Root) (*plugin.Builder, error) {
	body, err := read(ctx, root, pluginManifestFile, maxManifestBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errResourceLimit) {
			return nil, fmt.Errorf("%w: manifest: %w", plugin.ErrInvalid, err)
		}
		return nil, fmt.Errorf("pluginpackage: read manifest: %w", err)
	}
	envelope, unknown, err := admitManifestFields(body)
	if err != nil {
		return nil, err
	}
	var identity struct {
		Schema      string `json:"$schema"`
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &identity); err != nil {
		return nil, fmt.Errorf("%w: manifest identity: %w", plugin.ErrInvalid, err)
	}
	if identity.Schema != manifestSchema {
		return nil, fmt.Errorf("%w: unsupported manifest schema %q", plugin.ErrInvalid, identity.Schema)
	}
	release, err := plugin.NewBuilder(identity.Name, identity.Version, identity.Description)
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
	if raw, found := envelope[extensionsMember]; found {
		if err := parseExtensions(raw, release, ctx, root, nativeServers); err != nil {
			return nil, err
		}
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return nil, err
		}
	}
	if err = parseSkills(ctx, root, release); err != nil {
		return nil, err
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	return release, nil
}

func admitManagedBackends(raw jsontext.Value, release *plugin.Builder, root *os.Root, nativeServers []string) error {
	var values []wireManagedBackend
	if err := framevalue.DecodeValue(raw, &values, "plugin declaration"); err != nil {
		return fmt.Errorf("%w: managed backend declarations: %w", plugin.ErrInvalid, err)
	}
	for _, value := range values {
		if slices.Contains(nativeServers, value.ID.String()) {
			return fmt.Errorf("%w: server %s has native and managed startup declarations", plugin.ErrInvalid, value.ID)
		}
		server := plugin.Server{Name: value.ID, Startup: plugin.ManagedBackend{Runtime: value.Runtime, Entry: value.Entry}}
		if err := inspectServerFiles(root, server); err != nil {
			return fmt.Errorf("%w: managed backend %s: %w", plugin.ErrInvalid, value.ID, err)
		}
		if err := release.AdmitServer(server); err != nil {
			return fmt.Errorf("%w: managed backend %s: %w", plugin.ErrInvalid, value.ID, err)
		}
	}
	return nil
}
