// Package memory publishes the exact contract consumed by Runtime and implemented
// by independently built Memory providers. Generated schemas are opaque identities;
// an implementation's Go package path cannot redefine them.
package memory

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/contractgen | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out \"\" --go-out contract.generated.go --go-package memory"
//go:generate go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts --caller-go caller_policy.generated.go
