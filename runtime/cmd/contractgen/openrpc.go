package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/flame/runtime/protocol"
)

// The OpenRPC document is the method surface: what a client may call, what it
// passes, what comes back, and which errors are in scope. Result references point
// into schema.json; request components close the same generated type graph at the
// receiving boundary without closing reusable result shapes.

// openrpcVersion is the spec revision this document conforms to.
const openrpcVersion = "1.3.2"

// bundleRef is the sibling document holding the shapes. A relative reference
// resolves against this file's own location, which is why the bundle carries no
// $id to redirect the base.
const bundleRef = "schema.json"

type openrpcDocument struct {
	OpenRPC       string                `json:"openrpc"`
	Info          openrpcInfo           `json:"info"`
	Methods       []openrpcMethod       `json:"methods"`
	Notifications []openrpcNotification `json:"x-flame-notifications"`
	Components    openrpcComponents     `json:"components"`
}

type openrpcComponents struct {
	Schemas map[string]*schema `json:"schemas"`
}

type openrpcNotification struct {
	Name   string  `json:"name"`
	Params *schema `json:"params"`
}

type openrpcInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type openrpcMethod struct {
	Name string `json:"name"`

	// ParamStructure is by-name for every method: the wire passes one JSON object
	// whose keys are the request's own fields, so each field is a
	// named param rather than a positional one.
	ParamStructure string         `json:"paramStructure"`
	Params         []openrpcParam `json:"params"`
	Result         *openrpcResult `json:"result,omitzero"`
	Errors         []openrpcError `json:"errors,omitempty"`

	// The x-flame extensions carry what OpenRPC has no vocabulary for: retry
	// semantics, cursor pagination, capability gating, and the event stream a
	// streaming method's response body becomes.
	Kind         string          `json:"x-flame-kind"`
	Operation    string          `json:"x-flame-operation"`
	Idempotency  string          `json:"x-flame-idempotency"`
	ReplayCursor string          `json:"x-flame-replayCursor"`
	Pagination   string          `json:"x-flame-pagination"`
	Features     []string        `json:"x-flame-features,omitempty"`
	Capabilities []capabilityRow `json:"x-flame-capabilityRules,omitempty"`
	StreamEvent  *schema         `json:"x-flame-streamEvent,omitzero"`

	// RequestFrame references the whole params object. By-name params describe the
	// fields one at a time and so cannot express a cross-field rule; the frame
	// schema is where those live, and pointing at it beats restating them.
	RequestFrame *schema `json:"x-flame-requestFrame"`
}

type openrpcParam struct {
	Name     string  `json:"name"`
	Required bool    `json:"required,omitzero"`
	Schema   *schema `json:"schema"`
}

type openrpcResult struct {
	Name   string  `json:"name"`
	Schema *schema `json:"schema"`
}

// openrpcError pairs the symbolic problem type a client branches on with the
// numeric code the envelope carries. Both come from the error registry, so a
// method cannot document an error the runtime has no code for.
type openrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newOpenRPC(registry *delivery.Registry, shapes *dispatch.Shapes, set *schemaSet) openrpcDocument {
	codes := problemCodes(registry)
	requests := requestSchemas{set: set, definitions: make(map[string]*schema)}
	document := openrpcDocument{
		OpenRPC: openrpcVersion,
		Info: openrpcInfo{
			Title:       "Flame Runtime Protocol",
			Version:     protocol.ProtocolVersion,
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
	for _, field := range contractshape.Fields(meta.Params) {
		method.Params = append(method.Params, openrpcParam{
			Name:     field.Name,
			Required: !field.Optional,
			Schema:   requests.project(requestPropertySchema(set, requestFrame, meta.Params, field.Name)),
		})
	}
	method.Params = append(method.Params, openrpcParam{
		Name:   requestMetaField,
		Schema: requests.project(set.walk(reflect.TypeFor[protocol.RequestMeta]())),
	})
	result := &schema{Type: schemaTypeObject}
	if meta.Result != nil {
		result = set.walk(meta.Result)
		if meta.ResultNullable {
			result = &schema{AnyOf: []*schema{result, {Type: schemaTypeNull}}}
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

const requestMetaField = "_meta"

type requestSchemas struct {
	set         *schemaSet
	definitions map[string]*schema
}

func (r *requestSchemas) frame(business *schema) *schema {
	body := business
	if business.Ref != "" {
		name, _ := strings.CutPrefix(business.Ref, refPrefix)
		body = r.set.defs[name]
	}
	frame := r.closure(body)
	frame.AllOf = []*schema{external(business)}
	if frame.Properties == nil {
		frame.Properties = make(map[string]any)
	}
	frame.Properties[requestMetaField] = r.project(r.set.walk(reflect.TypeFor[protocol.RequestMeta]()))
	return frame
}

func (r *requestSchemas) project(node *schema) *schema {
	if node.Ref != "" {
		name, _ := strings.CutPrefix(node.Ref, refPrefix)
		body := r.set.defs[name]
		if body == nil {
			panic("contractgen: request references an undefined shape " + name)
		}
		if body.Type != schemaTypeObject {
			return external(node)
		}
		if _, defined := r.definitions[name]; !defined {
			// Reserve the component before walking its recursive references.
			r.definitions[name] = nil
			projection := r.closure(body)
			projection.AllOf = []*schema{external(node)}
			r.definitions[name] = projection
		}
		return &schema{Ref: "#/components/schemas/" + name}
	}
	if node.Type != schemaTypeObject && node.Type != schemaTypeArray {
		return external(node)
	}
	projection := r.closure(node)
	projection.AllOf = []*schema{external(node)}
	return projection
}

// closure projects only request structure. Value and cross-field constraints
// remain references to the reusable schema rather than a second published copy.
func (r *requestSchemas) closure(node *schema) *schema {
	if node.Ref != "" {
		name, _ := strings.CutPrefix(node.Ref, refPrefix)
		if body := r.set.defs[name]; body != nil && body.Type == schemaTypeObject {
			return r.project(node)
		}
		return &schema{}
	}
	switch node.Type {
	case schemaTypeArray:
		return &schema{Items: r.closure(node.Items)}
	case schemaTypeObject:
		out := &schema{Properties: make(map[string]any, len(node.Properties))}
		for name, property := range node.Properties {
			if child, ok := property.(*schema); ok {
				out.Properties[name] = r.closure(child)
			} else {
				out.Properties[name] = property
			}
		}
		if child, ok := node.AdditionalProps.(*schema); ok {
			out.AdditionalProps = r.closure(child)
		} else if node.AdditionalProps == nil {
			out.UnevaluatedProps = new(false)
		}
		return out
	default:
		return &schema{}
	}
}

// requestPropertySchema returns a by-name OpenRPC parameter from the whole
// request frame, not by walking the field's Go type again. Value constraints
// belong to the field in its owner (for example expectedRevision >= 1); a fresh
// type walk would lose that owner context and publish a weaker parameter than
// both the runtime validator and x-flame-requestFrame.
func requestPropertySchema(set *schemaSet, frame *schema, owner reflect.Type, field string) *schema {
	body := frame
	if name, ok := strings.CutPrefix(frame.Ref, refPrefix); ok {
		body = set.defs[name]
	}
	if body == nil || body.Properties == nil {
		panic(fmt.Sprintf("contractgen: %s request frame has no object properties", owner))
	}
	property, ok := body.Properties[field]
	if !ok {
		panic(fmt.Sprintf("contractgen: %s request frame has no property %q", owner, field))
	}
	node, ok := property.(*schema)
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

// external re-points every reference in a walked schema at the shape bundle.
//
// It has to be a deep copy, not a rewrite: a reference can be nested — a param of
// type []ContentBlock walks to an array whose items reference the bundle — and the
// two documents must stay independent, so rendering one can never leave a
// bundle-local reference in the other or a foreign one in the bundle.
func external(node *schema) *schema {
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
			if child, ok := value.(*schema); ok {
				out.Properties[name] = external(child)
				continue
			}
			// A forbidden field is the boolean schema `false`; it holds no reference.
			out.Properties[name] = value
		}
	}
	if child, ok := node.AdditionalProps.(*schema); ok {
		out.AdditionalProps = external(child)
	}
	// Required and Enum are read-only in both documents, so the slices are shared.
	return &out
}

func externalAll(nodes []*schema) []*schema {
	if nodes == nil {
		return nil
	}
	out := make([]*schema, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, external(node))
	}
	return out
}
