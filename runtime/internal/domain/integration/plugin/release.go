package plugin

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/process"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/qimu/integrity"
	skillspec "github.com/Tangerg/scope/skills"
)

var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var themeColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func ThemeColorPattern() string { return themeColor.String() }

func validName(name string) bool {
	return len(name) <= 64 && namePattern.MatchString(name) && !strings.Contains(name, "--") && !strings.Contains(name, "..")
}

const (
	maxServers     = 64
	maxInputs      = 64
	maxThemes      = 32
	maxDiagnostics = 1024
)

// Server is a package-declared MCP server in the MCP registry's transport
// vocabulary. Command and Dir keep the package-relative spelling; the package
// adapter resolves them against the release directory at realization.
type Server struct {
	Name    mcpserver.ServerName
	Startup ServerStartup
}

// ServerStartup is how a package starts or reaches one server: a backend the
// Runtime launches with its pinned interpreter, or a connection the package
// declares itself.
type ServerStartup interface {
	Transport() mcpserver.Transport
	clone() ServerStartup
}

// ManagedBackend is a Node ESM entry the Runtime starts over stdio.
type ManagedBackend struct {
	Runtime ManagedRuntime
	Entry   string
}

// DeclaredConnection is an MCP connection written in the package itself.
type DeclaredConnection struct{ mcpserver.Connection }

func (ManagedBackend) Transport() mcpserver.Transport { return mcpserver.TransportStdio }

func (m ManagedBackend) clone() ServerStartup { return m }

func (d DeclaredConnection) clone() ServerStartup {
	return DeclaredConnection{Connection: mcpserver.Server{Connection: d.Connection}.Clone().Connection}
}

func (s Server) Transport() mcpserver.Transport { return s.Startup.Transport() }

// stdio reports the package-written process connection, when there is one.
func (s Server) stdio() (mcpserver.StdioConnection, bool) {
	declared, ok := s.Startup.(DeclaredConnection)
	if !ok {
		return mcpserver.StdioConnection{}, false
	}
	connection, ok := declared.Connection.(mcpserver.StdioConnection)
	return connection, ok
}

// http reports the package-written HTTP connection, when there is one.
func (s Server) http() (mcpserver.HTTPConnection, bool) {
	declared, ok := s.Startup.(DeclaredConnection)
	if !ok {
		return mcpserver.HTTPConnection{}, false
	}
	connection, ok := declared.Connection.(mcpserver.HTTPConnection)
	return connection, ok
}

type ManagedRuntime string

const node ManagedRuntime = "node"

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
	for _, color := range []struct{ name, value string }{
		{"background", c.Background}, {"foreground", c.Foreground},
		{"accent", c.Accent}, {"muted", c.Muted}, {"border", c.Border},
	} {
		if color.value != "" && !themeColor.MatchString(color.value) {
			return fmt.Errorf("color %q", color.name)
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
	Name          string
	Version       string
	Description   string
	Servers       []Server
	Contributions []Contribution
	Points        []Point
	PointValues   []PointValue
	Components    []ClientComponent
	Inputs        []Input
	Themes        []Theme
	Locales       []Locale
	Skills        []Skill
	Diagnostics   []Diagnostic
}

func (d Declaration) clone() Declaration {
	d.Points = slices.Clone(d.Points)
	d.PointValues = slices.Clone(d.PointValues)
	d.Components = slices.Clone(d.Components)
	for i := range d.Components {
		if d.Components[i].Selector != nil {
			copy := *d.Components[i].Selector
			d.Components[i].Selector = &copy
		}
	}
	d.Servers = slices.Clone(d.Servers)
	for index := range d.Servers {
		d.Servers[index] = d.Servers[index].clone()
	}
	d.Contributions = slices.Clone(d.Contributions)
	for index := range d.Contributions {
		d.Contributions[index] = d.Contributions[index].clone()
	}
	d.Inputs = slices.Clone(d.Inputs)
	d.Themes = slices.Clone(d.Themes)
	d.Locales = slices.Clone(d.Locales)
	d.Skills = slices.Clone(d.Skills)
	d.Diagnostics = slices.Clone(d.Diagnostics)
	return d
}

func (s Server) clone() Server {
	if s.Startup != nil {
		s.Startup = s.Startup.clone()
	}
	return s
}

// Release is an admitted, immutable declaration of exact package bytes. It is
// validated once on construction; persistence, installations and projections
// trust it afterwards and read it only through copies.
type Release struct {
	digest      integrity.Digest
	declaration Declaration
}

// NewRelease admits a complete declaration, each member exactly once.
func NewRelease(digest integrity.Digest, declaration Declaration) (Release, error) {
	builder, err := NewBuilder(declaration.Name, declaration.Version, declaration.Description)
	if err != nil {
		return Release{}, err
	}
	for _, server := range declaration.Servers {
		if err := builder.AdmitServer(server); err != nil {
			return Release{}, err
		}
	}
	for _, contribution := range declaration.Contributions {
		if err := builder.AdmitContribution(contribution); err != nil {
			return Release{}, err
		}
	}
	for _, value := range declaration.Points {
		if err := builder.AdmitPoint(value); err != nil {
			return Release{}, err
		}
	}
	for _, value := range declaration.PointValues {
		if err := builder.AdmitPointValue(value); err != nil {
			return Release{}, err
		}
	}
	for _, value := range declaration.Components {
		if err := builder.AdmitComponent(value); err != nil {
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
	for _, locale := range declaration.Locales {
		if err := builder.AdmitLocale(locale); err != nil {
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

func (r Release) Digest() integrity.Digest { return r.digest }
func (r Release) Name() string             { return r.declaration.Name }

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
func (r Release) recipient(name mcpserver.ServerName) (integrity.Digest, bool) {
	server, found := r.server(name)
	if !found {
		return integrity.Digest{}, false
	}
	fields := []string{string(server.Transport())}
	if managed, ok := server.Startup.(ManagedBackend); ok {
		fields = append(fields, string(managed.Runtime), managed.Entry)
	}
	if http, ok := server.http(); ok {
		fields = append(fields, http.URL)
		for _, key := range slices.Sorted(maps.Keys(http.Headers)) {
			fields = append(fields, key, http.Headers[key])
		}
		return identity.DigestStrings(fields...), true
	}
	fields = append(fields, r.digest.String(), name.String())
	return identity.DigestStrings(fields...), true
}

func (r Release) declaresSkill(name string) bool {
	return slices.ContainsFunc(r.declaration.Skills, func(skill Skill) bool { return skill.Name == name })
}

// Builder admits a declaration one contribution at a time. Each contribution is
// validated once against what was already admitted, so a package adapter can
// isolate an invalid member without revalidating the members before it.
type Builder struct {
	declaration Declaration
	bindings    map[inputSlot]bool
}

func NewBuilder(name, version, description string) (*Builder, error) {
	if !validName(name) {
		return nil, fmt.Errorf("%w: package name", ErrInvalid)
	}
	return &Builder{declaration: Declaration{Name: name, Version: version, Description: description}, bindings: map[inputSlot]bool{}}, nil
}

// Report records an admission finding. Findings are validated with the
// release, because a finding never decides whether a member is admitted.
func (b *Builder) Report(diagnostic Diagnostic) {
	b.declaration.Diagnostics = append(b.declaration.Diagnostics, diagnostic)
}

func (b *Builder) AdmitServer(server Server) error {
	if len(b.declaration.Servers) >= maxServers {
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
	if len(b.declaration.Inputs) >= maxInputs {
		return refuse(DiagnosticComponentLimit, "input capacity")
	}
	release := b.release()
	if _, found := release.input(input.ID); found || !identity.ValidDeclarationID(input.ID) {
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
	if len(b.declaration.Themes) >= maxThemes {
		return refuse(DiagnosticComponentLimit, "theme capacity")
	}
	duplicate := slices.ContainsFunc(b.declaration.Themes, func(existing Theme) bool { return existing.ID == theme.ID })
	if duplicate || !identity.ValidDeclarationID(theme.ID) {
		return refuse(DiagnosticInvalidDeclaration, "theme identity %q", theme.ID)
	}
	if theme.Title == "" || len(theme.Title) > MaxPresentationTitleBytes || !utf8.ValidString(theme.Title) {
		return refuse(DiagnosticInvalidDeclaration, "theme %q title", theme.ID)
	}
	if theme.Scheme != dark && theme.Scheme != light {
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
	if err := (skillspec.Summary{Name: skill.Name, Description: skill.Description}).Validate(); err != nil {
		return &Refusal{Code: DiagnosticInvalidDeclaration, cause: fmt.Errorf("%w: skill metadata: %w", ErrInvalid, err)}
	}
	if b.release().declaresSkill(skill.Name) {
		return refuse(DiagnosticInvalidDeclaration, "duplicate skill %q", skill.Name)
	}
	b.declaration.Skills = append(b.declaration.Skills, skill)
	return nil
}

// Release binds the admitted declaration to the digest of its exact bytes.
func (b *Builder) Release(digest integrity.Digest) (Release, error) {
	if err := digest.Validate(); err != nil {
		return Release{}, fmt.Errorf("%w: release digest: %w", ErrInvalid, err)
	}
	declaration, err := b.Declaration()
	if err != nil {
		return Release{}, err
	}
	return Release{digest: digest, declaration: declaration}, nil
}

// Declaration finishes metadata admission without inventing a release identity.
// Author tooling binds the result only after Qimu has computed the manifest.
func (b *Builder) Declaration() (Declaration, error) {
	if len(b.declaration.Diagnostics) > maxDiagnostics {
		return Declaration{}, fmt.Errorf("%w: diagnostic capacity", ErrInvalid)
	}
	for index, diagnostic := range b.declaration.Diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return Declaration{}, fmt.Errorf("diagnostic %d: %w", index, err)
		}
	}
	return b.declaration.clone(), nil
}

func (b *Builder) release() Release { return Release{declaration: b.declaration} }

// managedNodeLaunchVariables steer how Node loads code; a managed backend's
// launch policy owns them, so a user input may not supply them.
var managedNodeLaunchVariables = []string{"NODE_OPTIONS", "NODE_PATH", "NODE_REPL_EXTERNAL_MODULE"}

// inputSlot is the configuration slot an input fills on its server.
type inputSlot struct {
	server mcpserver.ServerName
	target InputTarget
	key    string
}

// binding names the configuration slot an input fills on its server, in the
// canonical case of the target, so two spellings cannot fill one slot.
func (i Input) binding(server Server) (inputSlot, error) {
	key := i.Key
	switch i.Target {
	case Environment:
		if server.Transport() != mcpserver.TransportStdio {
			return inputSlot{}, refuse(DiagnosticInvalidDependencies, "input %q binds an environment variable of a non-stdio server", i.ID)
		}
		_, managed := server.Startup.(ManagedBackend)
		if managed && slices.Contains(managedNodeLaunchVariables, strings.ToUpper(key)) {
			return inputSlot{}, refuse(DiagnosticInvalidDependencies, "input %q replaces managed node launch policy", i.ID)
		}
		if err := validatePackageEnvironment(map[string]string{key: ""}); err != nil {
			return inputSlot{}, &Refusal{Code: DiagnosticInvalidDeclaration, cause: fmt.Errorf("input %q: %w", i.ID, err)}
		}
		key = strings.ToUpper(key)
		stdio, _ := server.stdio()
		for static := range stdio.Env {
			if strings.EqualFold(static, key) {
				return inputSlot{}, refuse(DiagnosticInvalidDependencies, "input %q duplicates static environment configuration", i.ID)
			}
		}
	case Header:
		if server.Transport() != mcpserver.TransportStreamableHTTP {
			return inputSlot{}, refuse(DiagnosticInvalidDependencies, "input %q binds a header of a non-HTTP server", i.ID)
		}
		if err := mcpserver.ValidateHTTPHeaders("", map[string]string{key: ""}); err != nil {
			return inputSlot{}, refuse(DiagnosticInvalidDeclaration, "input %q header: %v", i.ID, err)
		}
		if !i.Secret {
			return inputSlot{}, refuse(DiagnosticInvalidDeclaration, "input %q header must be a secret", i.ID)
		}
		key = strings.ToLower(key)
		http, _ := server.http()
		for static := range http.Headers {
			if strings.EqualFold(static, key) {
				return inputSlot{}, refuse(DiagnosticInvalidDependencies, "input %q duplicates static header configuration", i.ID)
			}
		}
	case Authorization:
		if server.Transport() != mcpserver.TransportStreamableHTTP {
			return inputSlot{}, refuse(DiagnosticInvalidDependencies, "input %q binds authorization of a non-HTTP server", i.ID)
		}
		if key != "" || !i.Secret {
			return inputSlot{}, refuse(DiagnosticInvalidDeclaration, "input %q authorization must be an unkeyed secret", i.ID)
		}
	default:
		return inputSlot{}, refuse(DiagnosticInvalidDeclaration, "input %q target", i.ID)
	}
	return inputSlot{server: i.Server, target: i.Target, key: key}, nil
}

// validate checks the connection rules the MCP registry owns, then the few
// rules that exist only because the declaration comes from a portable package.
func (s Server) validate() error {
	if err := s.Name.Validate(); err != nil {
		return fmt.Errorf("%w: local server identity: %w", ErrInvalid, err)
	}
	switch startup := s.Startup.(type) {
	case ManagedBackend:
		if startup.Runtime != node || !ValidResourcePath(startup.Entry) || !strings.HasSuffix(startup.Entry, ".mjs") {
			return fmt.Errorf("%w: managed backend requires one node ESM entry", ErrInvalid)
		}
		return nil
	case DeclaredConnection:
		if err := (mcpserver.Server{Name: s.Name, Connection: startup.Connection}).ValidateConnection(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		switch connection := startup.Connection.(type) {
		case mcpserver.StdioConnection:
			if err := validatePackageEnvironment(connection.Env); err != nil {
				return err
			}
			if err := validatePackageCommand(connection.Command); err != nil {
				return err
			}
			_, err := parseWorkingDirectory(connection.Dir)
			return err
		case mcpserver.HTTPConnection:
			return validatePackageEndpoint(connection.URL)
		}
	}
	return fmt.Errorf("%w: server %q has no startup", ErrInvalid, s.Name)
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
		if upper == process.RootVariable || upper == process.DataVariable {
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
func validatePackageCommand(command string) error {
	if packaged, found := strings.CutPrefix(command, "./"); found {
		if !ValidResourcePath(packaged) {
			return fmt.Errorf("%w: package command path", ErrInvalid)
		}
		return nil
	}
	if strings.ContainsAny(command, "/\\ \t\r\n$") {
		return fmt.Errorf("%w: bare command name", ErrInvalid)
	}
	return nil
}

// validatePackageEndpoint refuses URL user information, which is structurally
// a credential, and plaintext transport beyond the local machine. Package
// bytes are public content: other static values, such as headers, are never
// treated as secrets, and a credential reaches a server only through an input.
func validatePackageEndpoint(raw string) error {
	endpoint, err := httporigin.ParseEndpoint(raw)
	if err != nil {
		return fmt.Errorf("%w: package server URL: %w", ErrInvalid, err)
	}
	if endpoint.HasFragment() {
		return fmt.Errorf("%w: package server URL carries a fragment", ErrInvalid)
	}
	if !endpoint.Secure() && !endpoint.Loopback() {
		return fmt.Errorf("%w: package server requires HTTPS outside loopback", ErrInvalid)
	}
	return nil
}
