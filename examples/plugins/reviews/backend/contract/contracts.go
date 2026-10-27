package reviews

//go:generate sh -c "go run ../cmd/contracts | go tool github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../component"
//go:generate sh -c "go run ../cmd/contracts --analyzers > ../../package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --analyzers-ts > ../../component/analyzers.generated.ts"
//go:generate sh -c "go run ../cmd/contracts --analyzer-capability | go tool github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../analyzer/package/io.github.tangerg.flame/contracts --ts ../../analyzer"
//go:generate sh -c "go run ../cmd/contracts --analyzers > ../../analyzer/package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --analyzer-capability > ../../package/io.github.tangerg.flame/contracts/analyzer-capability.generated.json"

import (
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type Contracts struct {
	Capability *portable.Capability
	List       *qimumcp.Method[struct{}, ListResult]
	Update     *qimumcp.Method[Update, UpdateResult]
	Analyzers  *qimumcp.Method[struct{}, Analysis]
}

type ListResult struct {
	Reviews []Snapshot `json:"reviews"`
}

func NewContracts() (Contracts, error) {
	list, err := qimumcp.NewMethod[struct{}, ListResult](qimumcp.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.list", Kind: portable.Query, Target: "installation",
		Description: "Read the review backend's current review and revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	update, err := qimumcp.NewMethod[Update, UpdateResult](qimumcp.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.update", Kind: portable.Mutation, Target: "installation",
		Description: "Change one review's status using its inspected revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	analyzers, err := qimumcp.NewMethod[struct{}, Analysis](qimumcp.MethodConfig{Name: "io.github.tangerg.flame.examples/reviews.analyzers", Kind: portable.Query, Target: "installation", Description: "Analyze the current review through its installed analyzer collection and return admitted findings."})
	if err != nil {
		return Contracts{}, err
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "io.github.tangerg.flame.examples/reviews", Composition: portable.Collection, Scope: "installation"}, list.Descriptor(), update.Descriptor(), analyzers.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return Contracts{List: list, Update: update, Analyzers: analyzers, Capability: contract}, nil
}
