package persistence

import (
	"context"
	"crypto/rand"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/qimu/sqlite"
)

// The connector can nominate only a transport the user authorized for this exact installation and server.
// Secrets and executable launch material never appear in connector requests or command receipts.
type launchMaterial struct {
	HTTP    *mcpserver.HTTPConnection  `json:"http,omitzero"`
	Process *mcpserver.StdioConnection `json:"process,omitzero"`
}

func (m *MCPServerStore) Grant(ctx context.Context, owner identity.InstallationID, srv mcpserver.Server) (string, error) {
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
		material.HTTP = &c
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
		return db.QueryRowContext(ctx, `INSERT INTO mcp_launch_grants(grant_id,connector,source_id,material) VALUES(?,?,`+mcpSourceIDQuery+`,?) ON CONFLICT(connector,source_id,material) DO UPDATE SET material=excluded.material RETURNING grant_id`, rand.Text(), owner.String(), string(mcpserver.OriginUser), nil, srv.Name.String(), body).Scan(&grant)
	})
	return grant, err
}
func (m *MCPServerStore) OpenGrant(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, grant string) (mcpserver.Connection, bool, error) {
	var body []byte
	err := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT material FROM mcp_launch_grants WHERE grant_id=? AND connector=? AND source_id=`+mcpSourceIDQuery, grant, owner.String(), string(mcpserver.OriginUser), nil, name.String()).Scan(&body)
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
		if err := (mcpserver.Server{Source: mcpserver.UserSource(), Name: name, Connection: *material.HTTP}).Validate(); err != nil {
			return nil, false, err
		}
		return *material.HTTP, true, nil
	case material.Process != nil && material.HTTP == nil:
		if err := (mcpserver.Server{Source: mcpserver.UserSource(), Name: name, Connection: *material.Process}).Validate(); err != nil {
			return nil, false, err
		}
		return *material.Process, true, nil
	default:
		return nil, false, errors.New("mcp: invalid stored launch grant")
	}
}
func (m *MCPServerStore) RetireGrants(ctx context.Context, owner identity.InstallationID, name mcpserver.ServerName, active string) error {
	_, err := sqlite.Connection(ctx, m.db).ExecContext(ctx, `DELETE FROM mcp_launch_grants WHERE connector=? AND source_id=`+mcpSourceIDQuery+` AND grant_id<>?`, owner.String(), string(mcpserver.OriginUser), nil, name.String(), active)
	return err
}
