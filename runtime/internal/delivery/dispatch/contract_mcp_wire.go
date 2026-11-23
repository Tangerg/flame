package dispatch

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func registerMCPUnions(s *Shapes) {
	s.union(UnionSpec{GoType: typeOf[protocol.MCPOrigin](), Variants: []VariantSpec{
		{Tag: string(protocol.MCPOriginUser)},
		{Tag: string(protocol.MCPOriginInstallation), Required: []string{"installationId"}},
	}})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPHandshakeTimeout](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPHandshakeUnbounded)},
			{Tag: string(protocol.MCPHandshakeBounded), Required: []string{"seconds"}},
		},
	})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPConnection](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPTransportStreamableHTTP), Required: []string{"url"}, Optional: []string{"authorizationMasked", "headersMasked"}},
			{Tag: string(protocol.MCPTransportStdio), Required: []string{"command"}, Optional: []string{"args", "envMasked", "dir"}},
		},
	})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPConnectionInput](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPTransportStreamableHTTP), Required: []string{"url"}, Optional: []string{"authorization", "headers"}},
			{Tag: string(protocol.MCPTransportStdio), Required: []string{"command"}, Optional: []string{"args", "env", "dir"}},
		},
	})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPAuthorizationChange](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPHeadersChange](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPEnvironmentChange](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPSecretSet), Required: []string{"value"}},
			{Tag: string(protocol.MCPSecretClear)},
		},
	})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPServerState](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPServerDisabled)},
			{Tag: string(protocol.MCPServerDisconnected)},
			{Tag: string(protocol.MCPServerConnecting)},
			{Tag: string(protocol.MCPServerConnected), Required: []string{"toolCount"}},
			{Tag: string(protocol.MCPServerFailed), Required: []string{"error"}, AllowedValues: allowedMCPStatusProblems(protocol.MCPStatusConfigurationFailed, protocol.MCPStatusDialFailed, protocol.MCPStatusToolDiscoveryFailed, protocol.MCPStatusAuthorizationFailed)},
			{Tag: string(protocol.MCPServerNeedsAuth), Required: []string{"error"}, AllowedValues: allowedMCPStatusProblems(protocol.MCPStatusAuthorizationRequired)},
		},
	})
	s.union(UnionSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttemptStatus](),
		Variants: []VariantSpec{
			{Tag: string(protocol.MCPAuthorizationAttemptPending)},
			{Tag: string(protocol.MCPAuthorizationAttemptSucceeded)},
			{Tag: string(protocol.MCPAuthorizationAttemptFailed), Required: []string{"error"}, AllowedValues: allowedMCPStatusProblems(protocol.MCPStatusAuthorizationFailed)},
			{Tag: string(protocol.MCPAuthorizationAttemptCanceled)},
			{Tag: string(protocol.MCPAuthorizationAttemptUnconfirmed)},
		},
	})
}

func registerMCPConditions(s *Shapes) {
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttempt](),
		Rules: []ConditionalRule{{
			Required: []string{"createdAt"},
		}, {
			When:      whenEquals("status.type", string(protocol.MCPAuthorizationAttemptPending)),
			Forbidden: []string{"finishedAt"},
		}, {
			When:     whenEquals("status.type", string(protocol.MCPAuthorizationAttemptSucceeded)),
			Required: []string{"finishedAt"},
		}, {
			When:     whenEquals("status.type", string(protocol.MCPAuthorizationAttemptFailed)),
			Required: []string{"finishedAt"},
		}, {
			When:     whenEquals("status.type", string(protocol.MCPAuthorizationAttemptUnconfirmed)),
			Required: []string{"finishedAt"},
		}, {
			When:     whenEquals("status.type", string(protocol.MCPAuthorizationAttemptCanceled)),
			Required: []string{"finishedAt"},
		}},
	})
}

func registerMCPValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.MCPOrigin](),
		Constraints: []FieldConstraint{{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()}},
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.MCPServerID](), Constraints: mcpServerName("name")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.MCPToolExposure](), Constraints: mcpRemoteToolItems("disabledTools")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.SetMCPToolExposureRequest](), Constraints: mcpRemoteToolIdentity()})

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPServerState](),
		Constraints: []FieldConstraint{
			{Field: "toolCount", Kind: ConstraintNonNegative},
			{Field: "toolCount", Kind: ConstraintMaximum, Limit: mcpserver.MaxRemoteToolsPerServer},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.MCPTool](),
		Constraints: append(mcpRemoteToolIdentity(), FieldConstraint{Field: "modelName", Kind: ConstraintPattern, Value: tool.ModelNamePattern()}),
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttemptRequest](),
		Constraints: []FieldConstraint{{
			Field: "attemptId", Kind: ConstraintPattern, Value: mcpapp.AuthorizationAttemptIDPattern(),
		}},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPAuthorizationAttempt](),
		Constraints: []FieldConstraint{
			{Field: "id", Kind: ConstraintPattern, Value: mcpapp.AuthorizationAttemptIDPattern()},
		},
	})
	nonEmpty[protocol.MCPConnection](s, "url", "command")
	nonEmpty[protocol.MCPConnectionInput](s, "url", "command")
	nonEmpty[protocol.MCPAuthorizationChange](s, "value")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPHandshakeTimeout](),
		Constraints: []FieldConstraint{
			{Field: "seconds", Kind: ConstraintPositive},
			{Field: "seconds", Kind: ConstraintMaximum, Limit: protocol.MaximumDurationSeconds},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPHeadersChange](),
		Constraints: []FieldConstraint{
			{Field: "value", Kind: ConstraintNonEmptyProperties},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPEnvironmentChange](),
		Constraints: []FieldConstraint{
			{Field: "value", Kind: ConstraintNonEmptyProperties},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.MCPServerCandidate](),
		Constraints: mcpServerName("name"),
	})
}

func mcpServerName(field string) []FieldConstraint {
	return []FieldConstraint{{Field: field, Kind: ConstraintPattern, Value: mcpserver.ServerNamePattern()}}
}

func mcpRemoteToolIdentity() []FieldConstraint {
	return []FieldConstraint{{Field: "name", Kind: ConstraintPattern, Value: mcpserver.RemoteToolNamePattern()}}
}

func mcpRemoteToolItems(field string) []FieldConstraint {
	return []FieldConstraint{
		{Field: field, Kind: ConstraintMaxItems, Limit: mcpserver.MaxRemoteToolsPerServer},
		{Field: field, Kind: ConstraintUniqueItems},
		{Field: field, Kind: ConstraintPatternItems, Value: mcpserver.RemoteToolNamePattern()},
	}
}
