package plugin

import (
	"fmt"
	"maps"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var themeColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var themeColorName = regexp.MustCompile(`^(?:background|foreground|accent|muted|border)$`)

func ThemeColorPattern() string     { return themeColor.String() }
func ThemeColorNamePattern() string { return themeColorName.String() }

func ValidName(name string) bool {
	return len(name) <= 64 && namePattern.MatchString(name) && !strings.Contains(name, "--") && !strings.Contains(name, "..")
}

const ContributionIDPattern = `^[a-z][a-z0-9._-]{0,63}$`
const DigestPattern = `^[0-9a-f]{64}$`

const (
	MaxServers     = 64
	MaxInputs      = 64
	MaxThemes      = 32
	MaxRequests    = 16
	MaxDiagnostics = 1024
)

var idPattern = regexp.MustCompile(ContributionIDPattern)

func (r Release) Validate() error {
	if !ValidDigest(r.Digest) {
		return fmt.Errorf("%w: release digest", ErrInvalid)
	}
	if !ValidName(r.Name) {
		return fmt.Errorf("%w: package name", ErrInvalid)
	}
	for _, limit := range []struct {
		component      string
		count, maximum int
	}{
		{"server", len(r.Servers), MaxServers}, {"input", len(r.Inputs), MaxInputs}, {"theme", len(r.Themes), MaxThemes}, {"skill", len(r.Skills), skills.MaxSkillsPerSource}, {"diagnostic", len(r.Diagnostics), MaxDiagnostics},
	} {
		if limit.count > limit.maximum {
			return fmt.Errorf("%w: %s capacity", ErrInvalid, limit.component)
		}
	}
	names := map[string]bool{}
	for _, s := range r.Servers {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("server %q: %w", s.Name, err)
		}
		if names[s.Name] {
			return fmt.Errorf("%w: duplicate server %q", ErrInvalid, s.Name)
		}
		names[s.Name] = true
	}
	if err := validateRequests(r.Requests); err != nil {
		return fmt.Errorf("capability requests: %w", err)
	}
	for _, request := range r.Requests {
		for _, target := range request.Targets {
			server, _, _ := strings.Cut(target, "/")
			if !names[server] {
				return fmt.Errorf("%w: capability target references unknown server %q", ErrInvalid, server)
			}
		}
	}
	ids := map[string]bool{}
	bindings := map[string]bool{}
	for _, input := range r.Inputs {
		if !idPattern.MatchString(input.ID) || ids[input.ID] {
			return fmt.Errorf("%w: input identity %q", ErrInvalid, input.ID)
		}
		ids[input.ID] = true
		index := slices.IndexFunc(r.Servers, func(server Server) bool { return server.Name == input.Server })
		if index < 0 {
			return fmt.Errorf("%w: input %q references an unknown server", ErrInvalid, input.ID)
		}
		server := r.Servers[index]
		key := input.Key
		switch input.Target {
		case Environment:
			if err := mcpserver.ValidateEnvironment(map[string]string{key: ""}); err != nil {
				return fmt.Errorf("%w: input %q: %w", ErrInvalid, input.ID, err)
			}
			if server.Type != Stdio || !environmentKey.MatchString(key) || strings.EqualFold(key, "PLUGIN_ROOT") || strings.EqualFold(key, "PLUGIN_DATA") {
				return fmt.Errorf("%w: input %q environment binding", ErrInvalid, input.ID)
			}
			key = strings.ToUpper(key)
			for static := range server.Env {
				if strings.EqualFold(static, key) {
					return fmt.Errorf("%w: input %q duplicates static environment configuration", ErrInvalid, input.ID)
				}
			}
		case Header:
			key = strings.ToLower(key)
			if server.Type != StreamableHTTP || (len(input.Key) > 128 || !mcpserver.ValidHeaderName(input.Key)) || key == "authorization" {
				return fmt.Errorf("%w: input %q header binding", ErrInvalid, input.ID)
			}
			if (strings.Contains(key, "token") || strings.Contains(key, "key")) && !input.Secret {
				return fmt.Errorf("%w: input %q credential classification", ErrInvalid, input.ID)
			}
			for static := range server.Headers {
				if strings.EqualFold(static, key) {
					return fmt.Errorf("%w: input %q duplicates static header configuration", ErrInvalid, input.ID)
				}
			}
		case Authorization:
			if server.Type != StreamableHTTP || key != "" || !input.Secret {
				return fmt.Errorf("%w: input %q authorization binding", ErrInvalid, input.ID)
			}
		default:
			return fmt.Errorf("%w: input %q target", ErrInvalid, input.ID)
		}
		binding := input.Server + "/" + string(input.Target) + "/" + key
		if bindings[binding] {
			return fmt.Errorf("%w: input %q duplicates a configuration binding", ErrInvalid, input.ID)
		}
		bindings[binding] = true
	}
	themes := map[string]bool{}
	for _, theme := range r.Themes {
		if !idPattern.MatchString(theme.ID) || themes[theme.ID] {
			return fmt.Errorf("%w: theme identity %q", ErrInvalid, theme.ID)
		}
		themes[theme.ID] = true
		if theme.Title == "" || len(theme.Title) > 128 {
			return fmt.Errorf("%w: theme %q title", ErrInvalid, theme.ID)
		}
		if theme.Scheme != Dark && theme.Scheme != Light {
			return fmt.Errorf("%w: theme %q scheme", ErrInvalid, theme.ID)
		}
		if len(theme.Colors) > 32 {
			return fmt.Errorf("%w: theme %q color capacity", ErrInvalid, theme.ID)
		}
		for _, key := range slices.Sorted(maps.Keys(theme.Colors)) {
			if !themeColorName.MatchString(key) || !themeColor.MatchString(theme.Colors[key]) {
				return fmt.Errorf("%w: theme %q color %q", ErrInvalid, theme.ID, key)
			}
		}
	}
	names = map[string]bool{}
	for _, skill := range r.Skills {
		if err := skills.ValidateName(skill.Name); err != nil {
			return fmt.Errorf("%w: skill identity: %w", ErrInvalid, err)
		}
		if names[skill.Name] {
			return fmt.Errorf("%w: duplicate skill %q", ErrInvalid, skill.Name)
		}
		names[skill.Name] = true
	}
	return nil
}
func validateRequests(requests []RequestGrant) error {
	if len(requests) > MaxRequests {
		return fmt.Errorf("%w: capability capacity", ErrInvalid)
	}
	caps := map[Capability]bool{}
	for _, r := range requests {
		if caps[r.Capability] {
			return fmt.Errorf("%w: duplicate capability %q", ErrInvalid, r.Capability)
		}
		if len(r.Targets) == 0 || len(r.Targets) > 128 {
			return fmt.Errorf("%w: capability %q target capacity", ErrInvalid, r.Capability)
		}
		caps[r.Capability] = true
		targets := map[string]bool{}
		for _, t := range r.Targets {
			if targets[t] {
				return fmt.Errorf("%w: duplicate capability target %q", ErrInvalid, t)
			}
			targets[t] = true
			switch r.Capability {
			case InvokeTools:
				parts := strings.Split(t, "/")
				if len(parts) != 2 {
					return fmt.Errorf("%w: tool target %q must name a server and tool", ErrInvalid, t)
				}
				if _, err := mcpserver.ParseServerName(parts[0]); err != nil {
					return fmt.Errorf("%w: tool target server: %w", ErrInvalid, err)
				}
				if _, err := mcpserver.ParseRemoteToolName(parts[1]); err != nil {
					return fmt.Errorf("%w: tool target name: %w", ErrInvalid, err)
				}
			default:
				return fmt.Errorf("%w: unknown capability %q", ErrInvalid, r.Capability)
			}
		}
	}
	return nil
}
func validateGrants(requests, grants []RequestGrant) error {
	if err := validateRequests(grants); err != nil {
		return err
	}
	for _, g := range grants {
		index := slices.IndexFunc(requests, func(r RequestGrant) bool { return r.Capability == g.Capability })
		if index < 0 {
			return fmt.Errorf("%w: capability %q was not requested", ErrInvalid, g.Capability)
		}
		for _, target := range g.Targets {
			if !slices.Contains(requests[index].Targets, target) {
				return fmt.Errorf("%w: capability target %q was not requested", ErrInvalid, target)
			}
		}
	}
	return nil
}

func (r Release) Clone() Release {
	r.Servers = slices.Clone(r.Servers)
	for index := range r.Servers {
		s := &r.Servers[index]
		s.Args = slices.Clone(s.Args)
		s.Env = maps.Clone(s.Env)
		s.Headers = maps.Clone(s.Headers)
	}
	r.Requests = slices.Clone(r.Requests)
	for index := range r.Requests {
		r.Requests[index].Targets = slices.Clone(r.Requests[index].Targets)
	}
	r.Inputs = slices.Clone(r.Inputs)
	r.Themes = slices.Clone(r.Themes)
	for index := range r.Themes {
		r.Themes[index].Colors = maps.Clone(r.Themes[index].Colors)
	}
	r.Skills = slices.Clone(r.Skills)
	r.Diagnostics = slices.Clone(r.Diagnostics)
	return r
}

var environmentKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func (s Server) Validate() error {
	name, err := mcpserver.ParseServerName(s.Name)
	if err != nil {
		return fmt.Errorf("%w: local server identity: %w", ErrInvalid, err)
	}
	if name.Installation() != "" {
		return fmt.Errorf("%w: local server identity", ErrInvalid)
	}
	for _, limit := range []struct {
		field          string
		count, maximum int
	}{
		{"args", len(s.Args), 128}, {"env", len(s.Env), 128}, {"headers", len(s.Headers), 128},
		{"command", len(s.Command), 1024}, {"cwd", len(s.CWD), 1024}, {"url", len(s.URL), 8192},
	} {
		if limit.count > limit.maximum {
			return fmt.Errorf("%w: server %s capacity", ErrInvalid, limit.field)
		}
	}
	var transport mcpserver.Transport
	switch s.Type {
	case Stdio:
		transport = mcpserver.TransportStdio
	case StreamableHTTP:
		transport = mcpserver.TransportStreamableHTTP
	default:
		return fmt.Errorf("%w: package server transport %q", ErrInvalid, s.Type)
	}
	connection := mcpserver.Server{Name: name, Transport: transport, Command: s.Command, Args: s.Args, Env: s.Env, Dir: s.CWD, URL: s.URL, Headers: s.Headers}
	if err := connection.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	for index, value := range s.Args {
		if len(value) > 8192 || !utf8.ValidString(value) {
			return fmt.Errorf("%w: server argument %d", ErrInvalid, index)
		}
	}
	environmentKeys := map[string]bool{}
	for key, value := range s.Env {
		if !environmentKey.MatchString(key) || strings.EqualFold(key, "PLUGIN_ROOT") || strings.EqualFold(key, "PLUGIN_DATA") {
			return fmt.Errorf("%w: package environment key %q", ErrInvalid, key)
		}
		if len(value) > 8192 || !utf8.ValidString(value) {
			return fmt.Errorf("%w: package environment value for %q", ErrInvalid, key)
		}
		canonical := strings.ToUpper(key)
		if environmentKeys[canonical] {
			return fmt.Errorf("%w: ambiguous portable environment key %q", ErrInvalid, key)
		}
		environmentKeys[canonical] = true
	}
	for key, value := range s.Headers {
		canonical := strings.ToLower(key)
		if len(key) > 128 || strings.Contains(canonical, "token") || strings.Contains(canonical, "key") {
			return fmt.Errorf("%w: static header %q requires a declared credential input", ErrInvalid, key)
		}
		if len(value) > 8192 || !utf8.ValidString(value) {
			return fmt.Errorf("%w: static header value for %q", ErrInvalid, key)
		}
	}
	switch s.Type {
	case Stdio:
		if !utf8.ValidString(s.Command) {
			return fmt.Errorf("%w: command encoding", ErrInvalid)
		}
		if strings.HasPrefix(s.Command, "./") {
			if !ValidResourcePath(s.Command[2:]) {
				return fmt.Errorf("%w: package command path", ErrInvalid)
			}
		} else if strings.ContainsAny(s.Command, "/\\ \t\r\n$") {
			return fmt.Errorf("%w: bare command name", ErrInvalid)
		}
		if s.CWD == "" || s.CWD == "${PLUGIN_ROOT}" || s.CWD == "${PLUGIN_DATA}" {
			return nil
		}
		var relative string
		switch {
		case strings.HasPrefix(s.CWD, "./"):
			relative = s.CWD[2:]
		case strings.HasPrefix(s.CWD, "${PLUGIN_ROOT}/"):
			relative = strings.TrimPrefix(s.CWD, "${PLUGIN_ROOT}/")
		case strings.HasPrefix(s.CWD, "${PLUGIN_DATA}/"):
			relative = strings.TrimPrefix(s.CWD, "${PLUGIN_DATA}/")
		default:
			return fmt.Errorf("%w: package working directory", ErrInvalid)
		}
		if !ValidResourcePath(relative) {
			return fmt.Errorf("%w: package working directory path", ErrInvalid)
		}
	case StreamableHTTP:
		u, err := url.Parse(s.URL)
		if err != nil || u.User != nil || u.Fragment != "" || u.Host == "" || !slices.Contains([]string{"http", "https"}, u.Scheme) {
			return fmt.Errorf("%w: package server URL", ErrInvalid)
		}
		ip, ipErr := netip.ParseAddr(u.Hostname())
		if u.Scheme != "https" && u.Hostname() != "localhost" && (ipErr != nil || !ip.IsLoopback()) {
			return fmt.Errorf("%w: package server requires HTTPS outside loopback", ErrInvalid)
		}
	}
	return nil
}
