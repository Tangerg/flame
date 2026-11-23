package mcp

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/frame"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

func validateHostCall(ref CallbackRef, method *frame.Method, id CommandID) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if method.Kind() == frame.Command {
		return id.Validate()
	}
	if id != "" {
		return errQueryCommandIdentity
	}
	return nil
}

func newHostRequest(ctx context.Context, ref CallbackRef, method *frame.Method, id CommandID, admitted framevalue.JSON) (*http.Request, error) {
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
	request.Header.Set(headerAuthorization, ref.Credential)
	request.Header.Set(CallHeader, ref.Call)
	request.Header.Set(headerContentType, jsonMediaType)
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

// One pooled loopback client: a per-call transport would open a connection per
// streamed chunk. Redirects never follow.
var callbackClient = &http.Client{
	Transport:     &http.Transport{Proxy: nil},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func CallHost[P, R any](ctx context.Context, ref CallbackRef, method *Method[P, R], id CommandID, input P) (R, error) {
	var zero R
	if method == nil {
		return zero, errors.New("plugin: callback method is required")
	}
	if err := validateHostCall(ref, method.contract, id); err != nil {
		return zero, err
	}
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		return zero, err
	}
	request, err := newHostRequest(ctx, ref, method.contract, id, admitted)
	if err != nil {
		return zero, err
	}
	response, err := callbackClient.Do(request)
	if err != nil {
		if failure, ok := errors.AsType[*url.Error](err); ok {
			return zero, fmt.Errorf("plugin: callback transport: %w", errors.Join(failure.Err, ctx.Err(), context.Cause(ctx)))
		}
		return zero, err
	}
	defer response.Body.Close()
	body, err := readHostReply(response, ref.MaxResponseBytes)
	if err != nil {
		return zero, err
	}
	var result methodResult[jsontext.Value]
	if err := json.Unmarshal(body, &result, json.RejectUnknownMembers(true)); err != nil {
		return zero, err
	}
	if _, err := result.value(method.Name()); err != nil {
		return zero, err
	}
	if err := method.contract.ValidateOutput(result.valueJSON); err != nil {
		return zero, fmt.Errorf("plugin: callback output: %w", err)
	}
	typed, err := decodeBoundResult[R](&result)
	if err != nil {
		return zero, err
	}
	return typed.value(method.Name())
}

func setCallbackInvocation(request *http.Request, ctx context.Context) error {
	invocation, found := ctx.Value(invocationKey{}).(framecall.Invocation)
	if !found {
		return nil
	}
	if err := invocation.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		return err
	}
	request.Header.Set(InvocationHeader, string(encoded))
	return nil
}
