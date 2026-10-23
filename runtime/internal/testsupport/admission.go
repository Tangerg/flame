package testsupport

import "github.com/Tangerg/flame/runtime/internal/application/ownership"

// NewAdmissionGate treats fixture workspace names as already resolved identities.
func NewAdmissionGate() *ownership.Gate {
	gate, err := ownership.NewGate(func(path string) (string, error) { return path, nil })
	if err != nil {
		panic(err)
	}
	return gate
}
