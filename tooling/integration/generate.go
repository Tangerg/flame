package integration

//go:generate go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature ../../plugins/schedules/package/io.github.tangerg.flame/contracts/capability.generated.json --out "" --go-out schedules_contract.generated.go --go-package integration --go-prefix Schedules
//go:generate go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature ../../plugins/guardrails/package/io.github.tangerg.flame/contracts/capability.generated.json --out "" --go-out guardrails_contract.generated.go --go-package integration --go-prefix Guardrails
//go:generate go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature ../../plugins/usagereports/package/io.github.tangerg.flame/contracts/capability.generated.json --out "" --go-out usagereports_contract.generated.go --go-package integration --go-prefix UsageReports
