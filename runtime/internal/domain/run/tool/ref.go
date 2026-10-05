package tool

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type RefKind string

const (
	BuiltInKind           RefKind = "builtIn"
	MCPKind               RefKind = "mcp"
	A2AKind               RefKind = "a2a"
	MaximumModelNameBytes         = 64
)

// RefKinds is the closed variant set. Every boundary that encodes a reference
// covers exactly these kinds.
func RefKinds() []RefKind { return []RefKind{BuiltInKind, MCPKind, A2AKind} }

var ErrInvalidRef = errors.New("tool: invalid reference")
var modelNameExpression = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9_-]{1,%d}$`, MaximumModelNameBytes))

func ModelNamePattern() string { return modelNameExpression.String() }

// Ref identifies a source, never a lossy model-facing label. It is a closed
// union: each variant's data is reachable only through the accessor for that
// variant, so no caller can read a built-in name from an MCP reference. The
// zero value is invalid; values are comparable map keys.
type Ref struct {
	kind   RefKind
	name   string
	server mcpserver.ID
	remote mcpserver.RemoteToolName
}

func BuiltIn(name BuiltInName) (Ref, error) {
	if !slices.Contains(builtInNames, name) {
		return Ref{}, fmt.Errorf("%w: unknown built-in %q", ErrInvalidRef, name)
	}
	return Ref{kind: BuiltInKind, name: string(name)}, nil
}

func MCP(server mcpserver.ID, remote mcpserver.RemoteToolName) (Ref, error) {
	if err := server.Validate(); err != nil {
		return Ref{}, fmt.Errorf("%w: %w", ErrInvalidRef, err)
	}
	if err := remote.Validate(); err != nil {
		return Ref{}, fmt.Errorf("%w: %w", ErrInvalidRef, err)
	}
	return Ref{kind: MCPKind, server: server, remote: remote}, nil
}

func A2A(endpoint string) (Ref, error) {
	if !modelNameExpression.MatchString(endpoint) {
		return Ref{}, fmt.Errorf("%w: invalid A2A endpoint name", ErrInvalidRef)
	}
	return Ref{kind: A2AKind, name: endpoint}, nil
}

func (r Ref) Kind() RefKind { return r.kind }

func (r Ref) BuiltIn() (name BuiltInName, ok bool) {
	if r.kind != BuiltInKind {
		return "", false
	}
	return BuiltInName(r.name), true
}

func (r Ref) MCP() (server mcpserver.ID, remote mcpserver.RemoteToolName, ok bool) {
	return r.server, r.remote, r.kind == MCPKind
}

func (r Ref) A2A() (endpoint string, ok bool) {
	return r.name, r.kind == A2AKind
}

func (r Ref) Validate() error {
	if r.kind == "" {
		return fmt.Errorf("%w: reference is unset", ErrInvalidRef)
	}
	return nil
}
func (r Ref) IsBuiltIn(name BuiltInName) bool { return r.kind == BuiltInKind && r.name == string(name) }

func (r Ref) ModelName() string {
	if r.kind != MCPKind {
		return r.name
	}
	raw := r.server.Name().String() + "_" + r.remote.String()
	result := make([]byte, 0, min(len(raw), MaximumModelNameBytes))
	for i := 0; i < len(raw) && i < MaximumModelNameBytes; i++ {
		c := raw[i]
		if !modelNameByte(c) {
			c = '_'
		}
		result = append(result, c)
	}
	return string(result)
}

func modelNameByte(c byte) bool {
	letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	digit := c >= '0' && c <= '9'
	return letter || digit || c == '_' || c == '-'
}

// String is the reference's only text codec, used where a reference is one
// value inside an encoded payload or a derived key. No component grammar
// admits ':', so the fields need no escaping and the spelling is unique.
func (r Ref) String() string {
	switch r.kind {
	case BuiltInKind, A2AKind:
		return string(r.kind) + ":" + r.name
	case MCPKind:
		fields := []string{string(MCPKind), string(r.server.Origin().Kind())}
		if installation, ok := r.server.Origin().Installation(); ok {
			fields = append(fields, installation.String())
		}
		return strings.Join(append(fields, r.server.Name().String(), r.remote.String()), ":")
	default:
		return ""
	}
}

func ParseRef(text string) (Ref, error) {
	parts := strings.Split(text, ":")
	var r Ref
	var err error
	switch {
	case len(parts) == 2 && RefKind(parts[0]) == BuiltInKind:
		r, err = BuiltIn(BuiltInName(parts[1]))
	case len(parts) == 2 && RefKind(parts[0]) == A2AKind:
		r, err = A2A(parts[1])
	case len(parts) >= 4 && RefKind(parts[0]) == MCPKind:
		r, err = parseMCPRef(parts[1:])
	default:
		return Ref{}, fmt.Errorf("%w: unknown reference kind", ErrInvalidRef)
	}
	if err != nil {
		return Ref{}, fmt.Errorf("tool: parse reference: %w", err)
	}
	if r.String() != text {
		return Ref{}, fmt.Errorf("%w: noncanonical spelling", ErrInvalidRef)
	}
	return r, nil
}

func parseMCPRef(parts []string) (Ref, error) {
	var origin mcpserver.Origin
	switch {
	case len(parts) == 3 && mcpserver.OriginKind(parts[0]) == mcpserver.OriginUser:
		origin = mcpserver.UserOrigin()
	case len(parts) == 4 && mcpserver.OriginKind(parts[0]) == mcpserver.OriginInstallation:
		installation, err := resourceid.ParseInstallation(parts[1])
		if err != nil {
			return Ref{}, fmt.Errorf("%w: server origin: %w", ErrInvalidRef, err)
		}
		if origin, err = mcpserver.InstallationOrigin(installation); err != nil {
			return Ref{}, fmt.Errorf("%w: %w", ErrInvalidRef, err)
		}
		parts = parts[1:]
	default:
		return Ref{}, fmt.Errorf("%w: server origin", ErrInvalidRef)
	}
	name, err := mcpserver.ParseServerName(parts[1])
	if err != nil {
		return Ref{}, fmt.Errorf("%w: server: %w", ErrInvalidRef, err)
	}
	server, err := mcpserver.NewID(origin, name)
	if err != nil {
		return Ref{}, fmt.Errorf("%w: server: %w", ErrInvalidRef, err)
	}
	remote, err := mcpserver.ParseRemoteToolName(parts[2])
	if err != nil {
		return Ref{}, fmt.Errorf("%w: tool name: %w", ErrInvalidRef, err)
	}
	return MCP(server, remote)
}

func (r Ref) MarshalText() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return []byte(r.String()), nil
}
func (r *Ref) UnmarshalText(text []byte) error {
	parsed, err := ParseRef(string(text))
	if err != nil {
		return fmt.Errorf("tool: decode reference: %w", err)
	}
	*r = parsed
	return nil
}

// ValidateFingerprint states which references carry source authority: a
// built-in's authority is the Runtime binary, every other source has one.
func (r Ref) ValidateFingerprint(source fingerprint.Digest) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.kind == BuiltInKind {
		if !source.IsZero() {
			return errors.New("tool: built-in has no source fingerprint")
		}
		return nil
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("tool: source fingerprint: %w", err)
	}
	return nil
}
