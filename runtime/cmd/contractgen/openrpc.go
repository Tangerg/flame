package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/wiregen"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

const openrpcVersion = "1.3.2"

const bundleRef = "schema.json"

type openrpcDocument struct {
	OpenRPC       string                `json:"openrpc"`
	Info          openrpcInfo           `json:"info"`
	Methods       []openrpcMethod       `json:"methods"`
	Notifications []openrpcNotification `json:"x-flame-notifications"`
	Components    openrpcComponents     `json:"components"`
}

type openrpcComponents struct {
	Schemas map[string]*wiregen.Schema `json:"schemas"`
}

type openrpcNotification struct {
	Name   string          `json:"name"`
	Params *wiregen.Schema `json:"params"`
}

type openrpcInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type openrpcMethod struct {
	Name string `json:"name"`

	ParamStructure string         `json:"paramStructure"`
	Params         []openrpcParam `json:"params"`
	Result         *openrpcResult `json:"result,omitzero"`
	Errors         []openrpcError `json:"errors,omitempty"`

	Kind         string          `json:"x-flame-kind"`
	Operation    string          `json:"x-flame-operation"`
	Idempotency  string          `json:"x-flame-idempotency"`
	ReplayCursor string          `json:"x-flame-replayCursor"`
	Pagination   string          `json:"x-flame-pagination"`
	Features     []string        `json:"x-flame-features,omitempty"`
	Capabilities []capabilityRow `json:"x-flame-capabilityRules,omitempty"`
	StreamEvent  *wiregen.Schema `json:"x-flame-streamEvent,omitzero"`

	RequestFrame *wiregen.Schema `json:"x-flame-requestFrame"`
}

type openrpcParam struct {
	Name     string          `json:"name"`
	Required bool            `json:"required,omitzero"`
	Schema   *wiregen.Schema `json:"schema"`
}

type openrpcResult struct {
	Name   string          `json:"name"`
	Schema *wiregen.Schema `json:"schema"`
}

type openrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newOpenRPC(registry *delivery.Registry, shapes *dispatch.Shapes, set *schemaSet) openrpcDocument {
	codes := problemCodes(registry)
	requests := requestSchemas{set: set, definitions: make(map[string]*wiregen.Schema)}
	document := openrpcDocument{
		OpenRPC: openrpcVersion,
		Info: openrpcInfo{
			Title:       "Flame Runtime Protocol",
			Version:     protocol.Version,
			Description: "Generated from the Contract Registry. Shapes live in schema.json; this document is the method surface.",
		},
	}
	document.Components.Schemas = requests.definitions
	for _, meta := range registry.Metas() {
		document.Methods = append(document.Methods, openrpcMethodFor(meta, &requests, codes))
	}
	for _, notification := range shapes.Notifications() {
		document.Notifications = append(document.Notifications, openrpcNotification{
			Name:   notification.Name,
			Params: external(set.walk(notification.ParamsType)),
		})
	}
	return document
}

func openrpcMethodFor(meta delivery.MethodMeta, requests *requestSchemas, codes map[string]int) openrpcMethod {
	set := requests.set
	requestFrame := set.walk(meta.Params)
	method := openrpcMethod{
		Name:           meta.Name.String(),
		ParamStructure: "by-name",
		Params:         []openrpcParam{},
		Kind:           meta.Kind.String(),
		Operation:      meta.Operation.String(),
		Idempotency:    meta.Idempotency.String(),
		ReplayCursor:   meta.ReplayCursor.String(),
		Pagination:     meta.Pagination.String(),
		Features:       meta.Features(),
		Capabilities:   capabilityRowsFor(meta),
		RequestFrame:   requests.frame(requestFrame),
	}
	for _, field := range protocol.WireFields(meta.Params) {
		method.Params = append(method.Params, openrpcParam{
			Name:     field.Name,
			Required: !field.Optional,
			Schema:   requests.project(requestPropertySchema(set, requestFrame, meta.Params, field.Name)),
		})
	}
	method.Params = append(method.Params, openrpcParam{
		Name:   protocol.RequestMetaMember,
		Schema: requests.project(set.walk(reflect.TypeFor[protocol.RequestMeta]())),
	})
	result := &wiregen.Schema{Type: wiregen.SchemaTypeObject}
	if meta.Result != nil {
		result = set.walk(meta.Result)
		if meta.ResultNullable {
			result = &wiregen.Schema{AnyOf: []*wiregen.Schema{result, {Type: wiregen.SchemaTypeNull}}}
		}
	}
	method.Result = &openrpcResult{Name: "result", Schema: external(result)}
	if meta.Event != nil {
		method.StreamEvent = external(set.walk(meta.Event))
	}
	for _, problem := range meta.ProblemTypes() {
		code, ok := codes[problem]
		if !ok {
			panic("contractgen: " + meta.Name.String() + " declares problem type " + problem + ", which the error registry has no code for")
		}
		method.Errors = append(method.Errors, openrpcError{Code: code, Message: problem})
	}
	return method
}

type requestSchemas struct {
	set         *schemaSet
	definitions map[string]*wiregen.Schema
}

func (r *requestSchemas) frame(business *wiregen.Schema) *wiregen.Schema {
	body := business
	if business.Ref != "" {
		name, _ := strings.CutPrefix(business.Ref, wiregen.RefPrefix)
		body = r.set.defs[name]
	}
	frame := r.closure(body)
	frame.AllOf = []*wiregen.Schema{external(business)}
	if frame.Properties == nil {
		frame.Properties = make(map[string]any)
	}
	frame.Properties[protocol.RequestMetaMember] = r.project(r.set.walk(reflect.TypeFor[protocol.RequestMeta]()))
	return frame
}

func (r *requestSchemas) project(node *wiregen.Schema) *wiregen.Schema {
	if node.Ref != "" {
		name, _ := strings.CutPrefix(node.Ref, wiregen.RefPrefix)
		body := r.set.defs[name]
		if body == nil {
			panic("contractgen: request references an undefined shape " + name)
		}
		if body.Type != wiregen.SchemaTypeObject {
			return external(node)
		}
		if _, defined := r.definitions[name]; !defined {
			// Reserve the component before walking its recursive references.
			r.definitions[name] = nil
			projection := r.closure(body)
			projection.AllOf = []*wiregen.Schema{external(node)}
			r.definitions[name] = projection
		}
		return &wiregen.Schema{Ref: "#/components/schemas/" + name}
	}
	if node.Type != wiregen.SchemaTypeObject && node.Type != wiregen.SchemaTypeArray {
		return external(node)
	}
	projection := r.closure(node)
	projection.AllOf = []*wiregen.Schema{external(node)}
	return projection
}

func (r *requestSchemas) closure(node *wiregen.Schema) *wiregen.Schema {
	if node.Ref != "" {
		name, _ := strings.CutPrefix(node.Ref, wiregen.RefPrefix)
		if body := r.set.defs[name]; body != nil && body.Type == wiregen.SchemaTypeObject {
			return r.project(node)
		}
		return &wiregen.Schema{}
	}
	switch node.Type {
	case wiregen.SchemaTypeArray:
		return &wiregen.Schema{Items: r.closure(node.Items)}
	case wiregen.SchemaTypeObject:
		out := &wiregen.Schema{Properties: make(map[string]any, len(node.Properties))}
		for name, property := range node.Properties {
			if child, ok := property.(*wiregen.Schema); ok {
				out.Properties[name] = r.closure(child)
			} else {
				out.Properties[name] = property
			}
		}
		if child, ok := node.AdditionalProps.(*wiregen.Schema); ok {
			out.AdditionalProps = r.closure(child)
		} else if node.AdditionalProps == nil {
			out.UnevaluatedProps = new(false)
		}
		return out
	default:
		return &wiregen.Schema{}
	}
}

func requestPropertySchema(set *schemaSet, frame *wiregen.Schema, owner reflect.Type, field string) *wiregen.Schema {
	body := frame
	if name, ok := strings.CutPrefix(frame.Ref, wiregen.RefPrefix); ok {
		body = set.defs[name]
	}
	if body == nil || body.Properties == nil {
		panic(fmt.Sprintf("contractgen: %s request frame has no object properties", owner))
	}
	property, ok := body.Properties[field]
	if !ok {
		panic(fmt.Sprintf("contractgen: %s request frame has no property %q", owner, field))
	}
	node, ok := property.(*wiregen.Schema)
	if !ok {
		panic(fmt.Sprintf("contractgen: %s.%s is forbidden and cannot be an OpenRPC parameter", owner, field))
	}
	return node
}

func capabilityRowsFor(meta delivery.MethodMeta) []capabilityRow {
	if len(meta.CapabilityRules) == 0 {
		return nil
	}
	rows := make([]capabilityRow, 0, len(meta.CapabilityRules))
	for _, rule := range meta.CapabilityRules {
		rows = append(rows, capabilityRow{When: conditions(rule.When), Requires: rule.Requires})
	}
	return rows
}

// A deep copy, not a rewrite: references can be nested, and the two documents must
// stay independent.
func external(node *wiregen.Schema) *wiregen.Schema {
	if node == nil {
		return nil
	}
	out := *node
	if out.Ref != "" {
		out.Ref = bundleRef + out.Ref
	}
	out.Items = external(node.Items)
	out.PropertyNames = external(node.PropertyNames)
	out.If = external(node.If)
	out.Then = external(node.Then)
	out.OneOf = externalAll(node.OneOf)
	out.AnyOf = externalAll(node.AnyOf)
	out.AllOf = externalAll(node.AllOf)
	if node.Properties != nil {
		out.Properties = make(map[string]any, len(node.Properties))
		for name, value := range node.Properties {
			if child, ok := value.(*wiregen.Schema); ok {
				out.Properties[name] = external(child)
				continue
			}
			out.Properties[name] = value
		}
	}
	if child, ok := node.AdditionalProps.(*wiregen.Schema); ok {
		out.AdditionalProps = external(child)
	}
	// Required and Enum are read-only in both documents, so the slices are shared.
	return &out
}

func externalAll(nodes []*wiregen.Schema) []*wiregen.Schema {
	if nodes == nil {
		return nil
	}
	out := make([]*wiregen.Schema, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, external(node))
	}
	return out
}
