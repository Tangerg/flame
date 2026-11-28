package main

import (
	"context"
	_ "embed"
	json "encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	"github.com/Tangerg/frame"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

const teamPagePath = "/flame/team"

//go:embed team_page.html
var teamPage []byte

//go:embed team_page.js
var teamScript []byte

//go:embed team_page.css
var teamStyle []byte

func (c *connector) registerTeamHTTP(mux *http.ServeMux, host *runtimeclient.Client) error {
	if host == nil {
		return nil
	}
	if p := c.intakePolicy; p != nil {
		if len(p.linearSecret) > 0 {
			mux.HandleFunc("POST "+linearPath, func(w http.ResponseWriter, r *http.Request) { c.receiveLinear(w, r, host) })
		}
		if len(p.slackSecret) > 0 {
			mux.HandleFunc("POST "+slackPath, func(w http.ResponseWriter, r *http.Request) { c.receiveSlack(w, r, host) })
		}
	}
	if c.teamPolicy == nil {
		return nil
	}
	publish, err := issuePublishMethod()
	if err != nil {
		return err
	}
	mux.HandleFunc("GET "+teamPagePath, teamAsset("text/html; charset=utf-8", teamPage))
	mux.HandleFunc("GET "+teamPagePath+"/app.js", teamAsset("text/javascript; charset=utf-8", teamScript))
	mux.HandleFunc("GET "+teamPagePath+"/app.css", teamAsset("text/css; charset=utf-8", teamStyle))
	mux.HandleFunc("GET "+teamPagePath+"/task", func(w http.ResponseWriter, r *http.Request) { c.readTeamHTTP(w, r, host, publish, false) })
	mux.HandleFunc("GET "+teamPagePath+"/evidence", func(w http.ResponseWriter, r *http.Request) { c.readTeamHTTP(w, r, host, publish, true) })
	mux.HandleFunc("POST "+teamPagePath+"/approve", func(w http.ResponseWriter, r *http.Request) { c.approveTeamHTTP(w, r, host, publish) })
	return nil
}

func teamHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func teamAsset(contentType string, asset []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamHeaders(w)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(asset)
	}
}

func portalJSON(w http.ResponseWriter, result any) {
	teamHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.MarshalWrite(w, result)
}

func portalFailure(w http.ResponseWriter, status int) {
	teamHeaders(w)
	w.WriteHeader(status)
}

func portalError(ctx context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, pluginstorage.ErrCommandUnconfirmed) {
		portalFailure(w, http.StatusAccepted)
		return
	}
	if known, ok := errors.AsType[*framecall.Refusal](err); ok {
		switch known.Code {
		case refusalNotFound, refusalNotAllowed, refusalForbidden, refusalUnauthorized, refusalCredentialRequired:
			portalFailure(w, http.StatusForbidden)
		case refusalConflict:
			portalFailure(w, http.StatusConflict)
		default:
			portalFailure(w, http.StatusBadRequest)
		}
		return
	}
	slog.WarnContext(ctx, "github: team request", "error", err)
	portalFailure(w, http.StatusServiceUnavailable)
}

func (c *connector) teamBearer(r *http.Request) (string, bool) {
	if len(r.Header.Values("Authorization")) != 1 {
		return "", false
	}
	auth, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	id, mac, pair := strings.Cut(auth, ".")
	if !bearer || !pair || !intakeIdentity(id) || !validSignature(c.teamPolicy.secret, mac, []byte("flame:team:v1:"+c.teamScope()+":"+id)) {
		return "", false
	}
	return id, true
}

func (c *connector) authorizeTeam(ctx context.Context, id string, machine, publish bool) (wire.TeamGrant, error) {
	grant, err := c.store.grant(ctx, c.teamScope(), id)
	if err != nil {
		return wire.TeamGrant{}, err
	}
	if !grant.ProjectAccess || grant.RevokedAt != nil || !grant.ExpiresAt.After(time.Now()) ||
		(machine && !grant.MachineAccess) || (publish && grant.Role != teamPublisher) {
		return wire.TeamGrant{}, refused(refusalNotAllowed, "team access is unavailable")
	}
	return grant, nil
}

type teamTaskView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	HasEvidence bool   `json:"hasEvidence"`
}

type teamEvidenceView struct {
	Head        string                     `json:"head"`
	SessionID   string                     `json:"sessionId"`
	Workspace   string                     `json:"workspace"`
	Project     *wire.ProjectVerification  `json:"project,omitzero"`
	CI          *wire.IssueVerificationCI  `json:"ci,omitzero"`
	Draft       string                     `json:"draft,omitempty"`
	Digest      string                     `json:"digest,omitempty"`
	CanPublish  bool                       `json:"canPublish"`
	Publication string                     `json:"publication,omitempty"`
	Receipt     *wire.IssueDeliveryReceipt `json:"receipt,omitzero"`
}

func (c *connector) readTeamHTTP(w http.ResponseWriter, r *http.Request, host *runtimeclient.Client, method *frame.Method, evidence bool) {
	id, signed := c.teamBearer(r)
	if !signed {
		portalFailure(w, http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	grant, err := c.authorizeTeam(ctx, id, evidence, false)
	if err != nil {
		portalError(ctx, w, err)
		return
	}
	work, err := c.teamWork(ctx, host, grant.ProjectID, grant.DeliveryID)
	if err == nil && evidence {
		err = checkTeamMachine(ctx, host, work)
	}
	if err != nil {
		portalError(ctx, w, err)
		return
	}
	if _, err := c.authorizeTeam(ctx, id, evidence, false); err != nil {
		portalError(ctx, w, err)
		return
	}
	if !evidence {
		portalJSON(w, teamTaskView{ID: work.ID, Title: work.Title, URL: work.IssueURL, Status: work.Status,
			HasEvidence: grant.MachineAccess && work.Namespace != ""})
		return
	}
	value := teamEvidenceView{Head: work.Head, Workspace: work.Workspace, SessionID: work.SessionID, Project: work.Project, CI: work.CI}
	if grant.Role == teamReviewer || grant.Role == teamPublisher {
		value.Draft, value.Digest = work.Draft, work.Digest
		value.CanPublish = grant.Role == teamPublisher && work.Status == issueReady && work.Digest != ""
	}
	if work.Publication != nil {
		receipt, err := c.issuePublication(ctx, work, method)
		switch {
		case errors.Is(err, pluginstorage.ErrCommandUnconfirmed):
			value.Publication = "unconfirmed"
			value.CanPublish = grant.Role == teamPublisher &&
				work.Publication.Key == mobileApprovalKey(grant.ID, work.Digest)
		case err != nil:
			value.Publication = "refused"
		default:
			value.Publication, value.Receipt = "published", &receipt
		}
	}
	if _, err := c.authorizeTeam(ctx, id, true, false); err != nil {
		portalError(ctx, w, err)
		return
	}
	portalJSON(w, value)
}

func mobileApprovalKey(id, digest string) string {
	return "team:" + digestText(id+"\n"+digest)
}

func (c *connector) approveTeamHTTP(w http.ResponseWriter, r *http.Request, host *runtimeclient.Client, method *frame.Method) {
	id, signed := c.teamBearer(r)
	if !signed {
		portalFailure(w, http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	grant, err := c.authorizeTeam(ctx, id, true, true)
	if err != nil {
		portalError(ctx, w, err)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	var input wire.PublishIssueDeliveryInput
	if err != nil || json.Unmarshal(raw, &input, json.RejectUnknownMembers(true)) != nil ||
		input.ID != grant.DeliveryID || len(input.Digest) != 64 {
		portalFailure(w, http.StatusBadRequest)
		return
	}
	data, err := framevalue.EncodeJSON(input)
	if err != nil {
		portalError(ctx, w, err)
		return
	}
	if err := method.ValidateInput(data); err != nil {
		portalFailure(w, http.StatusBadRequest)
		return
	}
	work, err := c.teamWork(ctx, host, grant.ProjectID, grant.DeliveryID)
	if err == nil {
		err = checkTeamMachine(ctx, host, work)
	}
	if err != nil {
		portalError(ctx, w, err)
		return
	}
	authorize := func(ctx context.Context, admitting bool) error {
		if _, err := c.authorizeTeam(ctx, id, true, true); err != nil {
			return err
		}
		if admitting {
			return c.store.recordTeamEvent(ctx, c.teamScope(), id, "publicationApproved", time.Now().UTC())
		}
		return nil
	}
	receipt, err := c.publishAuthorizedIssue(ctx, input, mobileApprovalKey(id, input.Digest), data, method, authorize)
	c.noticeTeamChange()
	if err != nil {
		portalError(ctx, w, err)
		return
	}
	if err := authorize(ctx, false); err != nil {
		portalError(ctx, w, err)
		return
	}
	portalJSON(w, receipt)
}
