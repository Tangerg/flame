package plugin

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/process"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"

	framevalue "github.com/Tangerg/frame/value"
	skillspec "github.com/Tangerg/scope/skills"
)

var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var themeColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func ThemeColorPattern() string { return themeColor.String() }

const maxNameBytes = 64

func validName(name string) bool {
	return len(name) <= maxNameBytes && namePattern.MatchString(name) && !strings.Contains(name, "--") && !strings.Contains(name, "..")
}

const (
	maxServers     = 64
	maxInputs      = 64
	maxThemes      = 32
	maxDiagnostics = 1024
)

type Server struct {
	Name    mcpserver.ServerName
	Startup ServerStartup
}

type ServerStartup interface {
	Transport() mcpserver.Transport
	clone() ServerStartup
}

type ManagedBackend struct {
	Runtime ManagedRuntime
	Entry   string
}

type DeclaredConnection struct{ mcpserver.Connection }

func (ManagedBackend) Transport() mcpserver.Transport { return mcpserver.TransportStdio }

func (m ManagedBackend) clone() ServerStartup { return m }

func (d DeclaredConnection) clone() ServerStartup {
	return DeclaredConnection{Connection: mcpserver.Server{Connection: d.Connection}.Clone().Connection}
}

func (s Server) Transport() mcpserver.Transport { return s.Startup.Transport() }

func (s Server) stdio() mcpserver.StdioConnection {
	declared, _ := s.Startup.(DeclaredConnection)
	connection, _ := declared.Connection.(mcpserver.StdioConnection)
	return connection
}

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

type Declaration struct {
	Name          string
	Version       string
	Description   string
	Servers       []Server
	Contributions []Contribution
	Points        []Point
	PointValues   []PointValue
	Components    []ClientComponent
	TerminalViews []TerminalView
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
		d.Components[i] = d.Components[i].clone()
	}
	d.Servers = slices.Clone(d.Servers)
	for index := range d.Servers {
		d.Servers[index] = d.Servers[index].clone()
	}
	d.Contributions = slices.Clone(d.Contributions)
	for index := range d.Contributions {
		d.Contributions[index] = d.Contributions[index].clone()
	}
	d.TerminalViews = slices.Clone(d.TerminalViews)
	for index := range d.TerminalViews {
		d.TerminalViews[index] = d.TerminalViews[index].clone()
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

type Release struct {
	digest      framevalue.Digest
	declaration Declaration
}

func admitEach[T any](values []T, admit func(T) error) error {
	for _, value := range values {
		if err := admit(value); err != nil {
			return err
		}
	}
	return nil
}

func NewRelease(digest framevalue.Digest, declaration Declaration) (Release, error) {
	builder, err := NewBuilder(declaration.Name, declaration.Version, declaration.Description)
	if err != nil {
		return Release{}, err
	}
	// Later entries resolve earlier ones: a terminal view names a contribution,
	// which names a server.
	for _, admit := range []func() error{
		func() error { return admitEach(declaration.Servers, builder.AdmitServer) },
		func() error { return admitEach(declaration.Contributions, builder.AdmitContribution) },
		func() error { return admitEach(declaration.Points, builder.AdmitPoint) },
		func() error { return admitEach(declaration.PointValues, builder.AdmitPointValue) },
		func() error { return admitEach(declaration.Components, builder.AdmitComponent) },
		func() error { return admitEach(declaration.TerminalViews, builder.AdmitTerminalView) },
		func() error { return admitEach(declaration.Inputs, builder.AdmitInput) },
		func() error { return admitEach(declaration.Themes, builder.AdmitTheme) },
		func() error { return admitEach(declaration.Locales, builder.AdmitLocale) },
		func() error { return admitEach(declaration.Skills, builder.AdmitSkill) },
	} {
		if err := admit(); err != nil {
			return Release{}, err
		}
	}
	for _, diagnostic := range declaration.Diagnostics {
		builder.Report(diagnostic)
	}
	return builder.Release(digest)
}

func (r Release) Digest() framevalue.Digest { return r.digest }
func (r Release) Name() string              { return r.declaration.Name }

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

func (r Release) recipient(name mcpserver.ServerName) (framevalue.Digest, bool) {
	server, found := r.server(name)
	if !found {
		return framevalue.Digest{}, false
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
	if theme.Scheme != ThemeDark && theme.Scheme != ThemeLight {
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

func (b *Builder) Release(digest framevalue.Digest) (Release, error) {
	if err := digest.Validate(); err != nil {
		return Release{}, fmt.Errorf("%w: release digest: %w", ErrInvalid, err)
	}
	declaration, err := b.Declaration()
	if err != nil {
		return Release{}, err
	}
	return Release{digest: digest, declaration: declaration}, nil
}

func (b *Builder) Declaration() (Declaration, error) {
	if len(b.declaration.Diagnostics) > maxDiagnostics {
		return Declaration{}, fmt.Errorf("%w: diagnostic capacity", ErrInvalid)
	}
	for index, diagnostic := range b.declaration.Diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return Declaration{}, fmt.Errorf("diagnostic %d: %w", index, err)
		}
	}
	for _, component := range b.declaration.Components {
		if component.TargetView != "" && !slices.ContainsFunc(b.declaration.Components, func(view ClientComponent) bool {
			return view.ID.String() == component.TargetView && view.Placement == extension.ComponentPlacementWorkspaceView && view.Scope == extension.ComponentScopeWorkspace
		}) {
			return Declaration{}, refuse(DiagnosticInvalidDependencies, "workspace action %q targets an unavailable workspace view %q", component.ID, component.TargetView)
		}
	}
	return b.declaration.clone(), nil
}

func (b *Builder) release() Release { return Release{declaration: b.declaration} }

var managedNodeLaunchVariables = []string{"NODE_OPTIONS", "NODE_PATH", "NODE_REPL_EXTERNAL_MODULE"}

type inputSlot struct {
	server mcpserver.ServerName
	target InputTarget
	key    string
}

func (i Input) binding(server Server) (inputSlot, error) {
	var key string
	var err error
	switch i.Target {
	case Environment:
		key, err = i.environmentKey(server)
	case Header:
		key, err = i.headerKey(server)
	case Authorization:
		err = i.validateAuthorization(server)
	default:
		err = refuse(DiagnosticInvalidDeclaration, "input %q target", i.ID)
	}
	if err != nil {
		return inputSlot{}, err
	}
	return inputSlot{server: i.Server, target: i.Target, key: key}, nil
}

func (i Input) environmentKey(server Server) (string, error) {
	if server.Transport() != mcpserver.TransportStdio {
		return "", refuse(DiagnosticInvalidDependencies, "input %q binds an environment variable of a non-stdio server", i.ID)
	}
	_, managed := server.Startup.(ManagedBackend)
	if managed && slices.Contains(managedNodeLaunchVariables, strings.ToUpper(i.Key)) {
		return "", refuse(DiagnosticInvalidDependencies, "input %q replaces managed node launch policy", i.ID)
	}
	if err := validatePackageEnvironment(map[string]string{i.Key: ""}); err != nil {
		return "", &Refusal{Code: DiagnosticInvalidDeclaration, cause: fmt.Errorf("input %q: %w", i.ID, err)}
	}
	key := strings.ToUpper(i.Key)
	stdio := server.stdio()
	for static := range stdio.Env {
		if strings.EqualFold(static, key) {
			return "", refuse(DiagnosticInvalidDependencies, "input %q duplicates static environment configuration", i.ID)
		}
	}
	return key, nil
}

func (i Input) headerKey(server Server) (string, error) {
	if server.Transport() != mcpserver.TransportStreamableHTTP {
		return "", refuse(DiagnosticInvalidDependencies, "input %q binds a header of a non-HTTP server", i.ID)
	}
	if err := mcpserver.ValidateHTTPHeaders("", map[string]string{i.Key: ""}); err != nil {
		return "", refuse(DiagnosticInvalidDeclaration, "input %q header: %v", i.ID, err)
	}
	if !i.Secret {
		return "", refuse(DiagnosticInvalidDeclaration, "input %q header must be a secret", i.ID)
	}
	key := strings.ToLower(i.Key)
	http, _ := server.http()
	for static := range http.Headers {
		if strings.EqualFold(static, key) {
			return "", refuse(DiagnosticInvalidDependencies, "input %q duplicates static header configuration", i.ID)
		}
	}
	return key, nil
}

func (i Input) validateAuthorization(server Server) error {
	if server.Transport() != mcpserver.TransportStreamableHTTP {
		return refuse(DiagnosticInvalidDependencies, "input %q binds authorization of a non-HTTP server", i.ID)
	}
	if i.Key != "" || !i.Secret {
		return refuse(DiagnosticInvalidDeclaration, "input %q authorization must be an unkeyed secret", i.ID)
	}
	return nil
}

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

// URL user information is structurally a credential, and package bytes are public,
// so a credential reaches a server only through an input.
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
