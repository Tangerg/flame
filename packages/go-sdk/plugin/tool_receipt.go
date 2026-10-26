package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

const ToolReceiptMethod = "io.github.tangerg.flame/tools.getResult"

// ToolReceipt projects a private receipt through the original Tool's output
// codec. The business owner retains its only durable result representation.
type ToolReceipt struct {
	Found   bool   `json:"found"`
	Output  string `json:"output,omitzero"`
	IsError bool   `json:"isError,omitzero"`
}

func ToolReceiptQuery() (*Method[ReceiptRequest, ToolReceipt], error) {
	return NewMethod[ReceiptRequest, ToolReceipt](MethodConfig{Name: ToolReceiptMethod, Kind: extension.Query, Target: "installation", Description: ToolReceiptMethod})
}

func NewToolReceipt(output chat.ToolOutput, failed bool) (ToolReceipt, error) {
	encoded, err := json.Marshal(output)
	if err != nil {
		return ToolReceipt{}, err
	}
	return ToolReceipt{Found: true, Output: string(encoded), IsError: failed}, nil
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
	request := ReceiptRequest{ID: id, Method: remote, InputDigest: admitted.Digest()}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	method, err := ToolReceiptQuery()
	if err != nil {
		return nil, err
	}
	input, err := jsonvalue.Encode(request)
	if err != nil {
		return nil, err
	}
	binding := &Binding{session: session, method: method.contract}
	wire, err := binding.invoke(ctx, "", input, sdk.ParamsBase{})
	if err != nil {
		return nil, err
	}
	result, err := decodeBoundResult[ToolReceipt](wire)
	if err != nil {
		return nil, err
	}
	receipt, err := result.value(ToolReceiptMethod)
	if err != nil {
		return nil, err
	}
	if !receipt.Found {
		if receipt.Output != "" || receipt.IsError {
			return nil, errors.New("plugin: absent Tool receipt carries outcome evidence")
		}
		return nil, nil
	}
	if strings.TrimSpace(receipt.Output) == "null" {
		return nil, errors.New("plugin: Tool receipt requires an output object")
	}
	var output chat.ToolOutput
	if err := json.Unmarshal([]byte(receipt.Output), &output, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if receipt.IsError {
		failure, err := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindFailed, Cause: errors.New("plugin: original Tool failed"), Output: output})
		if err != nil {
			return nil, err
		}
		return &output, failure
	}
	return &output, nil
}
