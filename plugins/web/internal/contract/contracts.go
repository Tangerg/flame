package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/web/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	json "encoding/json"
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/tools/web"
)

type BrowseInput struct {
	SnapshotID string `json:"snapshotId" jsonschema:"minLength=64,maxLength=64"`
	Offset     int64  `json:"offset" jsonschema:"minimum=0"`
	Limit      int64  `json:"limit" jsonschema:"minimum=1"`
}

type Contracts struct {
	Capability *portable.Capability
	Search     *qimumcp.Method[web.SearchRequest, web.SearchResponse]
	Fetch      *qimumcp.Method[web.FetchRequest, capture.Page]
	Browse     *qimumcp.Method[BrowseInput, capture.Page]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Search, err = qimumcp.NewMethod[web.SearchRequest, web.SearchResponse](qimumcp.MethodConfig{Name: "flame.web/search.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Search through configured Tavily credentials with a query up to 8192 bytes, honoring explicit domain and supported recency filters. Each call queries the provider; results are bounded to 128 KiB."})
	if err != nil {
		return Contracts{}, err
	}
	c.Fetch, err = qimumcp.NewMethod[web.FetchRequest, capture.Page](qimumcp.MethodConfig{Name: "flame.web/fetch.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Fetch a URL through configured Jina Reader credentials and capture a private immutable snapshot. Return its first 8192 Unicode codepoints and exact continuation offset."})
	if err != nil {
		return Contracts{}, err
	}
	c.Browse, err = qimumcp.NewMethod[BrowseInput, capture.Page](qimumcp.MethodConfig{Name: "flame.web/browse.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read a bounded page of a previously captured snapshot by ID and Unicode codepoint offset. This reads captured material without network I/O; an evicted snapshot is refused."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = portable.NewCapability(portable.CapabilityConfig{Reference: "flame.web.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, c.Search.Descriptor(), c.Fetch.Descriptor(), c.Browse.Descriptor())
	return c, err
}

func (BrowseInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	limit, _ := schema.Properties.Get("limit")
	limit.Maximum = json.Number(strconv.Itoa(capture.MaxPageRunes))
}
