package delivery

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type Name string

func (n Name) Valid() bool {
	segments := strings.Split(string(n), ".")
	return len(segments) >= 2 && !slices.Contains(segments, "")
}

func (n Name) String() string { return string(n) }

type MethodKind string

const (
	kindUnary  MethodKind = "unary"
	KindStream MethodKind = "stream"
)

func (m MethodKind) Valid() bool { return m == kindUnary || m == KindStream }

func (m MethodKind) String() string {
	if !m.Valid() {
		return fmt.Sprintf("MethodKind(%q)", string(m))
	}
	return string(m)
}

// Registration factories set this with its only valid idempotency policy, so a new
// command cannot compile into the registry while opting out of replay protection.
type OperationKind string

const (
	operationUnspecified  OperationKind = ""
	OperationQuery        OperationKind = "query"
	OperationCommand      OperationKind = "command"
	OperationSubscription OperationKind = "subscription"
	OperationExtension    OperationKind = "extension"
)

func (o OperationKind) Valid() bool {
	return o == OperationQuery || o == OperationCommand || o == OperationSubscription || o == OperationExtension
}

func (o OperationKind) String() string {
	if o == operationUnspecified {
		return "unspecified"
	}
	if !o.Valid() {
		return fmt.Sprintf("OperationKind(%q)", string(o))
	}
	return string(o)
}

type IdempotencyPolicy string

const (
	idempotencyNone IdempotencyPolicy = "none"

	idempotencyReplayResponse IdempotencyPolicy = "replayResponse"

	idempotencyOwnerResponse IdempotencyPolicy = "ownerResponse"

	IdempotencyOwnerRun IdempotencyPolicy = "ownerRun"
)

func (i IdempotencyPolicy) Valid() bool {
	return i == idempotencyNone || i == idempotencyReplayResponse ||
		i == idempotencyOwnerResponse || i == IdempotencyOwnerRun
}

func (i IdempotencyPolicy) Replays() bool {
	return i == idempotencyReplayResponse ||
		i == idempotencyOwnerResponse || i == IdempotencyOwnerRun
}

func (i IdempotencyPolicy) OwnsOutcome() bool {
	return i == idempotencyOwnerResponse || i == IdempotencyOwnerRun
}

type ReplayCursorPolicy string

const (
	replayCursorNone ReplayCursorPolicy = "none"
	ReplayCursorRun  ReplayCursorPolicy = "run"
)

func (r ReplayCursorPolicy) Valid() bool { return r == replayCursorNone || r == ReplayCursorRun }

type PaginationKind string

const (
	paginationNone   PaginationKind = "none"
	paginationCursor PaginationKind = "cursor"
)

func (p PaginationKind) Valid() bool { return p == paginationNone || p == paginationCursor }

type ConditionOperator string

const (
	OperatorPresent ConditionOperator = "present"
	OperatorEquals  ConditionOperator = "equals"
)

func (c ConditionOperator) Valid() bool { return c == OperatorPresent || c == OperatorEquals }

type FieldCondition struct {
	Field    string
	Operator ConditionOperator
	Value    string
}

type CapabilityRule struct {
	When     []FieldCondition
	Requires []string
}

const includeDescendantsParameter = "includeDescendants"

// Descendant Runs are child Runs, so every request field that reaches them is
// gated on the child-runs feature.
func descendantsRequireChildRuns(field string) []CapabilityRule {
	return []CapabilityRule{{
		When:     []FieldCondition{{Field: field, Operator: OperatorPresent}},
		Requires: []string{protocol.FeatureChildRuns},
	}}
}

type MethodMeta struct {
	Name   Name
	GoName string

	Kind      MethodKind
	Operation OperationKind

	Idempotency  IdempotencyPolicy
	ReplayCursor ReplayCursorPolicy
	Pagination   PaginationKind

	Errors []string

	CapabilityRules []CapabilityRule
	Acknowledgement []AcknowledgementMatch

	Materializes []Name

	Params reflect.Type
	Result reflect.Type
	Event  reflect.Type

	// Cannot be derived: nearly every handler returns a pointer that is never nil, so
	// pointer-ness says nothing about whether null is legal.
	ResultNullable bool
}

func (m MethodMeta) Features() []string {
	var out []string
	for _, rule := range m.CapabilityRules {
		for _, feature := range rule.Requires {
			if !slices.Contains(out, feature) {
				out = append(out, feature)
			}
		}
	}
	return out
}

func (m MethodMeta) ProblemTypes() []string {
	problems := slices.Clone(m.Errors)
	if len(m.CapabilityRules) > 0 &&
		!slices.Contains(problems, protocol.ErrCapabilityNotNeg.Error()) {
		problems = append(problems, protocol.ErrCapabilityNotNeg.Error())
	}
	return problems
}

func (m MethodMeta) Validate() error {
	if err := m.validateIdentity(); err != nil {
		return err
	}
	if err := m.validateProblems(); err != nil {
		return err
	}
	if err := m.validateOperationPolicy(); err != nil {
		return err
	}
	if err := m.validateShapes(); err != nil {
		return err
	}
	if err := m.validateAcknowledgement(); err != nil {
		return err
	}
	return m.validateCapabilityRules()
}

func (m MethodMeta) validateIdentity() error {
	if m.Name == "" {
		return errors.New("method name is required")
	}
	if !m.Name.Valid() {
		return fmt.Errorf(
			"method name %q is invalid; expected dot-separated non-empty segments",
			m.Name,
		)
	}
	switch m.Kind {
	case kindUnary, KindStream:
	default:
		return fmt.Errorf(
			"%s: invalid method kind %s; expected %s or %s",
			m.Name, m.Kind, kindUnary, KindStream,
		)
	}
	switch m.Operation {
	case OperationQuery, OperationCommand, OperationSubscription, OperationExtension:
	default:
		return fmt.Errorf(
			"%s: invalid operation kind %s; expected %s, %s, %s or %s",
			m.Name,
			m.Operation,
			OperationQuery,
			OperationCommand,
			OperationSubscription,
			OperationExtension,
		)
	}
	switch m.Idempotency {
	case idempotencyNone, idempotencyReplayResponse,
		idempotencyOwnerResponse, IdempotencyOwnerRun:
	default:
		return fmt.Errorf(
			"%s: invalid idempotency policy %s; expected %s, %s, %s or %s",
			m.Name,
			m.Idempotency,
			idempotencyNone,
			idempotencyReplayResponse,
			idempotencyOwnerResponse,
			IdempotencyOwnerRun,
		)
	}
	switch m.ReplayCursor {
	case replayCursorNone, ReplayCursorRun:
	default:
		return fmt.Errorf(
			"%s: invalid replay cursor policy %s; expected %s or %s",
			m.Name,
			m.ReplayCursor,
			replayCursorNone,
			ReplayCursorRun,
		)
	}
	switch m.Pagination {
	case paginationNone, paginationCursor:
	default:
		return fmt.Errorf(
			"%s: invalid pagination kind %s; expected %s or %s",
			m.Name, m.Pagination, paginationNone, paginationCursor,
		)
	}
	return nil
}

func (m MethodMeta) validateProblems() error {
	for index, problem := range m.Errors {
		if !protocol.IsMethodProblemType(problem) {
			return fmt.Errorf("%s: %q is not a declarable problem type", m.Name, problem)
		}
		if slices.Contains(m.Errors[:index], problem) {
			return fmt.Errorf("%s: problem type %q is declared twice", m.Name, problem)
		}
	}
	return nil
}

func (m MethodMeta) validateOperationPolicy() error {
	if m.Pagination == paginationCursor && m.Operation != OperationQuery {
		return fmt.Errorf("%s: only a query may return a cursor page", m.Name)
	}
	return errors.Join(m.validateMaterializes(), m.validateIdempotency(), m.validateReplayCursor())
}

func (m MethodMeta) validateMaterializes() error {
	if len(m.Materializes) > 0 && m.Operation != OperationQuery {
		return fmt.Errorf("%s: only a query may materialize other query facts", m.Name)
	}
	for index, name := range m.Materializes {
		if name == m.Name {
			return fmt.Errorf("%s: a query cannot materialize itself", m.Name)
		}
		if slices.Contains(m.Materializes[:index], name) {
			return fmt.Errorf("%s: materialized query %q is declared twice", m.Name, name)
		}
	}
	return nil
}

func (m MethodMeta) validateIdempotency() error {
	switch m.Operation {
	case OperationExtension:
		if m.Idempotency != idempotencyOwnerResponse {
			return fmt.Errorf("%s: an extension must resolve an owner operation", m.Name)
		}
	case OperationQuery:
		if m.Idempotency != idempotencyNone {
			return fmt.Errorf("%s: a query must not keep an idempotency replay", m.Name)
		}
	case OperationSubscription:
		if m.Kind != KindStream || m.Idempotency != idempotencyNone {
			return fmt.Errorf("%s: a subscription must stream and must not keep an idempotency replay", m.Name)
		}
	case OperationCommand:
		if m.Kind == kindUnary && !m.Idempotency.Replays() {
			return fmt.Errorf("%s: a unary command must replay its first response", m.Name)
		}
		if m.Kind == KindStream && !m.Idempotency.OwnsOutcome() {
			return fmt.Errorf("%s: a streaming command must own its outcome", m.Name)
		}
	case operationUnspecified:
	}
	return nil
}

func (m MethodMeta) validateReplayCursor() error {
	if m.ReplayCursor != ReplayCursorRun {
		return nil
	}
	switch {
	case m.Kind != KindStream:
		return fmt.Errorf("%s: only a streaming method may accept a run replay cursor", m.Name)
	case m.Operation == OperationCommand && m.Idempotency != IdempotencyOwnerRun:
		return fmt.Errorf("%s: a cursor-aware command must own its run opening", m.Name)
	case m.Operation != OperationCommand && m.Operation != OperationSubscription:
		return fmt.Errorf("%s: only a run command or subscription may accept a run replay cursor", m.Name)
	}
	return nil
}

func (m MethodMeta) validateShapes() error {
	if m.Params == nil {
		return fmt.Errorf("%s: params type is required — a method with no schema cannot be published", m.Name)
	}
	derivedPagination, err := paginationOf(m.Params, m.Result)
	if err != nil {
		return fmt.Errorf("%s: invalid pagination shapes: %w", m.Name, err)
	}
	if m.Pagination != derivedPagination {
		return fmt.Errorf(
			"%s: pagination is %s but its request/result shapes derive %s",
			m.Name, m.Pagination, derivedPagination,
		)
	}
	if m.Event != nil {
		if err := protocol.ValidateWireType(m.Event); err != nil {
			return fmt.Errorf("%s: invalid event declaration: %w", m.Name, err)
		}
	}
	if (m.Kind == KindStream) != (m.Event != nil) {
		return fmt.Errorf("%s: a stream declares its event type and only a stream has one", m.Name)
	}
	if m.ReplayCursor == ReplayCursorRun && m.Event != reflect.TypeFor[protocol.RunEvent]() {
		return fmt.Errorf("%s: a run replay cursor requires RunEvent stream frames", m.Name)
	}
	if m.ResultNullable && (m.Result == nil || m.Result.Kind() != reflect.Pointer) {
		return fmt.Errorf("%s: a nullable result needs a pointer to be nil", m.Name)
	}
	return nil
}

func (m MethodMeta) validateCapabilityRules() error {
	for ruleIndex, rule := range m.CapabilityRules {
		if len(rule.Requires) == 0 {
			return fmt.Errorf(
				"%s: capability rule %d must require at least one feature",
				m.Name, ruleIndex,
			)
		}
		for conditionIndex, condition := range rule.When {
			if slices.Contains(rule.When[:conditionIndex], condition) {
				return fmt.Errorf(
					"%s: capability rule %d repeats condition for field %q with operator %s",
					m.Name, ruleIndex, condition.Field, condition.Operator,
				)
			}
			if err := ValidateFieldCondition(m.Name.String(), m.Params, condition); err != nil {
				return err
			}
		}
		for featureIndex, feature := range rule.Requires {
			if _, published := protocol.LookupFeature(feature); !published {
				return fmt.Errorf(
					"%s: capability rule %d requires unknown feature %q",
					m.Name, ruleIndex, feature,
				)
			}
			if slices.Contains(rule.Requires[:featureIndex], feature) {
				return fmt.Errorf(
					"%s: capability rule %d requires feature %q twice",
					m.Name, ruleIndex, feature,
				)
			}
		}
	}
	if len(m.CapabilityRules) > 0 &&
		slices.Contains(m.Errors, protocol.ErrCapabilityNotNeg.Error()) {
		return fmt.Errorf(
			"%s: capability_not_negotiated is derived from capability rules and must not be declared twice",
			m.Name,
		)
	}
	return nil
}

func (i IdempotencyPolicy) String() string {
	if !i.Valid() {
		return fmt.Sprintf("IdempotencyPolicy(%q)", string(i))
	}
	return string(i)
}

func (r ReplayCursorPolicy) String() string {
	if !r.Valid() {
		return fmt.Sprintf("ReplayCursorPolicy(%q)", string(r))
	}
	return string(r)
}

func (p PaginationKind) String() string {
	if !p.Valid() {
		return fmt.Sprintf("PaginationKind(%q)", string(p))
	}
	return string(p)
}

func (c ConditionOperator) String() string {
	if !c.Valid() {
		return fmt.Sprintf("ConditionOperator(%q)", string(c))
	}
	return string(c)
}

func ValidateFieldCondition(owner string, shape reflect.Type, condition FieldCondition) error {
	if condition.Field == "" {
		return fmt.Errorf("%s: condition must name a field", owner)
	}
	if err := protocol.ValidateWirePath(shape, condition.Field); err != nil {
		return fmt.Errorf("%s condition field %q: %w", owner, condition.Field, err)
	}
	_, leaf, _ := protocol.WireGoPath(shape, condition.Field)
	switch condition.Operator {
	case OperatorPresent:
		if condition.Value != "" {
			return fmt.Errorf(
				"%s condition field %q: operator %s does not accept value %q",
				owner, condition.Field, condition.Operator, condition.Value,
			)
		}
	case OperatorEquals:
		if condition.Value == "" {
			return fmt.Errorf(
				"%s condition field %q: operator %s requires a non-empty value",
				owner, condition.Field, condition.Operator,
			)
		}
		if leaf.Type.Kind() != reflect.String {
			return fmt.Errorf(
				"%s condition field %q: operator %s requires a string field, got %s",
				owner, condition.Field, condition.Operator, leaf.Type,
			)
		}
	default:
		return fmt.Errorf(
			"%s condition field %q: invalid operator %s; expected %s or %s",
			owner,
			condition.Field,
			condition.Operator,
			OperatorPresent,
			OperatorEquals,
		)
	}
	return nil
}
