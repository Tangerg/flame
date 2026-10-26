package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/web/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	json "encoding/json"
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/scope/tools/web"
)

type BrowseInput struct {
	SnapshotID string `json:"snapshotId" jsonschema:"minLength=64,maxLength=64"`
	Offset     int64  `json:"offset" jsonschema:"minimum=0"`
	Limit      int64  `json:"limit" jsonschema:"minimum=1"`
}

type Contracts struct {
	Capability *extension.Capability
	Search     *pluginsdk.Method[web.SearchRequest, web.SearchResponse]
	Fetch      *pluginsdk.Method[web.FetchRequest, capture.Page]
	Browse     *pluginsdk.Method[BrowseInput, capture.Page]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Search, err = pluginsdk.NewMethod[web.SearchRequest, web.SearchResponse](pluginsdk.MethodConfig{Name: "flame.web/search.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Search through configured Tavily credentials with a query up to 8192 bytes, honoring explicit domain and supported recency filters. Each call queries the provider; results are bounded to 128 KiB."})
	if err != nil {
		return Contracts{}, err
	}
	c.Fetch, err = pluginsdk.NewMethod[web.FetchRequest, capture.Page](pluginsdk.MethodConfig{Name: "flame.web/fetch.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Fetch a URL through configured Jina Reader credentials and capture a private immutable snapshot. Return its first 8192 Unicode codepoints and exact continuation offset."})
	if err != nil {
		return Contracts{}, err
	}
	c.Browse, err = pluginsdk.NewMethod[BrowseInput, capture.Page](pluginsdk.MethodConfig{Name: "flame.web/browse.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Read a bounded page of a previously captured snapshot by ID and Unicode codepoint offset. This reads captured material without network I/O; an evicted snapshot is refused."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: "flame.web.v1", Composition: extension.Collection, Scope: extension.ScopeInstallation}, c.Search.Descriptor(), c.Fetch.Descriptor(), c.Browse.Descriptor())
	return c, err
}

func (BrowseInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	limit, _ := schema.Properties.Get("limit")
	limit.Maximum = json.Number(strconv.Itoa(capture.MaxPageRunes))
}
