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
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = pluginmcp.ImplementMethod[web.SearchRequest, wire.SearchResponse](c.Capability, contracts.SearchV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Fetch, err = pluginmcp.ImplementMethod[web.FetchRequest, capture.Page](c.Capability, contracts.FetchV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Browse, err = pluginmcp.ImplementMethod[wire.BrowseInput, capture.Page](c.Capability, contracts.BrowseV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Request, err = pluginmcp.ImplementMethod[httpreq.Request, wire.HTTPResponse](c.Capability, contracts.RequestV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
