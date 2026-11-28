package main

import (
	"context"
	"errors"
	"strings"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/plugins/methods/internal/wire"
)

func (c *connector) board(ctx context.Context, in wire.ProjectInput) (wire.Board, error) {
	if _, err := identity.ParseProject(in.ProjectID); err != nil {
		return wire.Board{}, refuse("select a project")
	}
	latest, err := c.store.latest(ctx, in.ProjectID)
	if err != nil {
		return wire.Board{}, err
	}
	rows, err := c.store.executions(ctx, "SELECT value FROM method_executions WHERE project=? ORDER BY rowid DESC LIMIT 24", in.ProjectID)
	if err != nil {
		return wire.Board{}, err
	}
	out := wire.Board{Latest: latest, Executions: make([]wire.Execution, 0, len(rows))}
	for _, row := range rows {
		row.Patch = ""
		out.Executions = append(out.Executions, row.Execution)
	}
	return out, nil
}

func (c *connector) read(ctx context.Context, in wire.ExecutionInput) (wire.Execution, error) {
	record, err := c.store.execution(ctx, in.ProjectID, in.ID)
	return record.Execution, err
}

func (c *connector) save(ctx context.Context, in wire.SaveInput, key string) (pluginstorage.CommandPlan[wire.Version], error) {
	var zero pluginstorage.CommandPlan[wire.Version]
	if _, err := identity.ParseProject(in.ProjectID); err != nil {
		return zero, refuse("select a project")
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Change = strings.TrimSpace(in.Change)
	in.ReviewA = strings.TrimSpace(in.ReviewA)
	in.ReviewB = strings.TrimSpace(in.ReviewB)
	if in.Title == "" || len(in.Title) > 256 || in.Change == "" || len(in.Change) > 8192 {
		return zero, refuse("enter a method title and a bounded change instruction")
	}
	if in.ReviewA == "" || in.ReviewB == "" || in.ReviewA == in.ReviewB || len(in.ReviewA) > 4096 || len(in.ReviewB) > 4096 {
		return zero, refuse("enter two distinct review instructions")
	}
	selection, err := model.New(in.Provider, in.Model)
	if err != nil || selection.ValidateExact() != nil {
		return zero, refuse("select the exact provider and model for this version")
	}
	in.Provider, in.Model = selection.Provider(), selection.Model()
	versionID, err := methodDigest(struct {
		Input wire.SaveInput
		Key   string
	}{in, key})
	if err != nil {
		return zero, err
	}
	version := wire.Version{ID: versionID, Recipe: in.Recipe}
	return pluginstorage.NewCommandPlan(version, func(ctx context.Context) error {
		return c.store.putVersion(ctx, in.ProjectID, version)
	})
}

func (c *connector) begin(ctx context.Context, in wire.StartInput, key string) (plan pluginstorage.CommandPlan[wire.Execution], err error) {
	version, err := c.store.version(ctx, in.ProjectID, in.VersionID)
	if err != nil {
		return plan, err
	}
	host, err := hostClient(ctx)
	if err != nil {
		return plan, err
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	project, err := host.GetProject(ctx, protocol.GetProjectRequest{ProjectID: in.ProjectID}, protocol.CallOptions{})
	if err != nil {
		return plan, err
	}
	if project.Archived {
		return plan, refuse("the project is archived")
	}
	trees, err := host.ListTaskWorktrees(ctx, protocol.CallOptions{})
	if err != nil {
		return plan, err
	}
	var worktree protocol.TaskWorktree
	for _, candidate := range trees.Data {
		if candidate.ID == in.WorktreeID && candidate.Available {
			worktree = candidate
			break
		}
	}
	if worktree.ID == "" {
		return plan, refuse("select an available managed worktree")
	}
	active, err := c.store.openWorktree(ctx, worktree.Workspace.Path)
	if err != nil {
		return plan, err
	}
	if active {
		return plan, refuse("the worktree already has an active method execution")
	}
	discovered, err := host.Discover(ctx, protocol.CallOptions{})
	if err != nil {
		return plan, err
	}
	namespace := discovered.Capabilities.Limits.Idempotency.Namespace
	if namespace == "" {
		return plan, protocol.ErrInvalidResponse
	}
	id, err := methodDigest(key)
	if err != nil {
		return plan, err
	}
	work := record{Execution: wire.Execution{
		ID: id, WorktreeID: worktree.ID, ProjectID: in.ProjectID, Version: *version,
		Base: worktree.Base, Workspace: worktree.Workspace.Path, State: wire.Queued,
		Checks: []wire.Review{},
	}, Namespace: namespace}
	preview, err := currentWorktree(ctx, host, work)
	if err != nil {
		return plan, err
	}
	work.Before = preview.Digest
	return pluginstorage.NewCommandPlan(work.Execution, func(ctx context.Context) error { return c.store.putExecution(ctx, work) })
}

func (c *connector) approve(ctx context.Context, in wire.ApproveInput, _ string) (plan pluginstorage.CommandPlan[wire.Execution], err error) {
	work, err := c.store.execution(ctx, in.ProjectID, in.ID)
	if err != nil {
		return plan, err
	}
	if work.State != wire.Awaiting || work.Digest == "" || work.Digest != in.Digest {
		return plan, refuse("select the current review report for approval")
	}
	disposition := in.Decision
	switch disposition {
	case "reject":
		work.State = wire.Blocked
		work.Reason = "A person rejected the original report."
	case "", "approve":
		host, err := hostClient(ctx)
		if err != nil {
			return plan, err
		}
		defer func() { err = errors.Join(err, host.Close()) }()
		preview, err := currentWorktree(ctx, host, work)
		if err != nil {
			return plan, err
		}
		if preview.Digest != work.Handoff {
			return plan, refuse("the worktree or handoff target changed after review")
		}
		disposition = "approve"
		work.State = wire.Approved
	default:
		return plan, refuse("choose approval or rejection of the original report")
	}
	work.Decision = &wire.Decision{Digest: work.Digest, Disposition: disposition, At: instant.Timestamp{Time: time.Now().UTC()}}
	return pluginstorage.NewCommandPlan(work.Execution, func(ctx context.Context) error { return c.store.update(ctx, work) })
}

func hostClient(ctx context.Context) (*runtimeclient.Client, error) {
	ref, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return runtimeclient.ConnectCallback(ctx, ref)
}

func currentWorktree(ctx context.Context, host *runtimeclient.Client, work record) (*protocol.WorktreeHandoff, error) {
	preview, err := host.PreviewWorktreeHandoff(ctx, protocol.TaskWorktreeRequest{ID: work.WorktreeID}, protocol.CallOptions{})
	if err != nil {
		if errors.Is(err, protocol.ErrWorkspaceUnavailable) || errors.Is(err, protocol.ErrRevisionConflict) {
			return nil, refuse("the captured worktree or handoff target is unavailable")
		}
		return nil, err
	}
	task := preview.Worktree
	if !task.Available || task.ID != work.WorktreeID || task.Base != work.Base || task.Workspace.Path != work.Workspace || preview.Digest == "" {
		return nil, refuse("the original managed worktree binding changed")
	}
	return preview, nil
}
