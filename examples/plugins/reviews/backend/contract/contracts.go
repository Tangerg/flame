package reviews

//go:generate sh -c "go run ../cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../component"
//go:generate sh -c "go run ../cmd/contracts --analyzers > ../../package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --board-ts > ../../component/board.generated.ts"
//go:generate sh -c "go run ../cmd/contracts --analyzer-capability | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../analyzer/package/io.github.tangerg.flame/contracts --ts ../../analyzer"
//go:generate sh -c "go run ../cmd/contracts --analyzers > ../../analyzer/package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --analyzer-capability > ../../package/io.github.tangerg.flame/contracts/analyzer-capability.generated.json"

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/portable"
)

type Contracts struct {
	Capability *plugincontract.Capability
	List       *pluginmcp.Method[struct{}, ListResult]
	Update     *pluginmcp.Method[Update, UpdateResult]
	Analyzers  *pluginmcp.Method[struct{}, Analysis]
}

type ListResult struct {
	Reviews []Snapshot `json:"reviews"`
}

func NewContracts() (Contracts, error) {
	list, err := pluginmcp.NewMethod[struct{}, ListResult](portable.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.list", Kind: portable.Query, Target: "installation",
		Description: "Read the review backend's current review and revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	update, err := pluginmcp.NewMethod[Update, UpdateResult](portable.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.update", Kind: portable.Command, Target: "installation",
		Description: "Change one review's status using its inspected revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	analyzers, err := pluginmcp.NewMethod[struct{}, Analysis](portable.MethodConfig{Name: "io.github.tangerg.flame.examples/reviews.analyzers", Kind: portable.Query, Target: "installation", Description: "Analyze the current review through its installed analyzer collection and return admitted findings."})
	if err != nil {
		return Contracts{}, err
	}
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "io.github.tangerg.flame.examples/reviews", Composition: plugincontract.Collection, Scope: "installation"}, list.Descriptor(), update.Descriptor(), analyzers.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return Contracts{List: list, Update: update, Analyzers: analyzers, Capability: contract}, nil
}
