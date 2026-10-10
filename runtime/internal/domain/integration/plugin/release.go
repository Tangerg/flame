package plugin

import (
	"fmt"
	"maps"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var themeColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func ThemeColorPattern() string { return themeColor.String() }

func ValidName(name string) bool {
	return len(name) <= 64 && namePattern.MatchString(name) && !strings.Contains(name, "--") && !strings.Contains(name, "..")
}

const ContributionIDPattern = `^[a-z][a-z0-9._-]{0,63}$`

const (
	MaxServers     = 64
	MaxInputs      = 64
	MaxThemes      = 32
	MaxViews       = 16
	MaxActions     = 16
	MaxDiagnostics = 1024
)

var idPattern = regexp.MustCompile(ContributionIDPattern)

// Server is a package-declared MCP server in the MCP registry's transport
// vocabulary. Command and Dir keep the package-relative spelling; the package
// adapter resolves them against the release directory at realization.
type Server struct {
	Name      mcpserver.ServerName
	Transport mcpserver.Transport
	Command   string
	Args      []string
	Env       map[string]string
	Dir       string
	URL       string
	Headers   map[string]string
}

// Input is a value the user supplies for one server. Secrecy is declared, but
// a header or authorization input is admitted only as a secret: anything sent
// to an endpoint as a header is treated as a credential.
type Input struct {
	ID       string
	Secret   bool
	Required bool
	Server   mcpserver.ServerName
	Target   InputTarget
	Key      string
}

type Theme struct {
	ID     string
	Title  string
	Scheme ThemeScheme
	Colors ThemeColors
}

// ThemeColors names the closed set of colors a theme may override. An empty
// color is not overridden.
type ThemeColors struct {
	Background string
	Foreground string
	Accent     string
	Muted      string
	Border     string
}

func (c ThemeColors) validate() error {
	for name, value := range map[string]string{"background": c.Background, "foreground": c.Foreground, "accent": c.Accent, "muted": c.Muted, "border": c.Border} {
		if value != "" && !themeColor.MatchString(value) {
			return fmt.Errorf("color %q", name)
		}
	}
	return nil
}

type Skill struct {
	Name        string
	Description string
}

// Declaration is the plain data a package declares. It carries no trust of its
// own: only [NewRelease] or a [Builder] admits it into a [Release].
type Declaration struct {
	Name        string
	Version     string
	Description string
	Servers     []Server
	Inputs      []Input
	Themes      []Theme
	Views       []ViewDeclaration
	Actions     []Action
	Skills      []Skill
	Diagnostics []Diagnostic
}

func (d Declaration) clone() Declaration {
	d.Servers = slices.Clone(d.Servers)
	for index := range d.Servers {
		d.Servers[index] = d.Servers[index].clone()
	}
	d.Inputs = slices.Clone(d.Inputs)
	d.Themes = slices.Clone(d.Themes)
	d.Views = slices.Clone(d.Views)
	for index := range d.Views {
		d.Views[index] = d.Views[index].clone()
	}
	d.Actions = slices.Clone(d.Actions)
	d.Skills = slices.Clone(d.Skills)
	d.Diagnostics = slices.Clone(d.Diagnostics)
	return d
}

func (s Server) clone() Server {
	s.Args = slices.Clone(s.Args)
	s.Env = maps.Clone(s.Env)
	s.Headers = maps.Clone(s.Headers)
	return s
}

// Release is an admitted, immutable declaration of exact package bytes. It is
// validated once on construction; persistence, installations and projections
// trust it afterwards and read it only through copies.
type Release struct {
	digest      fingerprint.Digest
	declaration Declaration
}

// NewRelease admits a complete declaration, each member exactly once.
func NewRelease(digest fingerprint.Digest, declaration Declaration) (Release, error) {
	builder, err := NewBuilder(declaration.Name, declaration.Version, declaration.Description)
	if err != nil {
		return Release{}, err
	}
	for _, server := range declaration.Servers {
		if err := builder.AdmitServer(server); err != nil {
			return Release{}, err
		}
	}
	for _, input := range declaration.Inputs {
		if err := builder.AdmitInput(input); err != nil {
			return Release{}, err
		}
	}
	for _, theme := range declaration.Themes {
		if err := builder.AdmitTheme(theme); err != nil {
			return Release{}, err
		}
	}
	for _, view := range declaration.Views {
		if err := builder.AdmitView(view); err != nil {
			return Release{}, err
		}
	}
	for _, action := range declaration.Actions {
		if err := builder.AdmitAction(action); err != nil {
			return Release{}, err
		}
	}
	for _, skill := range declaration.Skills {
		if err := builder.AdmitSkill(skill); err != nil {
			return Release{}, err
		}
	}
	for _, diagnostic := range declaration.Diagnostics {
		builder.Report(diagnostic)
	}
	return builder.Release(digest)
}

func (r Release) Digest() fingerprint.Digest { return r.digest }
func (r Release) Name() string               { return r.declaration.Name }

// Declaration returns an owned copy; no reader can advance the release.
func (r Release) Declaration() Declaration { return r.declaration.clone() }

func (r Release) server(name mcpserver.ServerName) (Server, bool) {
	index := slices.IndexFunc(r.declaration.Servers, func(server Server) bool { return server.Name == name })
	if index < 0 {
		return Server{}, false
	}
	return r.declaration.Servers[index], true
}

func (r Release) input(id string) (Input, bool) {
	index := slices.IndexFunc(r.declaration.Inputs, func(input Input) bool { return input.ID == id })
	if index < 0 {
		return Input{}, false
	}
	return r.declaration.Inputs[index], true
}

// recipient names who receives a credential configured for a server. For an
// endpoint it is the declared transport, URL and static headers: a credential
// keeps reaching the same service while they are unchanged, whatever else the
// release changes. For a process it is the executable itself, which only the
// exact release bytes and the server's name within them identify. Static
// secret inputs and OAuth grants both follow this one rule.
func (r Release) recipient(name mcpserver.ServerName) (fingerprint.Digest, bool) {
	server, found := r.server(name)
	if !found {
		return fingerprint.Digest{}, false
	}
	fields := []string{string(server.Transport)}
	switch server.Transport {
	case mcpserver.TransportStreamableHTTP:
		fields = append(fields, server.URL)
		for _, key := range slices.Sorted(maps.Keys(server.Headers)) {
			fields = append(fields, key, server.Headers[key])
		}
	default:
		fields = append(fields, r.digest.String(), name.String())
	}
	return fingerprint.Strings(fields...), true
}

func (r Release) declaresSkill(name string) bool {
	return slices.ContainsFunc(r.declaration.Skills, func(skill Skill) bool { return skill.Name == name })
}

// Builder admits a declaration one contribution at a time. Each contribution is
// validated once against what was already admitted, so a package adapter can
// isolate an invalid member without revalidating the members before it.
type Builder struct {
	declaration Declaration
	bindings    map[string]bool
}

func NewBuilder(name, version, description string) (*Builder, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("%w: package name", ErrInvalid)
	}
	return &Builder{declaration: Declaration{Name: name, Version: version, Description: description}, bindings: map[string]bool{}}, nil
}

// Report records an admission finding. Findings are validated with the
// release, because a finding never decides whether a member is admitted.
func (b *Builder) Report(diagnostic Diagnostic) {
	b.declaration.Diagnostics = append(b.declaration.Diagnostics, diagnostic)
}

func (b *Builder) AdmitServer(server Server) error {
	if len(b.declaration.Servers) >= MaxServers {
		return refuse(DiagnosticComponentLimit, "server capacity")
	}
	if _, found := b.release().server(server.Name); found {
		return refuse(DiagnosticInvalidDeclaration, "duplicate server %q", server.Name.String())
	}
	if err := server.validate(); err != nil {
		return &Refusal{Code: DiagnosticInvalidDeclaration, cause: fmt.Errorf("server %q: %w", server.Name.String(), err)}
	}
	b.declaration.Servers = append(b.declaration.Servers, server.clone())
	return nil
}

func (b *Builder) AdmitInput(input Input) error {
	if len(b.declaration.Inputs) >= MaxInputs {
		return refuse(DiagnosticComponentLimit, "input capacity")
	}
	release := b.release()
	if _, found := release.input(input.ID); found || !idPattern.MatchString(input.ID) {
		return refuse(DiagnosticInvalidDeclaration, "input identity %q", input.ID)
	}
	server, found := release.server(input.Server)
	if !found {
		return refuse(DiagnosticInvalidDependencies, "input %q references an unknown server", input.ID)
	}
	binding, err := input.binding(server)
	if err != nil {
		return err
	}
	if b.bindings[binding] {
		return refuse(DiagnosticInvalidDeclaration, "input %q duplicates a configuration binding", input.ID)
	}
	b.bindings[binding] = true
	b.declaration.Inputs = append(b.declaration.Inputs, input)
	return nil
}

func (b *Builder) AdmitTheme(theme Theme) error {
	if len(b.declaration.Themes) >= MaxThemes {
		return refuse(DiagnosticComponentLimit, "theme capacity")
	}
	duplicate := slices.ContainsFunc(b.declaration.Themes, func(existing Theme) bool { return existing.ID == theme.ID })
	if duplicate || !idPattern.MatchString(theme.ID) {
		return refuse(DiagnosticInvalidDeclaration, "theme identity %q", theme.ID)
	}
	if theme.Title == "" || len(theme.Title) > 128 {
		return refuse(DiagnosticInvalidDeclaration, "theme %q title", theme.ID)
	}
	if theme.Scheme != Dark && theme.Scheme != Light {
		return refuse(DiagnosticInvalidDeclaration, "theme %q scheme", theme.ID)
	}
	if err := theme.Colors.validate(); err != nil {
		return refuse(DiagnosticInvalidDeclaration, "theme %q %v", theme.ID, err)
	}
	b.declaration.Themes = append(b.declaration.Themes, theme)
	return nil
}

func (b *Builder) AdmitSkill(skill Skill) error {
	if len(b.declaration.Skills) >= skills.MaxSkillsPerSource {
		return refuse(DiagnosticComponentLimit, "skill capacity")
	}
	if err := skills.ValidateName(skill.Name); err != nil {
		return &Refusal{Code: DiagnosticInvalidDeclaration, cause: fmt.Errorf("%w: skill identity: %w", ErrInvalid, err)}
	}
	if b.release().declaresSkill(skill.Name) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate skill %q", skill.Name)
	}
	b.declaration.Skills = append(b.declaration.Skills, skill)
	return nil
}

// Release binds the admitted declaration to the digest of its exact bytes.
func (b *Builder) Release(digest fingerprint.Digest) (Release, error) {
	if err := digest.Validate(); err != nil {
		return Release{}, fmt.Errorf("%w: release digest: %w", ErrInvalid, err)
	}
	if len(b.declaration.Diagnostics) > MaxDiagnostics {
		return Release{}, fmt.Errorf("%w: diagnostic capacity", ErrInvalid)
	}
	for index, diagnostic := range b.declaration.Diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return Release{}, fmt.Errorf("diagnostic %d: %w", index, err)
		}
	}
	return Release{digest: digest, declaration: b.declaration.clone()}, nil
}

func (b *Builder) release() Release { return Release{declaration: b.declaration} }

// binding names the configuration slot an input fills on its server, in the
// canonical case of the target, so two spellings cannot fill one slot.
func (i Input) binding(server Server) (string, error) {
	key := i.Key
	switch i.Target {
	case Environment:
		if server.Transport != mcpserver.TransportStdio {
			return "", refuse(DiagnosticInvalidDependencies, "input %q binds an environment variable of a non-stdio server", i.ID)
		}
		if err := validatePackageEnvironment(map[string]string{key: ""}); err != nil {
			return "", &Refusal{Code: DiagnosticInvalidDeclaration, cause: fmt.Errorf("input %q: %w", i.ID, err)}
		}
		key = strings.ToUpper(key)
		for static := range server.Env {
			if strings.EqualFold(static, key) {
				return "", refuse(DiagnosticInvalidDependencies, "input %q duplicates static environment configuration", i.ID)
			}
		}
	case Header:
		if server.Transport != mcpserver.TransportStreamableHTTP {
			return "", refuse(DiagnosticInvalidDependencies, "input %q binds a header of a non-HTTP server", i.ID)
		}
		if err := mcpserver.ValidateHTTPHeaders("", map[string]string{key: ""}); err != nil {
			return "", refuse(DiagnosticInvalidDeclaration, "input %q header: %v", i.ID, err)
		}
		if !i.Secret {
			return "", refuse(DiagnosticInvalidDeclaration, "input %q header must be a secret", i.ID)
		}
		key = strings.ToLower(key)
		for static := range server.Headers {
			if strings.EqualFold(static, key) {
				return "", refuse(DiagnosticInvalidDependencies, "input %q duplicates static header configuration", i.ID)
			}
		}
	case Authorization:
		if server.Transport != mcpserver.TransportStreamableHTTP {
			return "", refuse(DiagnosticInvalidDependencies, "input %q binds authorization of a non-HTTP server", i.ID)
		}
		if key != "" || !i.Secret {
			return "", refuse(DiagnosticInvalidDeclaration, "input %q authorization must be an unkeyed secret", i.ID)
		}
	default:
		return "", refuse(DiagnosticInvalidDeclaration, "input %q target", i.ID)
	}
	return i.Server.String() + "/" + string(i.Target) + "/" + key, nil
}

// validate checks the connection rules the MCP registry owns, then the few
// rules that exist only because the declaration comes from a portable package.
func (s Server) validate() error {
	if err := s.Name.Validate(); err != nil {
		return fmt.Errorf("%w: local server identity: %w", ErrInvalid, err)
	}
	connection := mcpserver.Server{Name: s.Name, Transport: s.Transport, Command: s.Command, Args: s.Args, Env: s.Env, Dir: s.Dir, URL: s.URL, Headers: s.Headers}
	if err := connection.ValidateConnection(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	switch s.Transport {
	case mcpserver.TransportStdio:
		if err := validatePackageEnvironment(s.Env); err != nil {
			return err
		}
		if err := validatePackageCommand(s); err != nil {
			return err
		}
		_, err := parseWorkingDirectory(s.Dir)
		return err
	default:
		return validatePackageEndpoint(s.URL)
	}
}

// validatePackageEnvironment reserves the variables only the Runtime binds and
// refuses names that collide on case-insensitive platforms.
func validatePackageEnvironment(environment map[string]string) error {
	if err := mcpserver.ValidateEnvironment(environment); err != nil {
		return fmt.Errorf("%w: package environment: %w", ErrInvalid, err)
	}
	canonical := map[string]bool{}
	for key := range environment {
		upper := strings.ToUpper(key)
		if upper == RootVariable || upper == DataVariable {
			return fmt.Errorf("%w: package environment key %q is reserved", ErrInvalid, key)
		}
		if canonical[upper] {
			return fmt.Errorf("%w: ambiguous portable environment key %q", ErrInvalid, key)
		}
		canonical[upper] = true
	}
	return nil
}

// validatePackageCommand admits a file inside the package or a bare command
// name resolved from PATH; a package cannot name a host path.
func validatePackageCommand(s Server) error {
	if packaged, found := s.PackagedCommand(); found {
		if !ValidResourcePath(packaged) {
			return fmt.Errorf("%w: package command path", ErrInvalid)
		}
		return nil
	}
	if strings.ContainsAny(s.Command, "/\\ \t\r\n$") {
		return fmt.Errorf("%w: bare command name", ErrInvalid)
	}
	return nil
}

// validatePackageEndpoint refuses URL user information, which is structurally
// a credential, and plaintext transport beyond the local machine. Package
// bytes are public content: other static values, such as headers, are never
// treated as secrets, and a credential reaches a server only through an input.
func validatePackageEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: package server URL: %w", ErrInvalid, err)
	}
	if u.User != nil || u.Fragment != "" {
		return fmt.Errorf("%w: package server URL carries credentials or a fragment", ErrInvalid)
	}
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: package server URL must be an absolute HTTP(S) endpoint", ErrInvalid)
	}
	if u.Scheme != "https" && !loopbackHost(u.Hostname()) {
		return fmt.Errorf("%w: package server requires HTTPS outside loopback", ErrInvalid)
	}
	return nil
}

func loopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}
