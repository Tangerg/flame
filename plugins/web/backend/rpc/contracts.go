package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/web/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	capture "github.com/Tangerg/flame/plugins/web/backend/domain"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/scope/tools/web"
)

type BrowseInput struct {
	SnapshotID string `json:"snapshotId" jsonschema:"minLength=64,maxLength=64"`
	Offset     int64  `json:"offset" jsonschema:"minimum=0"`
	Limit      int64  `json:"limit" jsonschema:"minimum=1,maximum=8192"`
}

type Contracts struct {
	Capability *capability.Contract
	Search     *pluginsdk.Method[web.SearchRequest, web.SearchResponse]
	Fetch      *pluginsdk.Method[web.FetchRequest, capture.Page]
	Browse     *pluginsdk.Method[BrowseInput, capture.Page]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Search, err = pluginsdk.NewMethod[web.SearchRequest, web.SearchResponse](pluginsdk.MethodConfig{Name: "flame.web/search.v1", Kind: capability.Query, Target: capability.ScopeInstallation, Description: "Search through configured Tavily credentials with a query up to 8192 bytes, honoring explicit domain and supported recency filters. Each call queries the provider; results are bounded to 128 KiB."})
	if err != nil {
		return Contracts{}, err
	}
	c.Fetch, err = pluginsdk.NewMethod[web.FetchRequest, capture.Page](pluginsdk.MethodConfig{Name: "flame.web/fetch.v1", Kind: capability.Query, Target: capability.ScopeInstallation, Description: "Fetch a URL through configured Jina Reader credentials and capture a private immutable snapshot. Return its first 8192 Unicode codepoints and exact continuation offset."})
	if err != nil {
		return Contracts{}, err
	}
	c.Browse, err = pluginsdk.NewMethod[BrowseInput, capture.Page](pluginsdk.MethodConfig{Name: "flame.web/browse.v1", Kind: capability.Query, Target: capability.ScopeInstallation, Description: "Read a bounded page of a previously captured snapshot by ID and Unicode codepoint offset. This reads captured material without network I/O; an evicted snapshot is refused."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.web.v1", Composition: capability.Collection, Scope: capability.ScopeInstallation}, c.Search.Descriptor(), c.Fetch.Descriptor(), c.Browse.Descriptor())
	return c, err
}

func Classify(err error) string {
	switch {
	case errors.Is(err, capture.ErrDocument):
		return "invalid_document"
	case errors.Is(err, capture.ErrRange):
		return "invalid_page_range"
	case errors.Is(err, capture.ErrNotFound):
		return "snapshot_not_found"
	case errors.Is(err, web.ErrUnsupportedFilter), errors.Is(err, web.ErrUnsupportedFormat):
		return "unsupported_request"
	default:
		return ""
	}
}
