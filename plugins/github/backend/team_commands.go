package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const (
	teamViewer    = "viewer"
	teamReviewer  = "reviewer"
	teamPublisher = "publisher"
	teamLifetime  = 7 * 24 * time.Hour
)

func requireTeamOwner(ctx context.Context) error {
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return err
	}
	if invocation.Origin != extension.Human {
		return refused(refusalNotAllowed, "a person must manage team access")
	}
	return nil
}

func registerTeamQuery[P, R any](backend *pluginmcp.Backend, method *pluginmcp.Method[P, R], handle func(context.Context, P) (R, error)) error {
	return method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[P]) (R, error) {
		var zero R
		if err := requireTeamOwner(ctx); err != nil {
			return zero, err
		}
		return handle(ctx, command.Input)
	})
}

func registerTeamCommand[P, R any](backend *pluginmcp.Backend, c *connector, method *pluginmcp.Method[P, R], confirm func(context.Context, P) (pluginstorage.CommandPlan[R], error)) error {
	return method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[P]) (R, error) {
		var zero R
		if err := requireTeamOwner(ctx); err != nil {
			return zero, err
		}
		raw, err := command.RequestInput()
		if err != nil {
			return zero, err
		}
		result, err := pluginstorage.Command(ctx, c.store.db, method.Name(), string(command.ID), raw, classify, confirm)
		if err == nil {
			c.noticeTeamChange()
		}
		return result, err
	})
}

func (c *connector) noticeTeamChange() {
	select {
	case c.teamWake <- struct{}{}:
	default:
	}
}

func (c *connector) teamProject(project string) bool {
	return c.teamPolicy != nil && c.issuePolicy != nil && c.issuePolicy.ProjectID == project
}

func (c *connector) teamScope() string {
	return c.issueScope() + ":" + c.issuePolicy.digest
}

func (c *connector) projectTeam(ctx context.Context, request wire.TeamProject) (wire.TeamBoard, error) {
	board := wire.TeamBoard{Sharing: c.teamProject(request.ProjectID),
		Grants: []wire.TeamGrant{}, Audit: []wire.TeamAuditEvent{}, Sources: []wire.WorkSource{}}
	if board.Sharing {
		if err := c.store.teamBoard(ctx, c.teamScope(), &board); err != nil {
			return wire.TeamBoard{}, err
		}
	}
	if c.intakePolicy != nil && c.intakePolicy.ProjectID == request.ProjectID {
		err := c.store.listSources(ctx, c.intakePolicy, &board.Sources)
		return board, err
	}
	return board, nil
}

func (c *connector) grantTeamWork(ctx context.Context, input wire.TeamGrantInput) (plan pluginstorage.CommandPlan[wire.TeamGrant], err error) {
	if !c.teamProject(input.ProjectID) {
		return plan, refused(refusalNotFound, "team sharing is unavailable for this project")
	}
	input.Member = strings.TrimSpace(input.Member)
	if input.Member == "" || len(input.Member) > 120 {
		return plan, refused(refusalInvalidRequest, "enter a team member name")
	}
	switch input.Role {
	case teamViewer, teamReviewer, teamPublisher:
	default:
		return plan, refused(refusalInvalidRequest, "choose a project role")
	}
	ref, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return plan, err
	}
	host, err := runtimeclient.ConnectCallback(ctx, ref)
	if err != nil {
		return plan, err
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	work, err := c.readTeamWork(ctx, host, input.ProjectID, input.DeliveryID)
	if err != nil {
		return plan, err
	}
	if input.MachineAccess {
		if err := checkTeamMachine(ctx, host, work); err != nil {
			return plan, err
		}
	}
	now := time.Now().UTC()
	grant := wire.TeamGrant{ID: rand.Text(), TeamGrantInput: input, ExpiresAt: instant.Timestamp{Time: now.Add(teamLifetime)}}
	return pluginstorage.NewCommandPlan(grant, func(tx context.Context) error { return c.store.addGrant(tx, c.teamScope(), grant, now) })
}

func (c *connector) revokeTeamWork(ctx context.Context, input wire.TeamGrantRef) (pluginstorage.CommandPlan[wire.TeamGrant], error) {
	var zero pluginstorage.CommandPlan[wire.TeamGrant]
	if !c.teamProject(input.ProjectID) {
		return zero, refused(refusalNotFound, "team sharing is unavailable for this project")
	}
	grant, err := c.store.grant(ctx, c.teamScope(), input.ID)
	if err != nil {
		return zero, err
	}
	if grant.RevokedAt != nil {
		return pluginstorage.NewCommandPlan(grant, nil)
	}
	now := time.Now().UTC()
	grant.RevokedAt = &instant.Timestamp{Time: now}
	return pluginstorage.NewCommandPlan(grant, func(tx context.Context) error { return c.store.revokeGrant(tx, c.teamScope(), input.ID, now) })
}

func (c *connector) teamInvite(ctx context.Context, input wire.TeamGrantRef) (wire.TeamInvite, error) {
	if !c.teamProject(input.ProjectID) {
		return wire.TeamInvite{}, refused(refusalNotFound, "team sharing is unavailable for this project")
	}
	grant, err := c.store.grant(ctx, c.teamScope(), input.ID)
	if err != nil {
		return wire.TeamInvite{}, err
	}
	if grant.RevokedAt != nil || !grant.ExpiresAt.After(time.Now()) {
		return wire.TeamInvite{}, refused(refusalNotAllowed, "the team grant is no longer active")
	}
	mac := signText(c.teamPolicy.secret, []byte("flame:team:v1:"+c.teamScope()+":"+grant.ID))
	return wire.TeamInvite{URL: c.teamPolicy.origin + teamPagePath + "#" + grant.ID + "." + hex.EncodeToString(mac)}, nil
}

func (c *connector) readTeamWork(ctx context.Context, host *runtimeclient.Client, project, delivery string) (issueDeliveryRecord, error) {
	var zero issueDeliveryRecord
	if !c.teamProject(project) {
		return zero, refused(refusalNotFound, "the project is unavailable")
	}
	active, err := host.GetProject(ctx, protocol.GetProjectRequest{ProjectID: project}, protocol.CallOptions{})
	if err != nil {
		return zero, err
	}
	if active.Archived {
		return zero, refused(refusalNotAllowed, "the project is archived")
	}
	work, err := c.store.readIssueDelivery(ctx, c.issueScope(), delivery)
	if err != nil {
		return zero, err
	}
	if work.Status == issueRevoked || work.Rule != c.issuePolicy.digest {
		return zero, refused(refusalNotAllowed, "the delivery is unavailable")
	}
	return work, nil
}

func (c *connector) teamWork(ctx context.Context, host *runtimeclient.Client, project, delivery string) (issueDeliveryRecord, error) {
	var zero issueDeliveryRecord
	work, err := c.readTeamWork(ctx, host, project, delivery)
	if err != nil {
		return zero, err
	}
	repoID, err := c.issueRepositoryID(ctx)
	if err != nil {
		return zero, err
	}
	issue, err := c.currentIssue(ctx, work, repoID)
	if err != nil {
		return zero, err
	}
	if work.Title == "" {
		work.Title, _ = limitReviewEvidence(issue.Title, 512)
		work.IssueURL = issue.URL
	}
	return work, nil
}

func checkTeamMachine(ctx context.Context, host *runtimeclient.Client, work issueDeliveryRecord) error {
	if work.Namespace == "" || work.RunID == "" || work.Head == "" {
		return refused(refusalNotAllowed, "no original Runtime work is available to share")
	}
	runtime, err := host.Discover(ctx, protocol.CallOptions{})
	if err != nil {
		return err
	}
	if runtime.Capabilities.Limits.Idempotency.Namespace != work.Namespace {
		return refused(refusalNotAllowed, "the original Runtime store is unavailable")
	}
	original, err := host.GetRun(ctx, protocol.GetRunRequest{RunID: work.RunID}, protocol.CallOptions{})
	if errors.Is(err, protocol.ErrRunNotFound) {
		return refused(refusalNotAllowed, "the original Run is no longer available")
	}
	if err != nil {
		return err
	}
	if original.ID != work.RunID || original.SessionID != work.SessionID {
		return refused(refusalNotAllowed, "the original Run binding changed")
	}
	return nil
}

func issuePublishMethod() (*frame.Method, error) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	return frame.AdmitMethod(contracts.PublishIssueDelivery.Descriptor())
}
