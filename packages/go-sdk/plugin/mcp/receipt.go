package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const ReceiptMethod = "io.github.tangerg.flame/commands.get"

type ReceiptRequest struct {
	ID          CommandID         `json:"id"`
	Method      string            `json:"method"`
	InputDigest framevalue.Digest `json:"inputDigest"`
}

type receiptRequestWire ReceiptRequest

func (r ReceiptRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(receiptRequestWire(r))
}

func (ReceiptRequest) JSONSchemaModel() any { return receiptRequestWire{} }

func (r *ReceiptRequest) UnmarshalJSON(data []byte) error {
	var decoded receiptRequestWire
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	request := ReceiptRequest(decoded)
	if err := request.Validate(); err != nil {
		return err
	}
	*r = request
	return nil
}

func (r ReceiptRequest) Validate() error {
	if err := r.ID.Validate(); err != nil {
		return err
	}
	if !frame.ValidName(r.Method) {
		return errors.New("plugin: bounded receipt method identity is required")
	}
	if r.InputDigest.IsZero() {
		return errors.New("plugin: canonical receipt input digest is required")
	}
	return nil
}

type Receipt struct {
	Found bool           `json:"found"`
	Reply jsontext.Value `json:"reply,omitzero"`
}

func EncodeReply[T any](value T, err error, classify Classifier) (framevalue.JSON, error) {
	outcome, err := reply(value, err, classify)
	if err != nil {
		return framevalue.JSON{}, err
	}
	return framevalue.EncodeJSON(outcome)
}

func ReceiptQuery() (*Method[ReceiptRequest, Receipt], error) {
	return NewMethod[ReceiptRequest, Receipt](frame.MethodConfig{Name: ReceiptMethod, Kind: frame.Query, Target: extension.ScopeInstallation, Description: ReceiptMethod})
}

func DecodeReply[T any](data []byte) (T, error) {
	var result methodResult[T]
	var zero T
	if err := json.Unmarshal(data, &result, json.RejectUnknownMembers(true)); err != nil {
		return zero, err
	}
	return result.value("plugin receipt")
}

func receiptBinding(session *sdk.ClientSession) (*Binding, error) {
	method, err := ReceiptQuery()
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errClientSessionRequired
	}
	return &Binding{session: session, method: method.contract}, nil
}

func readReceipt(ctx context.Context, binding *Binding, id CommandID, method string, admitted framevalue.JSON) (jsontext.Value, bool, error) {
	request := ReceiptRequest{ID: id, Method: method, InputDigest: admitted.Digest()}
	if err := request.Validate(); err != nil {
		return nil, false, err
	}
	input, err := framevalue.EncodeJSON(request)
	if err != nil {
		return nil, false, err
	}
	wire, err := binding.invoke(ctx, "", input, sdk.ParamsBase{})
	if err != nil {
		return nil, false, err
	}
	result, err := decodeBoundResult[Receipt](wire)
	if err != nil {
		return nil, false, err
	}
	receipt, err := result.value(ReceiptMethod)
	if err != nil {
		return nil, false, err
	}
	if receipt.Found != (len(receipt.Reply) != 0) {
		return nil, false, errors.New("plugin: malformed receipt evidence")
	}
	if !receipt.Found {
		return nil, false, nil
	}
	value, err := DecodeReply[jsontext.Value](receipt.Reply)
	return value, true, err
}
