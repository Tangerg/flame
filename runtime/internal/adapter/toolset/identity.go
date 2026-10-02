package toolset

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type toolIdentity interface{ ToolRef() tool.Ref }
type sourceAuthority interface{ SourceFingerprint() string }

// Identify resolves source-owned identity through Scope's capability chain.
// A model label is never a fallback identity for an unrecognized executable.
func Identify(executable toolcontract.Tool) (tool.Ref, error) {
	ref, found, err := mcp.IdentifyTool(executable)
	if err != nil || found {
		return ref, err
	}
	identity, found, err := toolcontract.Capability[toolIdentity](executable)
	if err != nil {
		return tool.Ref{}, err
	}
	if !found {
		return tool.Ref{}, fmt.Errorf("toolset: executable has no source identity")
	}
	ref = identity.ToolRef()
	return ref, ref.Validate()
}

func SourceFingerprint(executable toolcontract.Tool, ref tool.Ref) (string, error) {
	authority, found, err := toolcontract.Capability[sourceAuthority](executable)
	if err != nil {
		return "", err
	}
	fingerprint := ""
	if found {
		fingerprint = authority.SourceFingerprint()
	}
	return fingerprint, ref.ValidateFingerprint(fingerprint)
}

// WithIdentity binds a local or configured A2A source at construction. MCP
// identity and authority stay on Scope's connection-backed executable.
func WithIdentity(executable toolcontract.Tool, ref tool.Ref, fingerprint string) (toolcontract.Tool, error) {
	if err := ref.ValidateFingerprint(fingerprint); err != nil {
		return nil, err
	}
	if executable == nil || executable.Definition().Name != ref.ModelName() {
		return nil, fmt.Errorf("toolset: definition differs from source reference %s", ref)
	}
	return identifiedTool{Tool: executable, ref: ref, fingerprint: fingerprint}, nil
}

type identifiedTool struct {
	toolcontract.Tool
	ref         tool.Ref
	fingerprint string
}

func (t identifiedTool) Unwrap() toolcontract.Tool { return t.Tool }
func (t identifiedTool) ToolRef() tool.Ref         { return t.ref }
func (t identifiedTool) SourceFingerprint() string { return t.fingerprint }
