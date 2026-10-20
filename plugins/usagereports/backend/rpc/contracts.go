package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/usagereports/backend/cmd/contracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../component"

import (
	"errors"
	"time"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/plugins/usagereports/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
)

type CaptureInput struct {
	Period domain.Period `json:"period"`
}
type ReportInput struct {
	ID string `json:"id" jsonschema:"minLength=64,maxLength=64"`
}

type Header struct {
	ID         string        `json:"id"`
	Period     domain.Period `json:"period"`
	ObservedAt time.Time     `json:"observedAt"`
	Since      *time.Time    `json:"since,omitzero"`
}

// The executable binds the public core snapshot type; this contract must not
// create a second accounting representation.
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

func NewContracts[Summary any]() (Contracts[Summary], error) {
	var c Contracts[Summary]
	var err error
	c.Capture, err = pluginsdk.NewMethod[CaptureInput, Header](pluginsdk.MethodConfig{Name: "flame.usage-reports/capture.v1", Kind: capability.Mutation, Target: "installation", Description: "Capture a daily or weekly core accounting report with its actual observation time and lower bound. Totals remain core-owned; saved reports are immutable."})
	if err != nil {
		return Contracts[Summary]{}, err
	}
	c.List, err = pluginsdk.NewMethod[struct{}, []Header](pluginsdk.MethodConfig{Name: "flame.usage-reports/list.v1", Kind: capability.Query, Target: "installation", Description: "List saved accounting report headers in capture order, newest first."})
	if err != nil {
		return Contracts[Summary]{}, err
	}
	c.Get, err = pluginsdk.NewMethod[ReportInput, Report[Summary]](pluginsdk.MethodConfig{Name: "flame.usage-reports/get.v1", Kind: capability.Query, Target: "installation", Description: "Read one saved core report verbatim, preserving missing usage, unknown prices and recorded zeros."})
	if err != nil {
		return Contracts[Summary]{}, err
	}
	c.Delete, err = pluginsdk.NewMethod[ReportInput, bool](pluginsdk.MethodConfig{Name: "flame.usage-reports/delete.v1", Kind: capability.Mutation, Target: "installation", Description: "Remove one immutable saved report without changing core accounting evidence or totals."})
	if err != nil {
		return Contracts[Summary]{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.usage-reports.v1", Composition: capability.Collection, Scope: "installation"}, c.Capture.Descriptor(), c.List.Descriptor(), c.Get.Descriptor(), c.Delete.Descriptor())
	return c, err
}

func Classify(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return "invalid_report"
	case errors.Is(err, domain.ErrCapacity):
		return "archive_capacity"
	case errors.Is(err, domain.ErrNotFound):
		return "report_not_found"
	case errors.Is(err, idempotency.ErrKeyConflict):
		return "command_conflict"
	default:
		return ""
	}
}
