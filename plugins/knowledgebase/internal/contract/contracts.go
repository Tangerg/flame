package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/knowledgebase/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract context > ../../package/io.github.tangerg.flame/contracts/context.generated.json"

import (
	published "github.com/Tangerg/flame/plugins/knowledgebase/contracts"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	Import     *qimumcp.Method[wire.ImportInput, knowledge.Header]
	Reindex    *qimumcp.Method[wire.DocumentInput, knowledge.Header]
	Delete     *qimumcp.Method[wire.DocumentInput, bool]
	List       *qimumcp.Method[struct{}, []knowledge.Header]
	Read       *qimumcp.Method[wire.ReadInput, wire.DocumentRead]
	Search     *qimumcp.Method[wire.SearchInput, []knowledge.Match]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Capability, err = portable.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	c.Import, err = qimumcp.ImplementMethod[wire.ImportInput, knowledge.Header](c.Capability, published.ImportV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Reindex, err = qimumcp.ImplementMethod[wire.DocumentInput, knowledge.Header](c.Capability, published.ReindexV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = qimumcp.ImplementMethod[wire.DocumentInput, bool](c.Capability, published.DeleteV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = qimumcp.ImplementMethod[struct{}, []knowledge.Header](c.Capability, published.ListV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = qimumcp.ImplementMethod[wire.SearchInput, []knowledge.Match](c.Capability, published.SearchV1Method)
	if err != nil {
		return Contracts{}, err
	}
	c.Read, err = qimumcp.ImplementMethod[wire.ReadInput, wire.DocumentRead](c.Capability, published.ReadV1Method)
	if err != nil {
		return Contracts{}, err
	}
	return c, nil
}
