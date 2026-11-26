package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

type launchMaterial struct {
	HTTP    *httpGrant  `json:"http,omitzero"`
	Process *stdioGrant `json:"process,omitzero"`
}

// HTTP field values are octets. JSON strings cannot preserve every admitted credential.
type httpGrant struct {
	URL           string            `json:"url"`
	Authorization []byte            `json:"authorization"`
	Headers       map[string][]byte `json:"headers"`
}

// A nil Env inherits the parent environment and an empty one supplies none, so the
// encoding keeps null distinct from {}.
type stdioGrant struct {
	Command              string            `json:"command"`
	ExecutionEnvironment string            `json:"executionEnvironment"`
	Args                 []string          `json:"args"`
	Env                  map[string]string `json:"env"`
	Dir                  string            `json:"dir"`
}

func encodeStdioGrant(connection mcpserver.StdioConnection) *stdioGrant {
	return &stdioGrant{Command: connection.Command, ExecutionEnvironment: connection.ExecutionEnvironment, Args: connection.Args, Env: connection.Env, Dir: connection.Dir}
}

func (g stdioGrant) connection() mcpserver.StdioConnection {
	return mcpserver.StdioConnection{Command: g.Command, ExecutionEnvironment: g.ExecutionEnvironment, Args: g.Args, Env: g.Env, Dir: g.Dir}
}

func encodeHTTPGrant(connection mcpserver.HTTPConnection) *httpGrant {
	grant := &httpGrant{URL: connection.URL, Authorization: []byte(connection.Authorization)}
	if connection.Headers != nil {
		grant.Headers = make(map[string][]byte, len(connection.Headers))
		for name, value := range connection.Headers {
			grant.Headers[name] = []byte(value)
		}
	}
	return grant
}

func (g httpGrant) connection() mcpserver.HTTPConnection {
	connection := mcpserver.HTTPConnection{URL: g.URL, Authorization: string(g.Authorization)}
	if g.Headers != nil {
		connection.Headers = make(map[string]string, len(g.Headers))
		for name, value := range g.Headers {
			connection.Headers[name] = string(value)
		}
	}
	return connection
}

func encodeLaunchMaterial(server mcpserver.Server) ([]byte, error) {
	if err := server.Validate(); err != nil {
		return nil, err
	}
	if server.Source.Origin().Kind() != mcpserver.OriginUser {
		return nil, errors.New("mcp: launch grant requires a user source")
	}
	var material launchMaterial
	switch connection := server.Connection.(type) {
	case mcpserver.HTTPConnection:
		material.HTTP = encodeHTTPGrant(connection)
	case mcpserver.StdioConnection:
		material.Process = encodeStdioGrant(connection)
	default:
		return nil, errors.New("mcp: unsupported launch material")
	}
	return json.Marshal(material, json.Deterministic(true), json.FormatNilMapAsNull(true), json.FormatNilSliceAsNull(true))
}

func (m *MCPServerStore) OpenGrant(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, token string) (mcpapp.LaunchGrant, bool, error) {
	return scopedPair(ctx, m.db.read, func(ctx context.Context) (mcpapp.LaunchGrant, bool, error) {
		var body []byte
		var intent sql.NullString
		err := m.db.connection(ctx).QueryRowContext(ctx, `SELECT grant.material, pending.intent
			FROM mcp_launch_grants grant
			LEFT JOIN pending_mcp_directory_changes pending ON pending.invocation = grant.proposal
			WHERE grant.grant_id=? AND grant.connector=? AND grant.source_id=`+mcpSourceIDQuery,
			token, owner.String(), string(mcpserver.OriginUser), nil, name.String()).Scan(&body, &intent)
		if errors.Is(err, sql.ErrNoRows) {
			return mcpapp.LaunchGrant{}, false, nil
		}
		if err != nil {
			return mcpapp.LaunchGrant{}, false, err
		}
		var proposal *mcpapp.DirectoryIntent
		if intent.Valid {
			value, err := decodeDirectoryIntent(intent.String)
			if err != nil {
				return mcpapp.LaunchGrant{}, false, err
			}
			proposal = &value
		}
		connection, err := decodeLaunchMaterial(body, name)
		if err != nil {
			return mcpapp.LaunchGrant{}, false, err
		}
		return mcpapp.LaunchGrant{Connection: connection, Proposal: proposal}, true, nil
	})
}

func decodeLaunchMaterial(body []byte, name mcpserver.ServerName) (mcpserver.Connection, error) {
	var material launchMaterial
	if err := json.Unmarshal(body, &material, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	var connection mcpserver.Connection
	switch {
	case material.HTTP != nil && material.Process == nil:
		connection = material.HTTP.connection()
	case material.Process != nil && material.HTTP == nil:
		connection = material.Process.connection()
	default:
		return nil, errors.New("mcp: invalid stored launch grant")
	}
	if err := (mcpserver.Server{Source: mcpserver.UserSource(), Name: name, Connection: connection}).Validate(); err != nil {
		return nil, err
	}
	return connection, nil
}

func (m *MCPServerStore) settleLaunchGrants(ctx context.Context, intent mcpapp.DirectoryIntent, accepted bool) error {
	db := m.db.connection(ctx)
	if !accepted {
		_, err := db.ExecContext(ctx, `DELETE FROM mcp_launch_grants WHERE proposal=?`, intent.Invocation.ID)
		return err
	}
	source := []any{intent.Source.InstallationID, string(mcpserver.OriginUser), nil, intent.Change.Name}
	if intent.Change.Replacement == nil {
		_, err := db.ExecContext(ctx, `DELETE FROM mcp_launch_grants WHERE connector=? AND source_id=`+mcpSourceIDQuery, source...)
		return err
	}
	active := intent.Change.Replacement.LaunchGrant
	var proposal sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT proposal FROM mcp_launch_grants
		WHERE connector=? AND source_id=`+mcpSourceIDQuery+` AND grant_id=?`, append(source, active)...).Scan(&proposal); err != nil {
		return err
	}
	if !proposal.Valid {
		return nil
	}
	if proposal.String != intent.Invocation.ID {
		return errors.New("mcp: launch grant belongs to another directory change")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM mcp_launch_grants WHERE connector=? AND source_id=`+mcpSourceIDQuery+` AND proposal IS NULL`, source...); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `UPDATE mcp_launch_grants SET proposal=NULL WHERE grant_id=? AND proposal=?`, active, intent.Invocation.ID)
	return err
}
