package main

import (
	"fmt"
	"github.com/Tangerg/flame/packages/go-sdk/contractgen"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/contractshape"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	runtimehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
)

// The TypeScript wire types are emitted from the SCHEMA tree, not from a second
// walk of the Go types.
//
// That is the whole design: two independent walks would be two authors of the same
// shape. The schema and TypeScript types must agree.
// Deriving one from the other makes agreement structural — there is no path by
// which a variant can be forbidden in the schema and allowed in TypeScript.
//
// What TypeScript cannot express is left out on purpose: a conditional rule
// (`if/then`) is a runtime check, not a type, and the generated validator is where
// it lands. A type system that silently drops half a constraint is worse than one
// that says which half it holds.

// tsFileName is the generated shape module. Its name makes generated ownership
// explicit to every consumer that vendors the binding.
const tsFileName = "wire.generated.ts"

type tsEmitter struct {
	tsTypes
	out strings.Builder
}

// tsTypes answers "how does TypeScript spell this?" for a schema node or a Go
// type. Both generated modules ask it — the shapes module to declare a type, the
// methods module to name a method's frames — and two answers to that question
// would be two spellings of one shape.
type tsTypes struct {
	*contractgen.TypeScript
	set *schemaSet
}

func newTypeScript(set *schemaSet, notifications []string) string {
	emitter := &tsEmitter{tsTypes: tsTypes{set: set, TypeScript: contractgen.NewTypeScript(set.defs)}}
	emitter.header()
	emitter.protocolVersion()
	emitter.httpEndpoints()
	emitter.problemCodes()
	emitter.notifications(notifications)

	names := slices.Sorted(maps.Keys(set.defs))
	generics := genericBases(set)
	for genericName, instantiations := range generics {
		emitter.generic(genericName, instantiations)
	}
	for _, name := range names {
		if _, aliased := generics[genericBaseOf(set.origin[name])]; aliased && isInstantiation(set.origin[name]) {
			emitter.alias(name, set.origin[name])
			continue
		}
		emitter.out.WriteString(emitter.Definition(name, set.defs[name]))
	}
	emitter.enumValues(names)
	emitter.runEventSemantics()
	return emitter.out.String()
}

func (t *tsEmitter) problemCodes() {
	t.line("// RPC error codes and problem types describe one operation failure.")
	t.line("export const PROBLEM_CODES = {")
	codes := dispatch.ProblemCodes()
	for _, name := range slices.Sorted(maps.Keys(codes)) {
		t.line("  %s: %d,", strconv.Quote(name), codes[name])
	}
	t.line("} as const satisfies Partial<Record<ProblemData['type'], number>>;")
	t.line("")
	actions, ok := enumValues(reflect.TypeFor[protocol.RecoveryAction]())
	if !ok {
		panic("contractgen: RecoveryAction has no registered wire values")
	}
	quoted := make([]string, len(actions))
	for index, action := range actions {
		quoted[index] = strconv.Quote(action)
	}
	t.line("// The default next move a client offers for a problem, by where it arrived.")
	t.line("export type RecoveryAction = %s;", strings.Join(quoted, " | "))
	t.line("")
	t.line("// A failed request's problem.")
	t.line("export const PROBLEM_RECOVERY = {")
	for _, name := range slices.Sorted(maps.Keys(codes)) {
		recovery, declared := protocol.RecoveryFor(name)
		if !declared {
			panic("contractgen: problem type " + name + " declares no recovery action")
		}
		t.line("  %s: %s,", strconv.Quote(name), strconv.Quote(string(recovery)))
	}
	t.line("} as const satisfies Partial<Record<ProblemData['type'], RecoveryAction>>;")
	t.line("")
	t.line("// A problem that ended a Run, tool call or Item.")
	t.line("export const RUN_PROBLEM_RECOVERY = {")
	for _, contract := range dispatch.ProblemContracts() {
		if slices.Contains(contract.Channels, dispatch.ProblemChannelExecution) {
			t.line("  %s: %s,", strconv.Quote(contract.Type), strconv.Quote(string(contract.ExecutionRecovery)))
		}
	}
	t.line("} as const satisfies Partial<Record<ProblemData['type'], RecoveryAction>>;")
	t.line("")
}

// httpEndpoints publishes transport locations from the same Delivery registry
// the HTTP router uses. A vendored client never has to restate /v2 paths.
func (t *tsEmitter) httpEndpoints() {
	t.line("// HTTP entrypoints implemented by this runtime build.")
	t.line("export const HTTP_ENDPOINTS = {")
	contract := runtimehttp.Contract()
	for _, endpoint := range contract.Endpoints {
		t.line("  %s: {", contractgen.PropertyKey(endpoint.Name))
		t.line("    kind: %s,", strconv.Quote(string(endpoint.Kind)))
		t.line("    method: %s,", strconv.Quote(endpoint.Method))
		t.line("    path: %s,", strconv.Quote(endpoint.Path))
		t.line("    authentication: %s,", strconv.Quote(string(endpoint.Authentication)))
		if endpoint.Projects != "" {
			t.line("    projects: %s,", strconv.Quote(string(endpoint.Projects)))
		}
		statuses := make([]string, 0, len(endpoint.ResponseStatuses))
		for _, status := range endpoint.ResponseStatuses {
			statuses = append(statuses, strconv.Itoa(status))
		}
		t.line("    responseStatuses: [%s],", strings.Join(statuses, ", "))
		t.line("  },")
	}
	t.line("} as const;")
	t.line("")
	t.line("// Flat-JSON responses returned outside the JSON-RPC envelope.")
	t.line("export interface HTTPSidecarResponses {")
	for _, endpoint := range contract.Endpoints {
		if endpoint.Kind != runtimehttp.EndpointKindSidecar || endpoint.ResponseType == nil {
			continue
		}
		t.line("  %s: %s;", contractgen.PropertyKey(endpoint.Name), defName(endpoint.ResponseType))
	}
	t.line("}")
	t.line("")
	t.line("export type HTTPSidecarEndpointName = keyof HTTPSidecarResponses;")
	t.line("")
}

// notifications emits the downstream method names.
//
// A client subscribes to these by name — nothing inbound routes to them — so the
// name is the whole contract, and a client that spells it itself is a second author
// of the method surface. The constant name is derived mechanically so no naming
// scheme has to be invented per notification.
func (t *tsEmitter) notifications(methods []string) {
	t.line("// The methods the runtime sends downstream. A client only ever subscribes.")
	for _, method := range methods {
		t.line("export const %s = %s;", screamingSnake(method), strconv.Quote(method))
	}
	t.line("")
}

// screamingSnake turns a dotted method name into a TypeScript constant name:
// notifications.run.event becomes NOTIFICATIONS_RUN_EVENT.
func screamingSnake(name string) string {
	var out strings.Builder
	for index, letter := range name {
		switch {
		case letter == '.':
			out.WriteByte('_')
		case letter >= 'A' && letter <= 'Z' && index > 0:
			out.WriteByte('_')
			out.WriteRune(letter)
		default:
			out.WriteRune(letter - lowerToUpper(letter))
		}
	}
	return out.String()
}

func lowerToUpper(letter rune) rune {
	if letter >= 'a' && letter <= 'z' {
		return 'a' - 'A'
	}
	return 0
}

// enumValues emits the runtime face of the enums.
//
// A TypeScript union type does not exist at runtime, so a client that has to
// ITERATE a closed value set — check its sample coverage, validate an incoming
// tag — cannot get one from the types alone. One record keyed by type name avoids
// inventing a naming scheme for fifty-one separate constants, and `as const` keeps
// each list narrowed to its own literals.
func (t *tsEmitter) enumValues(names []string) {
	t.line("// The closed value sets, as data: a union type does not exist at runtime.")
	t.line("export const WIRE_ENUMS = {")
	for _, name := range names {
		node := t.set.defs[name]
		if len(node.Enum) == 0 {
			continue
		}
		t.line("  %s: [%s],", name, strings.Join(contractgen.QuoteAll(node.Enum), ", "))
	}
	t.line("} as const;")
}

func (t *tsEmitter) runEventSemantics() {
	values, ok := enumValues(reflect.TypeFor[protocol.StreamEventType]())
	if !ok {
		panic("contractgen: StreamEventType is not a registered wire enum")
	}
	t.line("")
	t.line("/** Reliability is owned by event type; a frame cannot promote itself. */")
	t.line("export type RunEventReliability = \"authoritative\" | \"ephemeral\";")
	t.line("export const RUN_EVENT_RELIABILITY = {")
	for _, value := range values {
		reliability := "ephemeral"
		if (protocol.StreamEvent{Type: protocol.StreamEventType(value)}).Authoritative() {
			reliability = "authoritative"
		}
		t.line("  %s: %s,", strconv.Quote(value), strconv.Quote(reliability))
	}
	t.line("} as const satisfies Record<StreamEventType, RunEventReliability>;")
	t.line("")
	t.line("export function runEventReliability(value: unknown): RunEventReliability | undefined {")
	t.line("  if (typeof value !== \"string\") return undefined;")
	t.line("  return (RUN_EVENT_RELIABILITY as Partial<Record<string, RunEventReliability>>)[value];")
	t.line("}")
	t.line("")
	t.line("/** Replay retention is owned by event type; live-only previews never consume the replay window. */")
	t.line("export const RUN_EVENT_REPLAYABLE = {")
	for _, value := range values {
		replayable := (protocol.StreamEvent{Type: protocol.StreamEventType(value)}).Replayable()
		t.line("  %s: %t,", strconv.Quote(value), replayable)
	}
	t.line("} as const satisfies Record<StreamEventType, boolean>;")
	t.line("")
	t.line("export function runEventIsReplayable(value: unknown): boolean | undefined {")
	t.line("  if (typeof value !== \"string\") return undefined;")
	t.line("  return (RUN_EVENT_REPLAYABLE as Partial<Record<string, boolean>>)[value];")
	t.line("}")
}

func (t *tsEmitter) header() {
	t.line("// Code generated by cmd/contractgen. DO NOT EDIT.")
	t.line("//")
	t.line("// The wire types of the Flame Runtime Protocol, projected from the Contract")
	t.line("// Registry. Prose lives with the Go types and protocol documentation; this file")
	t.line("// publishes the shapes without writing into any consuming client module.")
	t.line("//")
	t.line("// Cross-field rules — a finished run carries an outcome, an interrupt outcome")
	t.line("// carries no result — are NOT here: TypeScript has no way to state them. They are")
	t.line("// in the generated validator and in schema.json.")
	t.line("")
}

// protocolVersion emits the version the client states in request metadata.
//
// The runtime refuses a request naming a version outside the range it serves, so a
// client that spells the date itself is a second author of the negotiation — and
// the failure is a rejected handshake, not a type error. Projecting the constant
// makes "the client sends what this build serves" true by construction.
func (t *tsEmitter) protocolVersion() {
	t.line("// The wire version this runtime serves; a client states it in request metadata.")
	t.line("export const PROTOCOL_VERSION = %s;", strconv.Quote(protocol.Version))
	t.line("")
	t.line("// The only Session Artifact version this runtime imports or exports.")
	t.line("export const SESSION_ARTIFACT_VERSION = %d;", protocol.SessionArtifactVersion)
	t.line("")
	t.line("// The maximum length of one opaque pagination cursor on the public wire.")
	t.line("export const MAXIMUM_PAGINATION_CURSOR_CHARACTERS = %d;", protocol.MaximumPaginationCursorCharacters)
	t.line("")
	t.line("// Public Unicode code-point ceilings for model-selection identities.")
	t.line("export const MAXIMUM_PROVIDER_IDENTITY_CHARACTERS = %d;", protocol.MaximumProviderIdentityCharacters)
	t.line("export const MAXIMUM_MODEL_IDENTITY_CHARACTERS = %d;", protocol.MaximumModelIdentityCharacters)
	t.line("export const MAXIMUM_REASONING_EFFORT_IDENTITY_CHARACTERS = %d;", protocol.MaximumReasoningEffortIdentityCharacters)
	t.line("export const MAXIMUM_RESOURCE_IDENTITY_CHARACTERS = %d;", protocol.MaximumResourceIdentityCharacters)
	t.line("")
	t.line("// Public framing of one opaque Schedule resource identity.")
	t.line("")
	t.line("// Public framing and maximum length of one opaque Run event identity.")
	t.line("export const RUN_EVENT_ID_PREFIX = %s;", strconv.Quote(protocol.IDPrefixEvent))
	t.line("export const MAXIMUM_RUN_EVENT_ID_CHARACTERS = %d;", protocol.MaximumRunEventIDCharacters)
	t.line("")
	t.line("// Largest integer identity represented exactly by every supported JSON consumer.")
	t.line("export const MAXIMUM_EXACT_JSON_INTEGER = %d;", protocol.MaximumExactJSONInteger)
	t.line("")
	t.line("// Runtime subscriptions consume the shared exact-integer envelope.")
	t.line("export const MAXIMUM_RUNTIME_EVENT_SEQUENCE = MAXIMUM_EXACT_JSON_INTEGER;")
	t.line("")
}

// generic reconstructs a Go generic as a TypeScript generic.
//
// The walk sees only instantiations — `Page[Session]`, `Page[Model]` — because Go
// reflection has no view of an uninstantiated type. Emitting one interface per
// instantiation would publish nineteen copies of the same two fields and break
// generic helpers in consuming clients, so the shape is recovered by substituting
// the type argument out of one instantiation, and every OTHER instantiation must
// reproduce it exactly or generation fails.
func (t *tsEmitter) generic(genericName string, instantiations []string) {
	var body, source string
	for _, name := range instantiations {
		argument := t.typeName(typeArgumentOf(t.set.origin[name]))
		rendered := t.ObjectBody(t.set.defs[name], argument)
		if body == "" {
			body, source = rendered, name
			continue
		}
		if rendered != body {
			panic(fmt.Sprintf("contractgen: %s and %s are the same generic with different shapes:\n%s\n%s",
				source, name, body, rendered))
		}
	}
	if !strings.Contains(body, "T") {
		panic(fmt.Sprintf("contractgen: %s uses its type parameter nowhere", genericName))
	}
	t.line("export interface %s<T> {", genericName)
	t.out.WriteString(body)
	t.line("}")
	t.line("")
}

func (t *tsEmitter) alias(name string, shape reflect.Type) {
	t.line("export type %s = %s<%s>;", name, genericBaseOf(shape), t.typeName(typeArgumentOf(shape)))
	t.line("")
}

// typeName is the published name of a Go type, as TypeScript spells it.
func (t tsTypes) typeName(shape reflect.Type) string {
	if shape == nil {
		return "unknown"
	}
	if name := defName(shape); name != "" {
		return name
	}
	node, ok := t.set.walked[shape]
	if !ok {
		panic(fmt.Sprintf("contractgen: type %s was not walked before rendering", shape))
	}
	return t.TypeOf(node)
}

func (t *tsEmitter) line(format string, arguments ...any) {
	fmt.Fprintf(&t.out, format+"\n", arguments...)
}

// genericBases groups the instantiations of each generic Go type by generic name.
func genericBases(set *schemaSet) map[string][]string {
	out := make(map[string][]string)
	for name, t := range set.origin {
		if !isInstantiation(t) {
			continue
		}
		genericName := genericBaseOf(t)
		out[genericName] = append(out[genericName], name)
	}
	for genericName := range out {
		slices.Sort(out[genericName])
	}
	return out
}

func isInstantiation(t reflect.Type) bool {
	return t != nil && strings.Contains(t.Name(), "[")
}

func genericBaseOf(t reflect.Type) string {
	if t == nil {
		return ""
	}
	genericName, _, _ := strings.Cut(t.Name(), "[")
	return genericName
}

// typeArgumentOf returns the single type argument of a generic instantiation, found
// by matching the argument's own name against the struct's fields — reflection
// hands back instantiated field types, so the argument is recovered from them.
func typeArgumentOf(t reflect.Type) reflect.Type {
	_, arguments, generic := strings.Cut(t.Name(), "[")
	if !generic {
		return nil
	}
	wanted := strings.TrimSuffix(arguments, "]")
	if index := strings.LastIndex(wanted, "."); index >= 0 {
		wanted = wanted[index+1:]
	}
	for _, field := range contractshape.Fields(t) {
		if candidate := contractshape.Deref(field.Type); candidate.Name() == wanted {
			return candidate
		}
	}
	return nil
}
