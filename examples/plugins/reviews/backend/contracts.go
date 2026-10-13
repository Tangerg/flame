package reviews

//go:generate sh -c "go run ./cmd/contracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../package/io.github.tangerg.flame/contracts --ts ../client"

import (
	"github.com/Tangerg/flame/runtime/capability"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type Contracts struct {
	Capability *capability.Contract
	List       *pluginsdk.Method[struct{}, ListResult]
	Update     *pluginsdk.Method[Update, UpdateResult]
}

type ListResult struct {
	Reviews []Snapshot `json:"reviews"`
}

func NewContracts() (Contracts, error) {
	list, err := pluginsdk.NewMethod[struct{}, ListResult](pluginsdk.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.list", Kind: capability.Query, Target: "installation",
		Description: "Read the review backend's current review and revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	update, err := pluginsdk.NewMethod[Update, UpdateResult](pluginsdk.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.update", Kind: capability.Mutation, Target: "installation",
		Description: "Change one review's status using its inspected revision.",
	})
	if err != nil {
		return Contracts{}, err
	}
	contract, err := capability.New(capability.Config{Reference: "io.github.tangerg.flame.examples/reviews", Composition: capability.Collection, Scope: "installation"}, list.Descriptor(), update.Descriptor())
	if err != nil {
		return Contracts{}, err
	}
	return Contracts{List: list, Update: update, Capability: contract}, nil
}
