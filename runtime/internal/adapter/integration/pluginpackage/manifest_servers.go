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

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framevalue "github.com/Tangerg/frame/value"
)

func parseMCP(ctx context.Context, root *os.Root, release *plugin.Builder) ([]string, error) {
	body, err := read(ctx, root, mcpManifestFile, maxManifestBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
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
		if err == nil {
			if err := requirePackagedServer(name, server); err != nil {
				return nil, err
			}
			err = admitPackagedServer(root, release, server)
		}
		if limited := reportServerAdmission(release, name, err); limited {
			return names, nil
		}
	}
	return names, nil
}

func requirePackagedServer(name string, server plugin.Server) error {
	if server.Transport() != mcpserver.TransportStdio {
		return fmt.Errorf("%w: installed server %q requires a packaged process; configure remote MCP through the host registry", plugin.ErrInvalid, name)
	}
	if _, packaged := server.PackagedCommand(); !packaged {
		return fmt.Errorf("%w: installed server %q requires a packaged executable", plugin.ErrInvalid, name)
	}
	return nil
}

func admitPackagedServer(root *os.Root, release *plugin.Builder, server plugin.Server) error {
	if err := inspectServerFiles(root, server); err != nil {
		return err
	}
	return release.AdmitServer(server)
}

func reportServerAdmission(release *plugin.Builder, name string, err error) bool {
	code, admitted := admission(err)
	switch {
	case admitted:
		return false
	case code == plugin.DiagnosticComponentLimit:
		report(release, plugin.Component{Kind: plugin.ComponentMCP}, code)
		return true
	default:
		report(release, plugin.Component{Kind: plugin.ComponentMCPServer, Name: name}, code)
		return false
	}
}

func portableServer(name string, raw jsontext.Value) (plugin.Server, error) {
	var wire struct {
		Type    mcpserver.Transport `json:"type"`
		Command string              `json:"command,omitempty"`
		Args    []string            `json:"args,omitempty"`
		Env     map[string]string   `json:"env,omitempty"`
		CWD     string              `json:"cwd,omitempty"`
		URL     string              `json:"url,omitempty"`
		Headers map[string]string   `json:"headers,omitempty"`
	}
	if err := framevalue.DecodeValue(raw, &wire, "plugin declaration"); err != nil {
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
	case mcpserver.TransportStdio:
		server.Startup = plugin.DeclaredConnection{Connection: mcpserver.StdioConnection{Command: wire.Command, Args: wire.Args, Env: wire.Env, Dir: wire.CWD}}
		allowed = []string{"type", "command", "args", "env", "cwd"}
	case mcpserver.TransportStreamableHTTP:
		server.Startup = plugin.DeclaredConnection{Connection: mcpserver.HTTPConnection{URL: wire.URL, Headers: wire.Headers}}
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

func inspectServerFiles(root *os.Root, s plugin.Server) error {
	packaged, found := s.PackagedCommand()
	if managed, ok := s.Startup.(plugin.ManagedBackend); ok {
		packaged, found = managed.Entry, true
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
