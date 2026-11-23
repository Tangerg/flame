package reviews

//go:generate sh -c "go run ../cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../component"
//go:generate sh -c "go run ../cmd/contracts --projection analyzers > ../../package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --projection board-ts > ../../component/board.generated.ts"
//go:generate sh -c "go run ../cmd/contracts --projection analyzer-capability | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature - --out ../../analyzer/package/io.github.tangerg.flame/contracts --ts ../../analyzer"
//go:generate sh -c "go run ../cmd/contracts --projection analyzers > ../../analyzer/package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --projection analyzer-capability > ../../package/io.github.tangerg.flame/contracts/analyzer-capability.generated.json"

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/frame"
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
	list, err := pluginmcp.NewMethod[struct{}, ListResult](frame.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.list", Kind: frame.Query, Target: extension.ScopeInstallation,
		Description: "Read the review backend's current review and revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	update, err := pluginmcp.NewMethod[Update, UpdateResult](frame.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.update", Kind: frame.Command, Target: extension.ScopeInstallation,
		Description: "Change one review's status using its inspected revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	analyzers, err := pluginmcp.NewMethod[struct{}, Analysis](frame.MethodConfig{Name: "io.github.tangerg.flame.examples/reviews.analyzers", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Analyze the current review through its installed analyzer collection and return admitted findings."})
	if err != nil {
		return Contracts{}, err
	}
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "io.github.tangerg.flame.examples/reviews", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, list.Descriptor(), update.Descriptor(), analyzers.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return Contracts{List: list, Update: update, Analyzers: analyzers, Capability: contract}, nil
}
