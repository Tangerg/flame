package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

const (
	linearPath   = "/flame/intake/linear"
	slackPath    = "/flame/intake/slack"
	intakeSchema = `CREATE TABLE IF NOT EXISTS work_sources (
		scope TEXT NOT NULL, kind TEXT NOT NULL, source_id TEXT NOT NULL,
		title TEXT NOT NULL, url TEXT NOT NULL, received_at INTEGER NOT NULL,
		UNIQUE(scope,kind,source_id)
	);`
)

func (p *intakePolicy) sourceScope(kind string) string {
	switch kind {
	case "linear":
		return p.ProjectID + ":" + p.LinearOrganization + ":" + p.LinearTeam
	case "slack":
		return p.ProjectID + ":" + p.SlackTeam + ":" + p.SlackChannel + ":" + p.SlackOrigin
	default:
		panic("unknown work intake source")
	}
}

func (s *store) retainSource(ctx context.Context, scope string, source wire.WorkSource) (bool, error) {
	var created bool
	err := s.db.RunInTx(ctx, func(tx context.Context) error {
		r, err := s.db.Executor(tx).ExecContext(tx, `INSERT INTO work_sources(scope,kind,source_id,title,url,received_at)
			VALUES(?,?,?,?,?,?) ON CONFLICT(scope,kind,source_id) DO NOTHING`,
			scope, source.Kind, source.ID, source.Title, source.URL, source.ReceivedAt.UnixNano())
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		created = n == 1
		return err
	})
	return created, err
}

func (s *store) listSources(ctx context.Context, policy *intakePolicy, sources *[]wire.WorkSource) error {
	return s.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		rows, err := s.db.Executor(tx).QueryContext(tx, `SELECT kind,source_id,title,url,received_at FROM work_sources
			WHERE (kind='linear' AND scope=?) OR (kind='slack' AND scope=?) ORDER BY rowid DESC LIMIT 100`,
			policy.sourceScope("linear"), policy.sourceScope("slack"))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var source wire.WorkSource
			var when int64
			if err := rows.Scan(&source.Kind, &source.ID, &source.Title, &source.URL, &when); err != nil {
				return err
			}
			source.ReceivedAt = teamTime(when)
			*sources = append(*sources, source)
		}
		return rows.Err()
	})
}

type linearEvent struct {
	Type         string `json:"type"`
	Action       string `json:"action"`
	Organization string `json:"organizationId"`
	Timestamp    int64  `json:"webhookTimestamp"`
	URL          string `json:"url"`
	Data         struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		TeamID string `json:"teamId"`
	} `json:"data"`
}

func (c *connector) receiveLinear(w http.ResponseWriter, r *http.Request, host *runtimeclient.Client) {
	raw, ok := intakeBody(w, r)
	if !ok {
		return
	}
	policy := c.intakePolicy
	if !validSignature(policy.linearSecret, r.Header.Get("Linear-Signature"), raw) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var event linearEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	if !freshEvent(now, time.UnixMilli(event.Timestamp), time.Minute) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if event.Type != "Issue" || event.Action != "create" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if event.Organization != policy.LinearOrganization || event.Data.TeamID != policy.LinearTeam {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	link, err := url.Parse(event.URL)
	if err != nil || !intakeIdentity(event.Data.ID) || link.Scheme != "https" || link.Host != "linear.app" ||
		link.User != nil || !strings.Contains(link.Path, "/issue/") || link.RawQuery != "" || link.Fragment != "" || len(event.URL) > 2048 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c.saveIntake(w, r, host, wire.WorkSource{
		Kind: "linear", ID: event.Data.ID, Title: event.Data.Title, URL: link.String(), ReceivedAt: instant.Timestamp{Time: now},
	})
}

type slackEvent struct {
	Type      string `json:"type"`
	Team      string `json:"team_id"`
	ID        string `json:"event_id"`
	Challenge string `json:"challenge"`
	Event     struct {
		Type      string `json:"type"`
		Channel   string `json:"channel"`
		Timestamp string `json:"ts"`
		Text      string `json:"text"`
		BotID     string `json:"bot_id"`
	} `json:"event"`
}

func (c *connector) receiveSlack(w http.ResponseWriter, r *http.Request, host *runtimeclient.Client) {
	raw, ok := intakeBody(w, r)
	if !ok {
		return
	}
	policy := c.intakePolicy
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	now := time.Now().UTC()
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	signature, version := strings.CutPrefix(r.Header.Get("X-Slack-Signature"), "v0=")
	if err != nil || !version || !freshEvent(now, time.Unix(seconds, 0), 5*time.Minute) ||
		!validSignature(policy.slackSecret, signature, []byte("v0:"+timestamp+":"), raw) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var event slackEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if event.Type == "url_verification" && event.Challenge != "" && len(event.Challenge) <= 512 {
		portalJSON(w, struct {
			Challenge string `json:"challenge"`
		}{event.Challenge})
		return
	}
	if event.Type != "event_callback" || event.Event.Type != "app_mention" || event.Event.BotID != "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if event.Team != policy.SlackTeam || event.Event.Channel != policy.SlackChannel {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	whole, fraction, found := strings.Cut(event.Event.Timestamp, ".")
	if !intakeIdentity(event.ID) || !found || !slackTimestamp(whole, fraction) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c.saveIntake(w, r, host, wire.WorkSource{
		Kind: "slack", ID: event.ID, Title: event.Event.Text,
		URL:        policy.SlackOrigin + "/archives/" + policy.SlackChannel + "/p" + whole + fraction,
		ReceivedAt: instant.Timestamp{Time: now},
	})
}

func slackTimestamp(whole, fraction string) bool {
	if len(whole) < 10 || len(whole) > 12 || len(fraction) != 6 {
		return false
	}
	return strings.IndexFunc(whole+fraction, func(r rune) bool { return r < '0' || r > '9' }) == -1
}

func freshEvent(now, timestamp time.Time, window time.Duration) bool {
	age := now.Sub(timestamp)
	return age >= -window && age <= window
}

func intakeBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		w.WriteHeader(status)
	}
	return raw, err == nil
}

func (c *connector) saveIntake(w http.ResponseWriter, r *http.Request, host *runtimeclient.Client, source wire.WorkSource) {
	ctx := r.Context()
	project, err := host.GetProject(ctx, protocol.GetProjectRequest{ProjectID: c.intakePolicy.ProjectID}, protocol.CallOptions{})
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if project.Archived {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	source.Title, _ = limitReviewEvidence(strings.TrimSpace(source.Title), 512)
	if source.Title == "" {
		source.Title = source.ID
	}
	created, err := c.store.retainSource(ctx, c.intakePolicy.sourceScope(source.Kind), source)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if created {
		c.noticeTeamChange()
	}
	w.WriteHeader(http.StatusOK)
}
