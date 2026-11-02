package persistence

import (
	"context"
	"crypto/rand"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	app "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/qimu/sqlite"
)

// The connector can nominate only a transport the user authorized for this exact installation and server.
// Secrets and executable launch material never appear in connector requests or command receipts.
type launchMaterial struct {
	HTTP    *httpGrant                 `json:"http,omitzero"`
	Process *mcpserver.StdioConnection `json:"process,omitzero"`
}

// HTTP field values are octets. JSON strings cannot preserve every admitted credential.
type httpGrant struct {
	URL           string
	Authorization []byte
	Headers       map[string][]byte
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

func (m *MCPServerStore) PrepareGrant(ctx context.Context, owner identity.InstallationID, srv mcpserver.Server, proposal app.GrantProposal) (string, error) {
	if proposal.Source.InstallationID != owner.String() || proposal.Change.Name != srv.Name.String() || proposal.Change.Replacement == nil || proposal.Change.Replacement.LaunchGrant != "" {
		return "", errors.New("mcp: grant proposal does not match its original source")
	}
	if err := proposal.Source.Validate(); err != nil {
		return "", err
	}
	if err := proposal.Invocation.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(proposal, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	return m.storeGrant(ctx, owner, srv, data)
}

func (m *MCPServerStore) storeGrant(ctx context.Context, owner identity.InstallationID, srv mcpserver.Server, proposal []byte) (string, error) {
	if err := owner.Validate(); err != nil {
		return "", err
	}
	if err := srv.Validate(); err != nil {
		return "", err
	}
	if srv.Source.Origin().Kind() != mcpserver.OriginUser {
		return "", errors.New("mcp: external grant requires a user source")
	}
	var material launchMaterial
	switch c := srv.Connection.(type) {
	case mcpserver.HTTPConnection:
		material.HTTP = encodeHTTPGrant(c)
	case mcpserver.StdioConnection:
		material.Process = &c
	default:
		return "", errors.New("mcp: unsupported launch material")
	}
	body, err := json.Marshal(material, json.Deterministic(true), json.FormatNilMapAsNull(true), json.FormatNilSliceAsNull(true))
	if err != nil {
		return "", err
	}
	var grant string
	err = sqlite.RunInTx(ctx, m.db, func(ctx context.Context) error {
		db := sqlite.Connection(ctx, m.db)
		if _, err := db.ExecContext(ctx, `INSERT INTO mcp_sources(origin,name) VALUES(?,?) ON CONFLICT DO NOTHING`, mcpserver.OriginUser, srv.Name.String()); err != nil {
			return err
		}
		base := []any{owner.String(), string(mcpserver.OriginUser), nil, srv.Name.String()}
		err := db.QueryRowContext(ctx, `SELECT grant_id FROM mcp_launch_grants WHERE connector=? AND source_id=`+mcpSourceIDQuery+` AND material=? AND proposal IS NULL LIMIT 1`, append(base, body)...).Scan(&grant)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return db.QueryRowContext(ctx, `INSERT INTO mcp_launch_grants(grant_id,connector,source_id,material,proposal) VALUES(?,?,`+mcpSourceIDQuery+`,?,?) ON CONFLICT(connector,source_id,proposal) DO UPDATE SET proposal=excluded.proposal WHERE material=excluded.material RETURNING grant_id`, rand.Text(), owner.String(), string(mcpserver.OriginUser), nil, srv.Name.String(), body, proposal).Scan(&grant)
	})
	return grant, err
}
func (m *MCPServerStore) OpenGrant(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, grant string) (mcpserver.Connection, bool, error) {
	var body []byte
	err := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT material FROM mcp_launch_grants WHERE grant_id=? AND connector=? AND proposal IS NULL AND source_id=`+mcpSourceIDQuery, grant, owner.String(), string(mcpserver.OriginUser), nil, name.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var material launchMaterial
	if err := json.Unmarshal(body, &material, json.RejectUnknownMembers(true)); err != nil {
		return nil, false, err
	}
	switch {
	case material.HTTP != nil && material.Process == nil:
		connection := material.HTTP.connection()
		if err := (mcpserver.Server{Source: mcpserver.UserSource(), Name: name, Connection: connection}).Validate(); err != nil {
			return nil, false, err
		}
		return connection, true, nil
	case material.Process != nil && material.HTTP == nil:
		if err := (mcpserver.Server{Source: mcpserver.UserSource(), Name: name, Connection: *material.Process}).Validate(); err != nil {
			return nil, false, err
		}
		return *material.Process, true, nil
	default:
		return nil, false, errors.New("mcp: invalid stored launch grant")
	}
}
func (m *MCPServerStore) PendingGrant(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, grant string) (app.GrantProposal, bool, error) {
	var raw []byte
	err := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT proposal FROM mcp_launch_grants WHERE grant_id=? AND connector=? AND source_id=`+mcpSourceIDQuery+` AND proposal IS NOT NULL`, grant, owner.String(), string(mcpserver.OriginUser), nil, name.String()).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return app.GrantProposal{}, false, nil
	}
	if err != nil {
		return app.GrantProposal{}, false, err
	}
	var proposal app.GrantProposal
	if err := json.Unmarshal(raw, &proposal, json.RejectUnknownMembers(true)); err != nil {
		return app.GrantProposal{}, false, err
	}
	return proposal, true, nil
}

func (m *MCPServerStore) RejectGrant(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, grant string) error {
	_, err := sqlite.Connection(ctx, m.db).ExecContext(ctx, `DELETE FROM mcp_launch_grants WHERE grant_id=? AND connector=? AND source_id=`+mcpSourceIDQuery+` AND proposal IS NOT NULL`, grant, owner.String(), string(mcpserver.OriginUser), nil, name.String())
	return err
}

// A receipt read can race the next command's preparation. The original
// insert order fences retirement so this receipt cannot revoke a newer grant.
func (m *MCPServerStore) ConfirmPendingGrant(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, grant string) error {
	return sqlite.RunInTx(ctx, m.db, func(ctx context.Context) error {
		db := sqlite.Connection(ctx, m.db)
		var position, sourceID int64
		err := db.QueryRowContext(ctx, `SELECT sequence,source_id FROM mcp_launch_grants WHERE grant_id=? AND connector=? AND source_id=`+mcpSourceIDQuery, grant, owner.String(), string(mcpserver.OriginUser), nil, name.String()).Scan(&position, &sourceID)
		if err != nil {
			return err
		}
		var newer bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mcp_launch_grants WHERE connector=? AND source_id=? AND sequence>? AND proposal IS NULL)`, owner.String(), sourceID, position).Scan(&newer); err != nil {
			return err
		}
		if newer {
			return errors.New("mcp: newer transport grant already active")
		}
		if _, err := db.ExecContext(ctx, `UPDATE mcp_launch_grants SET proposal=NULL WHERE sequence=?`, position); err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `DELETE FROM mcp_launch_grants WHERE connector=? AND source_id=? AND sequence<?`, owner.String(), sourceID, position)
		return err
	})
}

func (m *MCPServerStore) RetireGrants(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, active string) error {
	return sqlite.RunInTx(ctx, m.db, func(ctx context.Context) error {
		db := sqlite.Connection(ctx, m.db)
		if active != "" {
			r, err := db.ExecContext(ctx, `UPDATE mcp_launch_grants SET proposal=NULL WHERE grant_id=? AND connector=? AND source_id=`+mcpSourceIDQuery, active, owner.String(), string(mcpserver.OriginUser), nil, name.String())
			if err != nil {
				return err
			}
			affected, err := r.RowsAffected()
			if err != nil {
				return err
			}
			if affected != 1 {
				return errors.New("mcp: original grant was revoked")
			}
		}
		_, err := db.ExecContext(ctx, `DELETE FROM mcp_launch_grants WHERE connector=? AND source_id=`+mcpSourceIDQuery+` AND grant_id<>?`, owner.String(), string(mcpserver.OriginUser), nil, name.String(), active)
		return err
	})
}
