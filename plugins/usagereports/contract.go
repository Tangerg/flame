package usagereports

//go:generate go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --feature contracts/capability.json --out package/io.github.tangerg.flame/contracts --ts component

import (
	_ "embed"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
)

//go:embed contracts/capability.json
var descriptor []byte

type CaptureInput struct {
	Period string `json:"period"`
}
type ReportInput struct {
	ID string `json:"id"`
}
type Header struct {
	ID         string     `json:"id"`
	Period     string     `json:"period"`
	ObservedAt time.Time  `json:"observedAt"`
	Since      *time.Time `json:"since,omitzero"`
}
type Report[Summary any] struct {
	Header
	Summary Summary `json:"summary"`
}
type Contracts[Summary any] struct {
	Capability *capability.Contract
	Capture    *pluginsdk.Method[CaptureInput, Header]
	List       *pluginsdk.Method[struct{}, []Header]
	Get        *pluginsdk.Method[ReportInput, Report[Summary]]
	Delete     *pluginsdk.Method[ReportInput, bool]
}

// NewContracts projects the published descriptor into local consumer codecs.
// The descriptor retains its original opaque schema names across languages.
func NewContracts[Summary any]() (Contracts[Summary], error) {
	var c Contracts[Summary]
	var err error
	if c.Capability, err = capability.Parse(descriptor); err != nil {
		return c, err
	}
	if c.Capture, err = pluginsdk.ImplementMethod[CaptureInput, Header](c.Capability, "flame.usage-reports/capture.v1"); err != nil {
		return c, err
	}
	if c.List, err = pluginsdk.ImplementMethod[struct{}, []Header](c.Capability, "flame.usage-reports/list.v1"); err != nil {
		return c, err
	}
	if c.Get, err = pluginsdk.ImplementMethod[ReportInput, Report[Summary]](c.Capability, "flame.usage-reports/get.v1"); err != nil {
		return c, err
	}
	c.Delete, err = pluginsdk.ImplementMethod[ReportInput, bool](c.Capability, "flame.usage-reports/delete.v1")
	return c, err
}
