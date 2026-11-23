package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/wiregen"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	runtimehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
)

type bundle struct {
	Schema  string                     `json:"$schema"`
	Title   string                     `json:"title"`
	Version string                     `json:"protocolVersion"`
	Defs    map[string]*wiregen.Schema `json:"$defs"`
}

func walkWireTypes(registry *delivery.Registry, shapes *dispatch.Shapes) *schemaSet {
	contracts := toolset.PresentationContracts()
	set := newSchemaSet(shapes)
	httpContract := runtimehttp.Contract()
	for _, enum := range httpContract.Enums {
		set.registerEnum(enum.Type, enum.Values)
	}
	for _, endpoint := range httpContract.Endpoints {
		if endpoint.ResponseType != nil {
			set.walk(endpoint.ResponseType)
		}
	}
	for _, contract := range contracts {
		for enumType, values := range contract.EnumValues {
			set.registerEnum(enumType, values)
		}
		set.walk(contract.ResultType)
	}
	for _, meta := range registry.Metas() {
		set.walk(meta.Params)
		if meta.Result != nil {
			set.walk(meta.Result)
		}
		if meta.Event != nil {
			set.walk(meta.Event)
		}
	}
	for _, notification := range shapes.Notifications() {
		set.walk(notification.ParamsType)
	}
	// Walked again so a spec for an unreachable type fails loudly instead of silently
	// describing nothing.
	for _, union := range shapes.Unions() {
		set.walk(union.GoType)
	}
	for _, constraint := range shapes.Constraints() {
		set.walk(constraint.GoType)
	}
	for _, carried := range shapes.Carried() {
		set.walk(carried.GoType)
	}
	return set
}

func newBundle(set *schemaSet) bundle {
	return bundle{
		Schema:  wiregen.SchemaDialect,
		Title:   "Flame Runtime Protocol wire types",
		Version: protocol.Version,
		Defs:    set.Definitions(),
	}
}
