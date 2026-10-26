package main

//go:generate go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature ../../plugins/usagereports/package/io.github.tangerg.flame/contracts/capability.generated.json --out "" --go-out usagereports_contract.generated.go --go-package main --go-prefix UsageReports
