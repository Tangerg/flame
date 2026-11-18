package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
)

const ToolReceiptMethod = "io.github.tangerg.flame/tools.getResult"

// ToolReceipt projects a private receipt through the original Tool's output
// codec. The business owner retains its only durable result representation.
type ToolReceipt struct {
	Result *ToolResult `json:"result,omitzero"`
}

type ToolResult struct {
	Output  tool.Output `json:"output"`
	IsError bool        `json:"isError,omitzero"`
}

func ToolReceiptQuery() (*pluginmcp.Method[pluginmcp.ReceiptRequest, ToolReceipt], error) {
	return pluginmcp.NewMethod[pluginmcp.ReceiptRequest, ToolReceipt](frame.MethodConfig{Name: ToolReceiptMethod, Kind: frame.Query, Target: extension.ScopeInstallation, Description: ToolReceiptMethod})
}

func NewToolReceipt(output tool.Output, failed bool) (ToolReceipt, error) {
	if err := output.Validate(); err != nil {
		return ToolReceipt{}, err
	}
	return ToolReceipt{Result: &ToolResult{Output: output.Clone(), IsError: failed}}, nil
}

// ToolReceiptFrom projects one decoded stored reply. A reply failure that
// classify turns into a Tool failure becomes a failed receipt; any other
// failure stays an error, and a result becomes its JSON output.
func ToolReceiptFrom[R any](result R, err error, classify func(error) error) (ToolReceipt, error) {
	if err != nil {
		failure, known := tool.ClassifyFailure(classify(err))
		if !known {
			return ToolReceipt{}, err
		}
		return NewToolReceipt(failure.Output(), true)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return ToolReceipt{}, err
	}
	output, err := tool.NewJSONOutput(encoded)
	if err != nil {
		return ToolReceipt{}, err
	}
	return NewToolReceipt(output, false)
}

// ReadToolReceipt binds the original remote Tool, logical call and admitted JSON.
// A missing or unsupported read never permits another Tool invocation.
func ReadToolReceipt(ctx context.Context, session *sdk.ClientSession, callID, remote string, admitted framevalue.JSON) (*tool.Output, error) {
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
	request := pluginmcp.ReceiptRequest{ID: pluginmcp.CommandID(id), Method: remote, InputDigest: admitted.Digest()}
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
