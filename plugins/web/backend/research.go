package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	capture "github.com/Tangerg/flame/plugins/web/backend/domain"
	"github.com/Tangerg/flame/plugins/web/backend/rpc"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/scope/tools/web"
)

const (
	maxQueryBytes          = 8192
	maxSearchResponseBytes = 128 << 10
)

type fetchPage func(context.Context, *web.FetchRequest) (*web.FetchResponse, error)
type searchWeb func(context.Context, *web.SearchRequest) (*web.SearchResponse, error)

type research struct {
	store  *store
	fetch  fetchPage
	search searchWeb
}

func newResearch(store *store, fetch fetchPage, search searchWeb) (*research, error) {
	if store == nil || store.db == nil || fetch == nil || search == nil {
		return nil, errors.New("web: archive, fetch and search boundaries are required")
	}
	return &research{store: store, fetch: fetch, search: search}, nil
}

func (r *research) searchWeb(ctx context.Context, input web.SearchRequest) (web.SearchResponse, error) {
	if len(input.Query) > maxQueryBytes {
		return web.SearchResponse{}, &capability.Refusal{Code: "invalid_request", Message: fmt.Sprintf("web: search query exceeds %d bytes", maxQueryBytes)}
	}
	prepared, err := input.Prepare()
	if err != nil {
		return web.SearchResponse{}, errors.Join(&capability.Refusal{Code: "invalid_request", Message: "web: invalid search request"}, err)
	}
	response, err := r.search(ctx, prepared)
	if err != nil {
		return web.SearchResponse{}, err
	}
	if err := response.ValidateFor(prepared); err != nil {
		return web.SearchResponse{}, err
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return web.SearchResponse{}, err
	}
	if len(encoded) > maxSearchResponseBytes {
		return web.SearchResponse{}, &capability.Refusal{Code: "response_too_large", Message: fmt.Sprintf("web: search result exceeds %d KiB", maxSearchResponseBytes>>10)}
	}
	return *response, nil
}

func (r *research) fetchPage(ctx context.Context, input web.FetchRequest) (capture.Page, error) {
	prepared, err := input.Prepare()
	if err != nil {
		return capture.Page{}, errors.Join(&capability.Refusal{Code: "invalid_request", Message: "web: invalid fetch request"}, err)
	}
	if err := capture.ValidateURL(prepared.URL); err != nil {
		return capture.Page{}, err
	}
	response, err := r.fetch(ctx, prepared)
	if err != nil {
		return capture.Page{}, err
	}
	if err := response.Validate(); err != nil {
		return capture.Page{}, err
	}
	if response.Format != prepared.Format {
		return capture.Page{}, errors.New("web: provider replaced the authored content format")
	}
	document, err := capture.NewDocument(capture.Content{URL: prepared.URL, Format: capture.Format(response.Format), Text: response.Content})
	if err != nil {
		return capture.Page{}, err
	}
	if err := r.store.capture(ctx, document); err != nil {
		return capture.Page{}, err
	}
	return document.Page(0, capture.MaxPageRunes)
}

func (r *research) browse(ctx context.Context, input rpc.BrowseInput) (capture.Page, error) {
	document, err := r.store.read(ctx, input.SnapshotID)
	if err != nil {
		return capture.Page{}, err
	}
	return document.Page(input.Offset, input.Limit)
}
