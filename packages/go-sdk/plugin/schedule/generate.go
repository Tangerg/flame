// Package schedule publishes the Schedules plugin's callable contract for Go
// consumers. It is generated from the plugin's published descriptor; clients
// read it here instead of each generating a copy.
package schedule

//go:generate go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature ../../../../plugins/schedules/package/io.github.tangerg.flame/contracts/capability.generated.json --out "" --go-out contract.generated.go --go-package schedule
