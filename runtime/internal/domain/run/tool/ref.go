package tool

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

type RefKind string

const (
	BuiltInKind           RefKind = "builtIn"
	MCPKind               RefKind = "mcp"
	A2AKind               RefKind = "a2a"
	MaximumModelNameBytes         = 64
)

var ErrInvalidRef = errors.New("tool: invalid reference")
var modelNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Ref identifies a source, never a lossy model-facing label. The zero value is invalid.
type Ref struct {
	kind   RefKind
	name   string
	server mcpserver.ServerName
	remote mcpserver.RemoteToolName
}

func BuiltIn(name string) (Ref, error) {
	if !slices.Contains(BuiltInNames(), name) {
		return Ref{}, fmt.Errorf("%w: unknown built-in %q", ErrInvalidRef, name)
	}
	return Ref{kind: BuiltInKind, name: name}, nil
}
func BuiltInNames() []string {
	return []string{ApplyPatch, AskUser, CreateGoal, CreateSchedule, DeleteSchedule, DelegateTask, Edit,
		EnterPlanMode, ExitPlanMode, GetGoal, Glob, Grep, HTTPRequest, ListSchedules, ListSkills,
		LoadSkill, LSP, ProposeSkill, Read, ReadShellOutput, ReadSkillResource, ReadToolResult,
		ReportGoalOutcome, SearchMemory, SearchTools, SetPlan, Shell, StopShell, WebFetch, WebSearch}
}

func MCP(server mcpserver.ServerName, remote mcpserver.RemoteToolName) (Ref, error) {
	if err := server.Validate(); err != nil {
		return Ref{}, fmt.Errorf("%w: %w", ErrInvalidRef, err)
	}
	if err := remote.Validate(); err != nil {
		return Ref{}, fmt.Errorf("%w: %w", ErrInvalidRef, err)
	}
	return Ref{kind: MCPKind, server: server, remote: remote}, nil
}

func A2A(endpoint string) (Ref, error) {
	if !modelNamePattern.MatchString(endpoint) {
		return Ref{}, fmt.Errorf("%w: invalid A2A endpoint name", ErrInvalidRef)
	}
	return Ref{kind: A2AKind, name: endpoint}, nil
}

func (r Ref) Kind() RefKind                    { return r.kind }
func (r Ref) Name() string                     { return r.name }
func (r Ref) Server() mcpserver.ServerName     { return r.server }
func (r Ref) Remote() mcpserver.RemoteToolName { return r.remote }
func (r Ref) Validate() error {
	if r.kind == "" {
		return ErrInvalidRef
	}
	return nil
}
func (r Ref) IsBuiltIn(name string) bool { return r.kind == BuiltInKind && r.name == name }

func (r Ref) ModelName() string {
	if r.kind != MCPKind {
		return r.name
	}
	raw := r.server.String() + "_" + r.remote.String()
	result := make([]byte, 0, min(len(raw), MaximumModelNameBytes))
	for i := 0; i < len(raw) && i < MaximumModelNameBytes; i++ {
		c := raw[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			c = '_'
		}
		result = append(result, c)
	}
	return string(result)
}

func (r Ref) String() string {
	switch r.kind {
	case BuiltInKind, A2AKind:
		return string(r.kind) + ":" + url.PathEscape(r.name)
	case MCPKind:
		return "mcp:" + url.PathEscape(r.server.String()) + ":" + url.PathEscape(r.remote.String())
	default:
		return ""
	}
}

func ParseRef(text string) (Ref, error) {
	parts := strings.Split(text, ":")
	var r Ref
	var err error
	if len(parts) < 2 {
		return r, ErrInvalidRef
	}
	name, err := url.PathUnescape(parts[1])
	if err != nil {
		return r, ErrInvalidRef
	}
	switch RefKind(parts[0]) {
	case BuiltInKind:
		if len(parts) != 2 {
			return r, ErrInvalidRef
		}
		r, err = BuiltIn(name)
	case A2AKind:
		if len(parts) != 2 {
			return r, ErrInvalidRef
		}
		r, err = A2A(name)
	case MCPKind:
		if len(parts) != 3 {
			return r, ErrInvalidRef
		}
		server, e := mcpserver.ParseServerName(name)
		if e != nil {
			return r, e
		}
		remoteText, e := url.PathUnescape(parts[2])
		if e != nil {
			return r, e
		}
		remote, e := mcpserver.ParseRemoteToolName(remoteText)
		if e != nil {
			return r, e
		}
		r, err = MCP(server, remote)
	default:
		return r, ErrInvalidRef
	}
	if err != nil {
		return Ref{}, err
	}
	if r.String() != text {
		return Ref{}, fmt.Errorf("%w: noncanonical spelling", ErrInvalidRef)
	}
	return r, nil
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
		return err
	}
	*r = parsed
	return nil
}

func (r Ref) ValidateFingerprint(fingerprint string) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.kind == BuiltInKind {
		if fingerprint != "" {
			return errors.New("tool: built-in has no source fingerprint")
		}
		return nil
	}
	if len(fingerprint) != 64 || strings.Trim(fingerprint, "0123456789abcdef") != "" {
		return errors.New("tool: source fingerprint must be a SHA-256 digest")
	}
	return nil
}
