package main

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/github/internal/wire"

	"github.com/Tangerg/frame"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

const issuePublishing = "publishing"

func (c *connector) listIssueDeliveries(ctx context.Context, input wire.IssueDeliveriesInput, method *frame.Method) (wire.IssueDeliveryPage, error) {
	page := wire.IssueDeliveryPage{Items: []wire.IssueDelivery{}}
	if p := c.issuePolicy; p == nil || p.ProjectID != input.ProjectID {
		return page, nil
	}
	scope := c.issueScope()
	rows, err := c.store.issueDeliveries(ctx, scope, input.Before, false)
	if err != nil || len(rows) == 0 {
		return page, err
	}
	repoID, err := c.issueRepositoryID(ctx)
	if issueAccessLost(err) {
		return page, nil
	}
	if err != nil {
		return page, err
	}
	if len(rows) == 25 {
		rows = rows[:24]
		page.Next = rows[23].Number
	}
	for _, row := range rows {
		if row.Status == issueRevoked {
			continue
		}
		issue, err := c.currentIssue(ctx, row, repoID)
		if issueAccessLost(err) {
			continue
		}
		if err != nil {
			return page, err
		}
		view := row.IssueDelivery
		if view.Title == "" {
			view.Title, _ = limitReviewEvidence(issue.Title, 512)
			view.IssueURL = issue.URL
		}
		if row.Publication != nil {
			view.ApprovedAt = &instant.Timestamp{Time: row.Publication.Approved}
			receipt, err := c.issuePublication(ctx, row, method)
			switch {
			case errors.Is(err, pluginstorage.ErrCommandUnconfirmed):
				view.Status = "publicationUnconfirmed"
			case err != nil:
				if _, known := classify(err); !known {
					return page, err
				}
				view.Status, view.Reason = "publicationRefused", err.Error()
			default:
				view.Status, view.Receipt = "published", &receipt
			}
		}
		page.Items = append(page.Items, view)
	}
	return page, nil
}

func (c *connector) issuePublication(ctx context.Context, work issueDeliveryRecord, method *frame.Method) (wire.IssueDeliveryReceipt, error) {
	prepared, _, reply, err := pluginstorage.ResumeCommand(ctx, c.store.db, method.Name(), work.Publication.Key)
	if err != nil {
		return wire.IssueDeliveryReceipt{}, err
	}
	if prepared != nil {
		return wire.IssueDeliveryReceipt{}, pluginstorage.ErrCommandUnconfirmed
	}
	return decodeWriteReply[wire.IssueDeliveryReceipt](method, reply)
}

func (c *connector) publishIssueDelivery(ctx context.Context, command pluginmcp.Command[wire.PublishIssueDeliveryInput], method *frame.Method) (wire.IssueDeliveryReceipt, error) {
	var zero wire.IssueDeliveryReceipt
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return zero, err
	}
	if invocation.Origin != extension.Human {
		return zero, refused(refusalNotAllowed, "a person must approve publication")
	}
	admitted, err := command.RequestInput()
	if err != nil {
		return zero, err
	}
	return c.publishAuthorizedIssue(ctx, command.Input, string(command.ID), admitted, method, nil)
}

type issuePublishAuthorization func(context.Context, bool) error

func (c *connector) publishAuthorizedIssue(ctx context.Context, input wire.PublishIssueDeliveryInput, key string, admitted framevalue.JSON, method *frame.Method, authorize issuePublishAuthorization) (wire.IssueDeliveryReceipt, error) {
	var zero wire.IssueDeliveryReceipt
	if c.issuePolicy == nil {
		return zero, refused(refusalNotFound, "issue delivery is not configured")
	}
	if authorize != nil {
		if err := authorize(ctx, false); err != nil {
			return zero, err
		}
	}
	scope := c.issueScope()
	work, err := c.store.readIssueDelivery(ctx, scope, input.ID)
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
	if work.Publication != nil {
		if work.Publication.Key != key || work.Publication.Digest != input.Digest {
			return zero, refused(refusalConflict, "the report already has an original publication command")
		}
		return c.issuePublication(ctx, work, method)
	}
	if work.Status != issueReady || work.Digest != input.Digest {
		return zero, refused(refusalConflict, "the report is not ready at the approved evidence revision")
	}
	if err := c.checkIssueApproval(ctx, work, issue); err != nil {
		return zero, err
	}
	prepared, replay, err := pluginstorage.AdmitCommand(ctx, c.store.db, method.Name(), key, admitted, func(ctx context.Context) error {
		if authorize != nil {
			if err := authorize(ctx, true); err != nil {
				return err
			}
		}
		work.Publication = &issueApproval{Key: key, Digest: input.Digest, Approved: time.Now().UTC()}
		work.Status = issuePublishing
		return c.store.saveIssueDelivery(ctx, scope, work)
	})
	if err != nil {
		return zero, err
	}
	if prepared == nil {
		return decodeWriteReply[wire.IssueDeliveryReceipt](method, replay)
	}
	if !prepared.NewlyAdmitted() {
		return zero, pluginstorage.ErrCommandUnconfirmed
	}
	result, outcomeErr := c.sendApprovedIssueDelivery(ctx, work, authorize)
	reply, err := pluginmcp.EncodeReply(result, outcomeErr, classify)
	if err != nil {
		return zero, err
	}
	if err := prepared.SettleExternal(ctx, reply.Bytes(), nil); err != nil {
		return zero, err
	}
	return decodeWriteReply[wire.IssueDeliveryReceipt](method, reply.Bytes())
}

func (c *connector) checkIssueApproval(ctx context.Context, work issueDeliveryRecord, issue wire.Issue) error {
	source, err := issueSourceVersion(issue)
	if err != nil {
		return err
	}
	if source != work.Source {
		return refused(refusalConflict, "the source issue changed after capture")
	}
	if err := checkIssueWorktree(ctx, work); err != nil {
		return err
	}
	ci, err := c.issueCI(ctx, work)
	if err != nil {
		return err
	}
	if ci.Status != "passed" {
		return refused(refusalConflict, "complete, passing GitHub checks are required")
	}
	work.CI = &ci
	_, digest, err := issueDraft(work)
	if err != nil {
		return err
	}
	if digest != work.Digest {
		return refused(refusalConflict, "verification evidence changed before approval")
	}
	return c.checkIssueHead(ctx, work)
}

func (c *connector) checkIssueHead(ctx context.Context, work issueDeliveryRecord) error {
	head, err := c.issueBranchHead(ctx)
	if err != nil {
		return err
	}
	if head != work.Head {
		return refused(refusalConflict, "the upstream branch head changed after verification")
	}
	return nil
}

func (c *connector) sendApprovedIssueDelivery(ctx context.Context, work issueDeliveryRecord, authorize issuePublishAuthorization) (wire.IssueDeliveryReceipt, error) {
	var zero wire.IssueDeliveryReceipt
	repoID, err := c.issueRepositoryID(ctx)
	if err != nil {
		return zero, err
	}
	issue, err := c.currentIssue(ctx, work, repoID)
	if err != nil {
		return zero, err
	}
	if err := c.checkIssueApproval(ctx, work, issue); err != nil {
		return zero, err
	}
	if authorize != nil {
		if err := authorize(ctx, false); err != nil {
			return zero, err
		}
	}
	comment, err := c.createComment(ctx, wire.CreateCommentInput{IssueInput: work.IssueInput, Body: work.Draft})
	if err != nil {
		return zero, err
	}
	if comment.Body != work.Draft {
		return zero, errors.New("github: acknowledged comment differs from approved report")
	}
	return wire.IssueDeliveryReceipt{Head: work.Head, Digest: work.Digest, Comment: comment}, nil
}

func issueConflict(err error) bool {
	r, ok := errors.AsType[*framecall.Refusal](err)
	return ok && r.Code == refusalConflict
}
