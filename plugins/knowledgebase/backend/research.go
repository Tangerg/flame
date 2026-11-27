package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	storage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/knowledge"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"
)

const researchSchema = `CREATE TABLE IF NOT EXISTS research_claims (project TEXT NOT NULL, id TEXT NOT NULL, revision INTEGER NOT NULL, claim TEXT NOT NULL CHECK(json_valid(claim)), PRIMARY KEY(project, id, revision))`

var errClaimInvalid = errors.New("knowledge base: a Project, statement, reason and valid material citations are required")
var errHumanClaim = errors.New("knowledge base: a person must accept the research claim")

type claimVersion struct {
	Statement  string                  `json:"statement"`
	Reason     string                  `json:"reason"`
	Citations  []wire.MaterialCitation `json:"citations"`
	AcceptedAt instant.Timestamp       `json:"acceptedAt"`
}

func checkClaimProject(project string) error {
	if _, err := identity.ParseOptionalProject(project); err != nil {
		return errClaimInvalid
	}
	if project == "" {
		return errClaimInvalid
	}
	return nil
}

func citationText(source materialSource, citation wire.MaterialCitation) (string, bool) {
	pages := sourcePages(source.Text)
	if citation.Page < 1 || citation.Page > len(pages) {
		return "", false
	}
	lines := strings.Split(pages[citation.Page-1], "\n")
	if citation.FirstLine < 1 || citation.LastLine < citation.FirstLine || citation.LastLine > len(lines) {
		return "", false
	}
	return strings.Join(lines[citation.FirstLine-1:citation.LastLine], "\n"), true
}

func (s *store) renderClaim(ctx context.Context, project, id string, revision int64, source claimVersion) (wire.ClaimRead, error) {
	view := wire.ClaimRead{
		ClaimSummary: wire.ClaimSummary{ID: id, Revision: revision, State: wire.ClaimCurrent, Statement: source.Statement},
		AcceptedAt:   source.AcceptedAt, Reason: source.Reason, Evidence: []wire.ClaimEvidence{},
	}
	for _, citation := range source.Citations {
		current, original, err := s.resolveSource(ctx, project, citation.DocumentID, citation.Revision)
		if errors.Is(err, knowledge.ErrNotFound) {
			// The statement and reason may quote revoked material.
			view.State, view.Statement, view.Reason, view.Evidence = wire.ClaimUnavailable, "", "", []wire.ClaimEvidence{}
			return view, nil
		}
		if err != nil {
			return wire.ClaimRead{}, err
		}
		before, valid := citationText(original, citation)
		if !valid {
			return wire.ClaimRead{}, errClaimInvalid
		}
		after, present := citationText(current, citation)
		recheck := !present || before != after
		if recheck {
			view.State = wire.ClaimRecheck
		}
		view.Evidence = append(view.Evidence, wire.ClaimEvidence{
			MaterialCitation: citation, Title: original.Header.Title, Source: original.Header.Source,
			Excerpt: before, NeedsRecheck: recheck,
		})
	}
	return view, nil
}

func (s *store) acceptClaim(ctx context.Context, input wire.ClaimAcceptInput) (storage.CommandPlan[wire.ClaimReceipt], error) {
	var empty storage.CommandPlan[wire.ClaimReceipt]
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return empty, err
	}
	if invocation.Origin != extension.Human {
		return empty, errHumanClaim
	}
	if err := checkClaimProject(input.Project); err != nil {
		return empty, err
	}
	if strings.TrimSpace(input.Statement) == "" || strings.TrimSpace(input.Reason) == "" || len(input.Citations) == 0 {
		return empty, errClaimInvalid
	}
	connection := s.db.Executor(ctx)
	var head int64
	if err := connection.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(revision),0) FROM research_claims WHERE project=? AND id=?`, input.Project, input.ID).Scan(&head); err != nil {
		return empty, err
	}
	if head != input.ExpectedRevision {
		return empty, knowledge.ErrRevisionConflict
	}
	var count int
	if err := connection.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM research_claims WHERE project=?`, input.Project).Scan(&count); err != nil {
		return empty, err
	}
	if count >= 128 {
		return empty, knowledge.ErrCapacity
	}
	newVersion := claimVersion{Statement: input.Statement, Reason: input.Reason, Citations: input.Citations, AcceptedAt: instant.Timestamp{Time: time.Now().UTC()}}
	view, err := s.renderClaim(ctx, input.Project, input.ID, head+1, newVersion)
	if err != nil {
		return empty, err
	}
	switch view.State {
	case wire.ClaimUnavailable:
		return empty, knowledge.ErrNotFound
	case wire.ClaimRecheck:
		return empty, knowledge.ErrRevisionConflict
	}
	encoded, err := json.Marshal(newVersion)
	if err != nil {
		return empty, err
	}
	return storage.NewCommandPlan(wire.ClaimReceipt{ID: input.ID, Revision: head + 1, AcceptedAt: newVersion.AcceptedAt}, func(ctx context.Context) error {
		_, err := s.db.Executor(ctx).ExecContext(ctx, `INSERT INTO research_claims(project,id,revision,claim) VALUES(?,?,?,?)`,
			input.Project, input.ID, head+1, string(encoded))
		return err
	})
}

func (s *store) readClaim(ctx context.Context, input wire.ClaimReadInput) (result wire.ClaimRead, err error) {
	if err := checkClaimProject(input.Project); err != nil {
		return result, err
	}
	err = s.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		var raw string
		var revision int64
		err := s.db.Executor(ctx).QueryRowContext(ctx,
			`SELECT revision,claim FROM research_claims WHERE project=? AND id=? AND (?=0 OR revision=?) ORDER BY revision DESC LIMIT 1`,
			input.Project, input.ID, input.Revision, input.Revision).Scan(&revision, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return knowledge.ErrNotFound
		}
		if err != nil {
			return err
		}
		result, err = s.decodeClaim(ctx, input.Project, input.ID, revision, raw)
		return err
	})
	return result, err
}

func (s *store) decodeClaim(ctx context.Context, project, id string, revision int64, raw string) (wire.ClaimRead, error) {
	var source claimVersion
	if err := json.Unmarshal([]byte(raw), &source, json.RejectUnknownMembers(true)); err != nil {
		return wire.ClaimRead{}, errors.Join(errStoredDocument, err)
	}
	return s.renderClaim(ctx, project, id, revision, source)
}

func (s *store) listClaims(ctx context.Context, input wire.ListInput) ([]wire.ClaimSummary, error) {
	if err := checkClaimProject(input.Project); err != nil {
		return nil, err
	}
	result := []wire.ClaimSummary{}
	err := s.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		rows, err := s.db.Executor(ctx).QueryContext(ctx,
			`SELECT r.id, r.revision, r.claim FROM research_claims r JOIN
			 (SELECT id, MAX(revision) AS revision FROM research_claims WHERE project=? GROUP BY id) h
			 ON r.id=h.id AND r.revision=h.revision WHERE r.project=? ORDER BY r.id`, input.Project, input.Project)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, raw string
			var revision int64
			if err := rows.Scan(&id, &revision, &raw); err != nil {
				return err
			}
			claim, err := s.decodeClaim(ctx, input.Project, id, revision, raw)
			if err != nil {
				return err
			}
			result = append(result, claim.ClaimSummary)
		}
		return rows.Err()
	})
	return result, err
}
