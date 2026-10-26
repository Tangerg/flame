package reviews

//go:generate sh -c "go run ../cmd/contracts | go tool github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../component"
//go:generate sh -c "go run ../cmd/contracts --analyzers > ../../package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --analyzers-ts > ../../component/analyzers.generated.ts"
//go:generate sh -c "go run ../cmd/contracts --analyzer-capability | go tool github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../analyzer/package/io.github.tangerg.flame/contracts --ts ../../analyzer"
//go:generate sh -c "go run ../cmd/contracts --analyzers > ../../analyzer/package/io.github.tangerg.flame/contracts/analyzers.generated.json"
//go:generate sh -c "go run ../cmd/contracts --analyzer-capability > ../../package/io.github.tangerg.flame/contracts/analyzer-capability.generated.json"

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
)

type Contracts struct {
	Capability *extension.Capability
	List       *pluginsdk.Method[struct{}, ListResult]
	Update     *pluginsdk.Method[Update, UpdateResult]
	Analyzers  *pluginsdk.Method[struct{}, Analysis]
}

type ListResult struct {
	Reviews []Snapshot `json:"reviews"`
}

func NewContracts() (Contracts, error) {
	list, err := pluginsdk.NewMethod[struct{}, ListResult](pluginsdk.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.list", Kind: extension.Query, Target: "installation",
		Description: "Read the review backend's current review and revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	update, err := pluginsdk.NewMethod[Update, UpdateResult](pluginsdk.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.update", Kind: extension.Mutation, Target: "installation",
		Description: "Change one review's status using its inspected revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	analyzers, err := pluginsdk.NewMethod[struct{}, Analysis](pluginsdk.MethodConfig{Name: "io.github.tangerg.flame.examples/reviews.analyzers", Kind: extension.Query, Target: "installation", Description: "Analyze the current review through its installed analyzer collection and return admitted findings."})
	if err != nil {
		return Contracts{}, err
	}
	contract, err := extension.NewCapability(extension.CapabilityConfig{Reference: "io.github.tangerg.flame.examples/reviews", Composition: extension.Collection, Scope: "installation"}, list.Descriptor(), update.Descriptor(), analyzers.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return Contracts{List: list, Update: update, Analyzers: analyzers, Capability: contract}, nil
}
