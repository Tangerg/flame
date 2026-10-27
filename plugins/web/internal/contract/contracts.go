package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/web/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"github.com/Tangerg/flame/plugins/web/contracts"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/flame/plugins/web/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/tools/web"
)

type Contracts struct {
	Capability *portable.Capability
	Search     *qimumcp.Method[web.SearchRequest, web.SearchResponse]
	Fetch      *qimumcp.Method[web.FetchRequest, capture.Page]
	Browse     *qimumcp.Method[wire.BrowseInput, capture.Page]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(contracts.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = qimumcp.ImplementMethod[web.SearchRequest, web.SearchResponse](c.Capability, contracts.SearchV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Fetch, err = qimumcp.ImplementMethod[web.FetchRequest, capture.Page](c.Capability, contracts.FetchV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Browse, err = qimumcp.ImplementMethod[wire.BrowseInput, capture.Page](c.Capability, contracts.BrowseV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
