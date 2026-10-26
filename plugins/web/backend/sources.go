package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	capture "github.com/Tangerg/flame/plugins/web/backend/domain"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/httpresponse"
	"github.com/Tangerg/scope/tools/web"
	"github.com/Tangerg/scope/tools/web/jina"
	"github.com/Tangerg/scope/tools/web/tavily"
)

const maxProviderResponseBytes int64 = 8 << 20

var errProviderResponseTooLarge = errors.New("web: provider response exceeds eight MiB")

func newProviderHTTPClient() *http.Client {
	client := httpresponse.NewClient(maxProviderResponseBytes, errProviderResponseTooLarge)
	client.Timeout = 30 * time.Second
	return client
}

type sourceFailure struct {
	operation string
	cause     error
}

func (e *sourceFailure) Error() string { return "web: " + e.operation + " provider request failed" }
func (e *sourceFailure) Unwrap() error { return e.cause }

func jinaSource(ctx context.Context, key capture.APIKey, httpClient *http.Client) (fetchPage, error) {
	if !key.Configured() {
		return func(context.Context, *web.FetchRequest) (*web.FetchResponse, error) {
			return nil, &capability.Refusal{Code: "fetch_not_configured", Message: "web: Jina Reader credentials are not configured"}
		}, nil
	}
	client, err := jina.NewClient(ctx, jina.Config{APIKey: key.Value(), HTTPClient: httpClient})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, input *web.FetchRequest) (*web.FetchResponse, error) {
		response, err := client.Fetch(ctx, input)
		if err != nil {
			return nil, &sourceFailure{operation: "fetch", cause: err}
		}
		return response, nil
	}, nil
}

func tavilySource(ctx context.Context, key capture.APIKey, httpClient *http.Client) (searchWeb, error) {
	if !key.Configured() {
		return func(context.Context, *web.SearchRequest) (*web.SearchResponse, error) {
			return nil, &capability.Refusal{Code: "search_not_configured", Message: "web: Tavily credentials are not configured"}
		}, nil
	}
	client, err := tavily.NewClient(ctx, tavily.Config{APIKey: key.Value(), HTTPClient: httpClient})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, input *web.SearchRequest) (*web.SearchResponse, error) {
		response, err := client.Search(ctx, input)
		if err != nil {
			return nil, &sourceFailure{operation: "search", cause: err}
		}
		return response, nil
	}, nil
}
