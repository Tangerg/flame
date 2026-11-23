package main

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/wiregen"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

const tsMethodsFileName = "wire.methods.generated.ts"

type methodsEmitter struct {
	tsTypes
	out strings.Builder

	imported map[string]bool
}

func newWireMethods(registry *delivery.Registry, set *schemaSet) string {
	emitter := &methodsEmitter{tsTypes: tsTypes{set: set, TypeScript: wiregen.NewTypeScript(set.defs)}, imported: make(map[string]bool)}
	metas := registry.Metas()

	shapes := emitter.shapes(metas)

	emitter.header()
	emitter.imports()
	emitter.features()
	emitter.names(metas)
	emitter.streamingNames(metas)
	emitter.valueMethodNames(metas)
	emitter.methodPolicy(metas)
	emitter.acknowledgementChecks(metas)
	emitter.policy(metas)
	emitter.out.WriteString(shapes)
	emitter.helpers()
	return emitter.out.String()
}

func (m *methodsEmitter) methodPolicy(metas []delivery.MethodMeta) {
	m.line("export type WireOperationKind = %s;", methodPolicyUnion(metas, func(meta delivery.MethodMeta) string { return meta.Operation.String() }))
	m.line("export type WireResponseKind = %s;", methodPolicyUnion(metas, func(meta delivery.MethodMeta) string { return meta.Kind.String() }))
	m.line("export type WireIdempotencyPolicy = %s;", methodPolicyUnion(metas, func(meta delivery.MethodMeta) string { return meta.Idempotency.String() }))
	m.line("export type WireReplayCursorPolicy = %s;", methodPolicyUnion(metas, func(meta delivery.MethodMeta) string { return meta.ReplayCursor.String() }))
	m.line("export type WirePaginationKind = %s;", methodPolicyUnion(metas, func(meta delivery.MethodMeta) string { return meta.Pagination.String() }))
	m.line("")
	m.line("export interface WireMethodPolicy {")
	m.line("  operation: WireOperationKind;")
	m.line("  response: WireResponseKind;")
	m.line("  idempotency: WireIdempotencyPolicy;")
	m.line("  replayCursor: WireReplayCursorPolicy;")
	m.line("  pagination: WirePaginationKind;")
	m.line("}")
	m.line("")
	m.line("export const WIRE_METHOD_POLICY = {")
	for _, meta := range metas {
		m.line("  %s: {", wiregen.StringLiteral(meta.Name.String()))
		m.line("    operation: %s,", wiregen.StringLiteral(meta.Operation.String()))
		m.line("    response: %s,", wiregen.StringLiteral(meta.Kind.String()))
		m.line("    idempotency: %s,", wiregen.StringLiteral(meta.Idempotency.String()))
		m.line("    replayCursor: %s,", wiregen.StringLiteral(meta.ReplayCursor.String()))
		m.line("    pagination: %s,", wiregen.StringLiteral(meta.Pagination.String()))
		m.line("  },")
	}
	m.line("} as const satisfies { readonly [M in WireMethodName]: WireMethodPolicy };")
	m.line("")
	m.line("/** True only for calls whose first response the runtime durably replays. */")
	m.line("export type WireMutationMethodName = {")
	m.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"operation\"] extends \"command\"")
	m.line("    ? M")
	m.line("    : never;")
	m.line("}[WireMethodName];")
	m.line("")
	m.line("export function wireMethodRequiresIdempotency(method: WireMethodName): boolean {")
	m.line("  return WIRE_METHOD_POLICY[method].operation === \"command\";")
	m.line("}")
	m.line("")
	m.line("/** Every stream that accepts an opaque Run event replay cursor. */")
	m.line("export type WireReplayCursorMethodName = {")
	m.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"replayCursor\"] extends \"run\"")
	m.line("    ? M")
	m.line("    : never;")
	m.line("}[WireMethodName];")
	m.line("")
	m.line("export function wireMethodAcceptsReplayCursor(")
	m.line("  method: WireMethodName,")
	m.line("): method is WireReplayCursorMethodName {")
	m.line("  return WIRE_METHOD_POLICY[method].replayCursor === \"run\";")
	m.line("}")
	m.line("")
	m.line("/** Every cursor-paginated method, derived from the method policy above. */")
	m.line("export type WirePaginatedMethodName = {")
	m.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"pagination\"] extends \"cursor\"")
	m.line("    ? M")
	m.line("    : never;")
	m.line("}[WireMethodName];")
	m.line("")
	m.line("export function wireMethodIsPaginated(")
	m.line("  method: WireMethodName,")
	m.line("): method is WirePaginatedMethodName {")
	m.line("  return WIRE_METHOD_POLICY[method].pagination === \"cursor\";")
	m.line("}")
	m.line("")
}

func (m *methodsEmitter) policy(metas []delivery.MethodMeta) {
	m.line("/** One condition on the request that decides whether a rule applies. */")
	m.line("export interface WireCapabilityCondition {")
	m.line("  field: string;")
	m.line("  operator: \"present\" | \"equals\";")
	m.line("  value?: string;")
	m.line("}")
	m.line("")
	m.line("/**")
	m.line(" * What a method needs negotiated. An absent `when` gates the whole method; a")
	m.line(" * condition gates only the requests that match it.")
	m.line(" */")
	m.line("export interface WireCapabilityRule {")
	m.line("  when?: readonly WireCapabilityCondition[];")
	m.line("  requires: readonly WireFeature[];")
	m.line("}")
	m.line("")
	m.line("export const WIRE_CAPABILITY_POLICY: {")
	m.line("  readonly [M in WireMethodName]?: readonly WireCapabilityRule[];")
	m.line("} = {")
	for _, meta := range metas {
		if len(meta.CapabilityRules) == 0 {
			continue
		}
		m.line("  %s: [", wiregen.StringLiteral(meta.Name.String()))
		for _, rule := range meta.CapabilityRules {
			m.line("    { %s },", renderRule(rule))
		}
		m.line("  ],")
	}
	m.line("};")
	m.line("")
}

func renderRule(rule delivery.CapabilityRule) string {
	var members []string
	if len(rule.When) != 0 {
		conditions := make([]string, 0, len(rule.When))
		for _, condition := range rule.When {
			parts := []string{
				"field: " + wiregen.StringLiteral(condition.Field),
				"operator: " + wiregen.StringLiteral(condition.Operator.String()),
			}
			if condition.Value != "" {
				parts = append(parts, "value: "+wiregen.StringLiteral(condition.Value))
			}
			conditions = append(conditions, "{ "+strings.Join(parts, ", ")+" }")
		}
		members = append(members, "when: ["+strings.Join(conditions, ", ")+"]")
	}
	members = append(members, "requires: ["+strings.Join(wiregen.QuoteAll(rule.Requires), ", ")+"]")
	return strings.Join(members, ", ")
}

func (m *methodsEmitter) header() {
	m.line("// Code generated by cmd/contractgen. DO NOT EDIT.")
	m.line("//")
	m.line("// The method surface of the Flame Runtime Protocol: which methods exist, what each")
	m.line("// one carries, and what has to be negotiated before one will run.")
	m.line("//")
	m.line("// The SDK in methods.ts composes these with transport concerns. Every protocol")
	m.line("// fact in this file is read from the Contract Registry.")
	m.line("")
}

func (m *methodsEmitter) imports() {
	names := slices.Sorted(maps.Keys(m.imported))
	if len(names) == 0 {
		return
	}
	m.line("import type {")
	for _, name := range names {
		m.line("  %s,", name)
	}
	m.line("} from \"./wire.generated\";")
	m.line("")
}

func (m *methodsEmitter) features() {
	m.line("// Every capability key discovery may advertise. Private: it exists to")
	m.line("// derive the union, and a published array with no reader would be a second table.")
	m.line("const FEATURES = [")
	for _, key := range protocol.FeatureKeys() {
		m.line("  %s,", wiregen.StringLiteral(key))
	}
	m.line("] as const;")
	m.line("")
	m.line("/** One capability key discovery may advertise. */")
	m.line("export type WireFeature = (typeof FEATURES)[number];")
	m.line("")
}

func (m *methodsEmitter) names(metas []delivery.MethodMeta) {
	m.line("// Every method the runtime routes, in registration order.")
	m.line("const METHOD_NAMES = [")
	for _, meta := range metas {
		m.line("  %s,", wiregen.StringLiteral(meta.Name.String()))
	}
	m.line("] as const;")
	m.line("")
	m.line("/** One method the runtime routes. */")
	m.line("export type WireMethodName = (typeof METHOD_NAMES)[number];")
	m.line("")
}

func (m *methodsEmitter) streamingNames(metas []delivery.MethodMeta) {
	m.line("// Every method whose HTTP response remains open as an event stream.")
	m.line("export const WIRE_STREAMING_METHOD_NAMES = [")
	for _, meta := range metas {
		if meta.Event != nil {
			m.line("  %s,", wiregen.StringLiteral(meta.Name.String()))
		}
	}
	m.line("] as const;")
	m.line("")
	m.line("export type WireStreamingMethodName = (typeof WIRE_STREAMING_METHOD_NAMES)[number];")
	m.line("")
	m.line("export function isWireStreamingMethodName(")
	m.line("  method: WireMethodName,")
	m.line("): method is WireStreamingMethodName {")
	m.line("  return (WIRE_STREAMING_METHOD_NAMES as readonly string[]).includes(method);")
	m.line("}")
	m.line("")
}

func (m *methodsEmitter) valueMethodNames(metas []delivery.MethodMeta) {
	m.line("// Methods whose validated wire result becomes a value in the ergonomic SDK.")
	m.line("const VALUE_METHOD_NAMES = [")
	for _, meta := range metas {
		if meta.Result != nil {
			m.line("  %s,", wiregen.StringLiteral(meta.Name.String()))
		}
	}
	m.line("] as const;")
	m.line("")
	m.line("type WireValueMethodName = (typeof VALUE_METHOD_NAMES)[number];")
	m.line("")
	m.line("export function wireMethodReturnsValue(")
	m.line("  method: WireMethodName,")
	m.line("): method is WireValueMethodName {")
	m.line("  return (VALUE_METHOD_NAMES as readonly string[]).includes(method);")
	m.line("}")
	m.line("")
}

func (m *methodsEmitter) shapes(metas []delivery.MethodMeta) string {
	var out strings.Builder
	fmt.Fprintln(&out, "/** The frames each method carries. */")
	fmt.Fprintln(&out, "export interface WireShapes {")
	for _, meta := range metas {
		members := []string{"params: " + m.shape(meta.Params)}
		if meta.Result != nil {
			result := m.shape(meta.Result)
			if meta.ResultNullable {
				result += " | null"
			}
			members = append(members, "result: "+result)
		}
		fmt.Fprintf(&out, "  %s: { %s };\n", wiregen.StringLiteral(meta.Name.String()), strings.Join(members, "; "))
	}
	fmt.Fprintln(&out, "}")
	fmt.Fprintln(&out, "")
	return out.String()
}

func (m *methodsEmitter) helpers() {
	m.line("export type WireParams<M extends WireMethodName> = WireShapes[M][\"params\"];")
	m.line("")
	m.line("/** A method with no result answers with an empty success. */")
	m.line("export type WireResult<M extends WireMethodName> =")
	m.line("  WireShapes[M] extends { result: infer R } ? R : void;")
}

func (m *methodsEmitter) shape(t reflect.Type) string {
	name := m.typeName(t)
	if _, published := m.set.defs[name]; published {
		m.imported[name] = true
	}
	return name
}

func (m *methodsEmitter) line(format string, arguments ...any) {
	fmt.Fprintf(&m.out, format+"\n", arguments...)
}

func methodPolicyUnion(metas []delivery.MethodMeta, value func(delivery.MethodMeta) string) string {
	values := make([]string, 0, len(metas))
	for _, meta := range metas {
		values = append(values, wiregen.StringLiteral(value(meta)))
	}
	slices.Sort(values)
	values = slices.Compact(values)
	if len(values) == 0 {
		return "never"
	}
	return strings.Join(values, " | ")
}
