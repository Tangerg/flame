package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/knowledgebase/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract context > ../../package/io.github.tangerg.flame/contracts/context.generated.json"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	published "github.com/Tangerg/flame/plugins/knowledgebase/contracts"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"
)

type Contracts struct {
	Capability *plugincontract.Capability
	Import     *pluginmcp.Method[wire.ImportInput, knowledge.Header]
	Reindex    *pluginmcp.Method[wire.DocumentInput, knowledge.Header]
	Delete     *pluginmcp.Method[wire.DocumentInput, bool]
	List       *pluginmcp.Method[struct{}, []knowledge.Header]
	Read       *pluginmcp.Method[wire.ReadInput, wire.DocumentRead]
	Search     *pluginmcp.Method[wire.SearchInput, []knowledge.Match]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Import, err = pluginmcp.ImplementMethod[wire.ImportInput, knowledge.Header](c.Capability, published.ImportV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Reindex, err = pluginmcp.ImplementMethod[wire.DocumentInput, knowledge.Header](c.Capability, published.ReindexV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = pluginmcp.ImplementMethod[wire.DocumentInput, bool](c.Capability, published.DeleteV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = pluginmcp.ImplementMethod[struct{}, []knowledge.Header](c.Capability, published.ListV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = pluginmcp.ImplementMethod[wire.SearchInput, []knowledge.Match](c.Capability, published.SearchV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Read, err = pluginmcp.ImplementMethod[wire.ReadInput, wire.DocumentRead](c.Capability, published.ReadV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
