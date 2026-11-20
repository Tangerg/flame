package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/web/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/web/contracts"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/flame/plugins/web/internal/wire"

	"github.com/Tangerg/scope/tools/httpreq"
	"github.com/Tangerg/scope/tools/web"
)

type Contracts struct {
	Capability *plugincontract.Capability
	Search     *pluginmcp.Method[web.SearchRequest, wire.SearchResponse]
	Fetch      *pluginmcp.Method[web.FetchRequest, capture.Page]
	Browse     *pluginmcp.Method[wire.BrowseInput, capture.Page]
	Request    *pluginmcp.Method[httpreq.Request, wire.HTTPResponse]
}

func NewContracts() (Contracts, error) {
	capability, err := plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		Search:     pluginmcp.Implement[web.SearchRequest, wire.SearchResponse](methods, contracts.SearchV1Method),
		Fetch:      pluginmcp.Implement[web.FetchRequest, capture.Page](methods, contracts.FetchV1Method),
		Browse:     pluginmcp.Implement[wire.BrowseInput, capture.Page](methods, contracts.BrowseV1Method),
		Request:    pluginmcp.Implement[httpreq.Request, wire.HTTPResponse](methods, contracts.RequestV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
