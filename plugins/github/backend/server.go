package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func classify(err error) string {
	if refusal, known := errors.AsType[*portable.Refusal](err); known {
		return refusal.Code
	}
	if errors.Is(err, portable.ErrInvocationConflict) {
		return "command_conflict"
	}
	if errors.Is(err, errResource) {
		return "invalid_resource"
	}
	return ""
}

func toolQuery[P, R any](name, description string, call func(context.Context, P) (R, error)) (tool.Tool, error) {
	return tool.NewFunc(tool.FuncConfig{Name: name, Description: description}, func(ctx context.Context, input P) (R, error) {
		result, err := call(ctx, input)
		_, known := errors.AsType[*portable.Refusal](err)
		if err == nil || !known && classify(err) == "" {
			return result, err
		}
		failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: chat.NewTextToolOutput(err.Error()), Cause: err})
		return result, errors.Join(failure, failureErr)
	})
}

func registerRead[P, R any](backend *qimumcp.Backend, tools *[]tool.Tool, method *qimumcp.Method[P, R], name string, call func(context.Context, P) (R, error)) error {
	modelTool, err := toolQuery(name, method.Descriptor().Description, call)
	if err != nil {
		return err
	}
	if err := method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[P]) (R, error) {
		return call(ctx, command.Input)
	}); err != nil {
		return err
	}
	*tools = append(*tools, modelTool)
	return nil
}

func newServer(connector *connector) (*sdk.Server, error) {
	if connector == nil {
		return nil, errors.New("github: connector is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.github", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	var tools []tool.Tool
	receipts := make(map[string]toolReceiptReader)
	toolReceipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		registerRead(backend, &tools, contracts.List, "list_issues", connector.listIssues),
		registerRead(backend, &tools, contracts.Issue, "get_issue", connector.getIssue),
		registerRead(backend, &tools, contracts.File, "read_file", connector.readFile),
		registerRead(backend, &tools, contracts.Pulls, "list_pull_requests", connector.listPulls),
		registerRead(backend, &tools, contracts.Pull, "get_pull_request", connector.getPull),
		registerRead(backend, &tools, contracts.PullFiles, "list_pull_request_files", connector.listPullFiles),
		registerRead(backend, &tools, contracts.Comments, "list_issue_comments", connector.listComments),
		registerRead(backend, &tools, contracts.Reviews, "list_pull_request_reviews", connector.listReviews),
		registerRead(backend, &tools, contracts.ReviewComments, "list_pull_request_comments", connector.listReviewComments),
		registerRead(backend, &tools, contracts.Checks, "list_check_runs", connector.listChecks),
		registerRead(backend, &tools, contracts.Annotations, "list_check_annotations", connector.listAnnotations),
		registerRead(backend, &tools, contracts.Statuses, "list_commit_statuses", connector.listStatuses),
		registerWrite(backend, &tools, receipts, connector, contracts.CreateIssue, "create_issue", connector.createIssue),
		registerWrite(backend, &tools, receipts, connector, contracts.UpdateIssue, "update_issue", connector.updateIssue),
		registerWrite(backend, &tools, receipts, connector, contracts.CreateComment, "create_issue_comment", connector.createComment),
		registerWrite(backend, &tools, receipts, connector, contracts.UpdateComment, "update_issue_comment", connector.updateComment),
		registerWrite(backend, &tools, receipts, connector, contracts.DeleteComment, "delete_issue_comment", connector.deleteComment),
		registerWrite(backend, &tools, receipts, connector, contracts.CreatePull, "create_pull_request", connector.createPull),
		registerWrite(backend, &tools, receipts, connector, contracts.SubmitReview, "submit_pull_request_review", connector.submitReview),
		registerWrite(backend, &tools, receipts, connector, contracts.MergePull, "merge_pull_request", connector.mergePull),
		registerWrite(backend, &tools, receipts, connector, contracts.CreateReviewComment, "create_pull_request_comment", connector.createReviewComment),
		registerWrite(backend, &tools, receipts, connector, contracts.ReplyReviewComment, "reply_pull_request_comment", connector.replyReviewComment),
		registerWrite(backend, &tools, receipts, connector, contracts.UpdateReviewComment, "update_pull_request_comment", connector.updateReviewComment),
		registerWrite(backend, &tools, receipts, connector, contracts.DeleteReviewComment, "delete_pull_request_comment", connector.deleteReviewComment),
		pluginstorage.RegisterReceipts(backend, connector.store.db),
		toolReceipts.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			read := receipts[command.Input.Method]
			if read == nil {
				return pluginsdk.ToolReceipt{}, nil
			}
			return read(ctx, command.Input)
		}),
		backend.BindCapability("github", contracts.Capability),
	); err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, tools...); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
