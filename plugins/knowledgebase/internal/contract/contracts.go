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
	capability, err := plugincontract.ParseCapability([]byte(published.CapabilityJSON))
	if err != nil {
		return Contracts{}, err
	}
	methods := pluginmcp.NewMethodSet(capability)
	c := Contracts{
		Capability: capability,
		Import:     pluginmcp.Implement[wire.ImportInput, knowledge.Header](methods, published.ImportV1Method),
		Reindex:    pluginmcp.Implement[wire.DocumentInput, knowledge.Header](methods, published.ReindexV1Method),
		Delete:     pluginmcp.Implement[wire.DocumentInput, bool](methods, published.DeleteV1Method),
		List:       pluginmcp.Implement[struct{}, []knowledge.Header](methods, published.ListV1Method),
		Search:     pluginmcp.Implement[wire.SearchInput, []knowledge.Match](methods, published.SearchV1Method),
		Read:       pluginmcp.Implement[wire.ReadInput, wire.DocumentRead](methods, published.ReadV1Method),
	}
	if err := methods.Err(); err != nil {
		return Contracts{}, err
	}
	return c, nil
}
