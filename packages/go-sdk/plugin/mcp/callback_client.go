package mcp

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"
)

func validateHostCall(ref CallbackRef, method *portable.Method, id CommandID) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if method.Kind() == portable.Command {
		return id.Validate()
	}
	if id != "" {
		return errors.New("plugin: query must not carry a command identity")
	}
	return nil
}

// A callback's captured reference supplies its endpoint and authority. Business
// input supplies neither, and is checked against the original method before I/O.
func newHostRequest(ctx context.Context, ref CallbackRef, method *portable.Method, id CommandID, admitted framevalue.JSON) (*http.Request, error) {
	if err := method.ValidateInput(admitted); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(invokeParams{Protocol: plugincontract.Protocol, Method: method.Name(), Contract: method.Identity().Digest(), Input: admitted.Bytes(), CommandID: id})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ref.Endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", ref.Credential)
	request.Header.Set(CallHeader, ref.Call)
	request.Header.Set("Content-Type", "application/json")
	if err := setCallbackInvocation(request, ctx); err != nil {
		return nil, err
	}
	return request, nil
}

func readHostReply(response *http.Response, maximum int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, errors.New("plugin: callback response capacity exceeded")
	}
	switch response.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusGone:
		return nil, ErrStaleCall
	case http.StatusTooManyRequests:
		return nil, ErrCallbackCapacity
	default:
		return nil, fmt.Errorf("plugin: callback could not be confirmed (%d)", response.StatusCode)
	}
}
