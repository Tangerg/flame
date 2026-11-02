package plugin

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

const ToolReceiptMethod = "io.github.tangerg.flame/tools.getResult"

// ToolReceipt projects a private receipt through the original Tool's output
// codec. The business owner retains its only durable result representation.
type ToolReceipt struct {
	Result *ToolResult `json:"result,omitzero"`
}

type ToolResult struct {
	Output  chat.ToolOutput `json:"output"`
	IsError bool            `json:"isError,omitzero"`
}

func ToolReceiptQuery() (*qimumcp.Method[qimumcp.ReceiptRequest, ToolReceipt], error) {
	return qimumcp.NewMethod[qimumcp.ReceiptRequest, ToolReceipt](qimumcp.MethodConfig{Name: ToolReceiptMethod, Kind: portable.Query, Target: extension.ScopeInstallation, Description: ToolReceiptMethod})
}

func NewToolReceipt(output chat.ToolOutput, failed bool) (ToolReceipt, error) {
	if err := output.Validate(); err != nil {
		return ToolReceipt{}, err
	}
	return ToolReceipt{Result: &ToolResult{Output: output.Clone(), IsError: failed}}, nil
}

// ReadToolReceipt binds the original remote Tool, logical call and admitted JSON.
// A missing or unsupported read never permits another Tool invocation.
func ReadToolReceipt(ctx context.Context, session *sdk.ClientSession, callID, remote string, admitted jsonvalue.Value) (*chat.ToolOutput, error) {
	if session == nil {
		return nil, errors.New("plugin: client session is required")
	}
	if err := admitted.Validate(); err != nil {
		return nil, err
	}
	id, err := toolCommandID(callID)
	if err != nil {
		return nil, err
	}
	request := qimumcp.ReceiptRequest{ID: qimumcp.CommandID(id), Method: remote, InputDigest: admitted.Digest()}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	method, err := ToolReceiptQuery()
	if err != nil {
		return nil, err
	}
	receipt, err := method.Call(ctx, session, request)
	if err != nil {
		return nil, err
	}
	if receipt.Result == nil {
		return nil, nil
	}
	output := receipt.Result.Output
	if receipt.Result.IsError {
		failure, err := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindFailed, Cause: errors.New("plugin: original Tool failed"), Output: output})
		if err != nil {
			return nil, err
		}
		return &output, failure
	}
	return &output, nil
}
