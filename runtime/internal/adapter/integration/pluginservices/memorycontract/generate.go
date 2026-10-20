package memorycontract

//go:generate go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature ../../../../../../plugins/memory/io.github.tangerg.flame/contracts/capability.generated.json --out "" --go-out contract.generated.go --go-package memorycontract
//go:generate sh -c "go run github.com/Tangerg/flame/plugins/memory/backend/cmd/contracts --caller-json ../../../../../../plugins/memory/io.github.tangerg.flame/contracts/caller-policy.generated.json --caller-go caller_policy.generated.go > /dev/null"
