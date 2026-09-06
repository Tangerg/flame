package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
)

// WireValidator is implemented by a DTO whose wire contract is stricter than its
// flat Go representation.
//
// Implementations are GENERATED (wire_constraints.generated.go) from the Contract
// Registry. Value constraints, closed-enum membership, union variants and
// conditional field rules therefore have one author shared by Go, JSON Schema
// and TypeScript. A hand-written ValidateWire would be a second author.
//
// [ValidateWireTree] composes these node-local validators at delivery boundaries.
// Keeping the generated method local to one DTO avoids generated parent shapes
// restating child rules, while the tree walk makes it impossible for a response
// or event to skip a constrained nested DTO.
//
// ValidateWire stays a pure function of the value — no storage, dispatcher or
// executor. "Does this session exist" is not a
// shape constraint and remains a use-case decision.
type WireValidator interface {
	ValidateWire() error
}

// ValidateWireTree validates every constrained DTO reachable through the JSON
// representation of value. It is the delivery-boundary operation: requests,
// responses, errors and events call it once at their root, and it composes the
// generated node-local [WireValidator] implementations with precise JSON paths.
//
// Interface-valued payloads are intentionally opaque. They carry extension or
// provider data whose schema is owned outside the first-party wire contract, so
// recursively interpreting a concrete value hidden behind `any` would turn an
// implementation detail into protocol.
func ValidateWireTree(value any) error {
	root := reflect.ValueOf(value)
	if !root.IsValid() {
		return nil
	}
	rootType := root.Type()
	for rootType.Kind() == reflect.Pointer {
		rootType = rootType.Elem()
	}
	shape := rootType.Name()
	if shape == "" {
		shape = "wire value"
	}

	fields := validateWireValue(root, "", make(map[wirePointer]bool))
	if len(fields) == 0 {
		return nil
	}
	return &ConstraintError{Shape: shape, Fields: uniqueFieldErrors(fields)}
}

type wirePointer struct {
	typ reflect.Type
	ptr uintptr
}

func validateWireValue(value reflect.Value, path string, visiting map[wirePointer]bool) []FieldError {
	if !value.IsValid() {
		return nil
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		pointer := wirePointer{typ: value.Type(), ptr: value.Pointer()}
		if visiting[pointer] {
			return nil
		}
		visiting[pointer] = true
		defer delete(visiting, pointer)
		value = value.Elem()
	}
	// An interface marks an intentionally opaque wire boundary. Do not unwrap it:
	// Result, payload and Arguments may contain arbitrary third-party JSON.
	if value.Kind() == reflect.Interface {
		return nil
	}

	var fields []FieldError
	if value.CanInterface() {
		if validator, ok := value.Interface().(WireValidator); ok {
			fields = append(fields, prefixedWireErrors(path, validator.ValidateWire())...)
		}
	}

	switch value.Kind() {
	case reflect.Struct:
		for _, field := range contractshape.Fields(value.Type()) {
			fields = append(fields, validateWireValue(
				value.FieldByName(field.GoName),
				joinWirePath(path, field.Name),
				visiting,
			)...)
		}
	case reflect.Slice, reflect.Array:
		for index := range value.Len() {
			fields = append(fields, validateWireValue(
				value.Index(index),
				fmt.Sprintf("%s[%d]", path, index),
				visiting,
			)...)
		}
	}
	return fields
}

func prefixedWireErrors(path string, err error) []FieldError {
	if err == nil {
		return nil
	}
	if constraint, ok := errors.AsType[*ConstraintError](err); ok {
		fields := make([]FieldError, 0, len(constraint.Fields))
		for _, field := range constraint.Fields {
			field.Field = joinWirePath(path, field.Field)
			fields = append(fields, field)
		}
		return fields
	}
	field := path
	if field == "" {
		field = "$"
	}
	return []FieldError{{Field: field, Detail: err.Error()}}
}

func joinWirePath(prefix, field string) string {
	switch {
	case prefix == "":
		return field
	case field == "":
		return prefix
	default:
		return prefix + "." + field
	}
}

func uniqueFieldErrors(fields []FieldError) []FieldError {
	seen := make(map[FieldError]bool, len(fields))
	out := make([]FieldError, 0, len(fields))
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

// ConstraintError reports which fields of a wire shape violated their contract.
// Shape qualifies diagnostics without polluting [FieldError.Field]: request errors
// still carry client-addressable paths such as "scope.type", while logs say
// "ListItemsRequest.scope.type".
//
// Detail strings are programmer diagnostics, not UI copy — a client renders its
// own localized message keyed by field + type, exactly as it
// does for a ProblemData.type.
type ConstraintError struct {
	Shape  string
	Fields []FieldError
}

func (c *ConstraintError) Error() string {
	parts := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		path := f.Field
		if c.Shape != "" {
			path = c.Shape + "." + path
		}
		parts = append(parts, path+": "+f.Detail)
	}
	return strings.Join(parts, "; ")
}

// Enrich preserves the exact offending fields when a nested decoder returns the
// error through the normal dispatcher error path.
func (c *ConstraintError) Enrich(data *ProblemData) {
	data.Errors = append(data.Errors, c.Fields...)
}

// collectWireViolations returns nil when there is nothing to report, so a
// generated validator can compose independent rules without branching around
// every check.
func collectWireViolations(shape string, fields ...FieldError) error {
	present := make([]FieldError, 0, len(fields))
	for _, f := range fields {
		if f.Field != "" {
			present = append(present, f)
		}
	}
	if len(present) == 0 {
		return nil
	}
	return &ConstraintError{Shape: shape, Fields: present}
}

func requiredText(field, value string) FieldError {
	if value == "" {
		return FieldError{Field: field, Detail: "is required"}
	}
	return FieldError{}
}

func optionalText[Text ~string](field string, value *Text) FieldError {
	if value != nil && *value == "" {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

type wireNumber interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type wirePositiveNumber interface {
	wireNumber | ~float32 | ~float64
}

func positiveNumber[Number wirePositiveNumber](field string, value Number) FieldError {
	number := float64(value)
	if number <= 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return FieldError{Field: field, Detail: "must be finite and greater than zero"}
	}
	return FieldError{}
}

// optionalPositiveScalarNumber treats zero as absence because encoding/json omits an
// optional scalar at its zero value. A negative value is still present and
// illegal. JSON Schema and the TypeScript validator apply minimum: 1 only when
// the property exists, which is the same serialized contract.
func optionalPositiveScalarNumber[Number wirePositiveNumber](field string, value Number) FieldError {
	number := float64(value)
	if number < 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return FieldError{Field: field, Detail: "must be finite and greater than zero when present"}
	}
	return FieldError{}
}

func optionalPositiveNumber[Number wirePositiveNumber](field string, value *Number) FieldError {
	if value == nil {
		return FieldError{}
	}
	return positiveNumber(field, *value)
}

type wireNumeric interface {
	wireNumber | ~float32 | ~float64
}

func nonNegativeNumber[Number wireNumeric](field string, value Number) FieldError {
	number := float64(value)
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return FieldError{Field: field, Detail: "must be finite and non-negative"}
	}
	return FieldError{}
}

func optionalNonNegativeNumber[Number wireNumeric](field string, value *Number) FieldError {
	if value == nil {
		return FieldError{}
	}
	return nonNegativeNumber(field, *value)
}

func minimumNumber[Number wireNumeric](field string, value Number, minimum Number) FieldError {
	number := float64(value)
	// Compare in Number's own representation. Converting both operands to
	// float64 first would collapse adjacent uint64 values above 2^53 and could
	// silently accept an integer outside the generated contract.
	if math.IsNaN(number) || math.IsInf(number, 0) || value < minimum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must be at least %v", minimum)}
	}
	return FieldError{}
}

func maximumNumber[Number wireNumeric](field string, value Number, maximum Number) FieldError {
	number := float64(value)
	if math.IsNaN(number) || math.IsInf(number, 0) || value > maximum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must be at most %v", maximum)}
	}
	return FieldError{}
}

func optionalMaximumNumber[Number wireNumeric](field string, value *Number, maximum Number) FieldError {
	if value == nil {
		return FieldError{}
	}
	return maximumNumber(field, *value, maximum)
}

// requiredItems rejects an absent or empty required array. Requiredness comes
// from the DTO's JSON tag; the generator selects this helper for a field without
// omitempty so the runtime enforces the same required + minItems contract as the
// schema and generated client.
func requiredItems[T any](field string, values []T) FieldError {
	if values == nil {
		return FieldError{Field: field, Detail: "is required"}
	}
	if len(values) == 0 {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

// nonEmptyItems rejects an optional array that was sent with nothing in it. A
// nil slice is the field's absence, which remains valid for an optional field.
func nonEmptyItems[T any](field string, values []T) FieldError {
	if values != nil && len(values) == 0 {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

func optionalMinItems[T any](field string, values []T, minimum int) FieldError {
	if values != nil && len(values) < minimum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must contain at least %d items", minimum)}
	}
	return FieldError{}
}

func maxItems[T any](field string, values []T, maximum int) FieldError {
	if len(values) > maximum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must contain at most %d items", maximum)}
	}
	return FieldError{}
}

func optionalMaxItems[T any](field string, values *[]T, maximum int) FieldError {
	if values == nil {
		return FieldError{}
	}
	return maxItems(field, *values, maximum)
}

func maxLength(field, value string, maximum int) FieldError {
	if utf8.RuneCountInString(value) > maximum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must contain at most %d characters", maximum)}
	}
	return FieldError{}
}

func optionalMaxLength(field string, value *string, maximum int) FieldError {
	if value == nil {
		return FieldError{}
	}
	return maxLength(field, *value, maximum)
}

func maxItemLength[Identity ~string](field string, values []Identity, maximum int) FieldError {
	for index, value := range values {
		if violation := maxLength(fmt.Sprintf("%s[%d]", field, index), string(value), maximum); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func identity(field, value string) FieldError {
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return FieldError{Field: field, Detail: "must contain only printable non-whitespace characters"}
		}
	}
	return FieldError{}
}

func optionalIdentity(field string, value *string) FieldError {
	if value == nil {
		return FieldError{}
	}
	return identity(field, *value)
}

func identityItems[Identity ~string](field string, values []Identity) FieldError {
	for index, value := range values {
		if violation := identity(fmt.Sprintf("%s[%d]", field, index), string(value)); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func textPrefixItems[Identity ~string](field string, values []Identity, prefix string) FieldError {
	for index, value := range values {
		if !strings.HasPrefix(string(value), prefix) {
			return FieldError{
				Field:  fmt.Sprintf("%s[%d]", field, index),
				Detail: fmt.Sprintf("must be prefixed by %q", prefix),
			}
		}
	}
	return FieldError{}
}

func textPatternItems[Identity ~string](field string, values []Identity, pattern string) FieldError {
	for index, value := range values {
		if violation := requiredTextPattern(
			fmt.Sprintf("%s[%d]", field, index),
			string(value),
			pattern,
		); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func optionalTextPatternItems[Identity ~string](field string, values *[]Identity, pattern string) FieldError {
	if values == nil {
		return FieldError{}
	}
	return textPatternItems(field, *values, pattern)
}

func maxPropertyNameLength[Value any](field string, values map[string]Value, maximum int) FieldError {
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if violation := maxLength(fmt.Sprintf("%s[%q]", field, key), key, maximum); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func identityPropertyNames[Value any](field string, values map[string]Value) FieldError {
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if violation := identity(fmt.Sprintf("%s[%q]", field, key), key); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func requiredTextPrefix(field, value, prefix string) FieldError {
	if !strings.HasPrefix(value, prefix) {
		return FieldError{Field: field, Detail: fmt.Sprintf("must start with %q", prefix)}
	}
	return FieldError{}
}

func optionalTextPointerPrefix(field string, value *string, prefix string) FieldError {
	if value == nil {
		return FieldError{}
	}
	return requiredTextPrefix(field, *value, prefix)
}

func requiredTextPattern(field, value, pattern string) FieldError {
	matched, err := regexp.MatchString(pattern, value)
	if err != nil || !matched {
		return FieldError{Field: field, Detail: fmt.Sprintf("must match %q", pattern)}
	}
	return FieldError{}
}

func optionalTextPattern(field, value, pattern string) FieldError {
	if value == "" {
		return FieldError{}
	}
	return requiredTextPattern(field, value, pattern)
}

func optionalTextPointerPattern(field string, value *string, pattern string) FieldError {
	if value == nil {
		return FieldError{}
	}
	return requiredTextPattern(field, *value, pattern)
}

// nonEmptyProperties rejects an empty object map. nil remains a valid omission;
// a present empty map is rejected by the same length check after decoding.
func nonEmptyProperties[Value any](field string, values map[string]Value) FieldError {
	if values != nil && len(values) == 0 {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

// uniqueItems rejects a repeated JSON value, so objects compare by content rather
// than Go comparability or pointer identity. encoding/json sorts map keys, giving
// every representable value a deterministic key without a second equality model.
func uniqueItems[T any](field string, values []T) FieldError {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return FieldError{Field: field, Detail: "must contain only JSON values"}
		}
		key := string(encoded)
		if seen[key] {
			return FieldError{Field: field, Detail: "must not repeat a value"}
		}
		seen[key] = true
	}
	return FieldError{}
}

func optionalUniqueItems[T any](field string, values *[]T) FieldError {
	if values == nil {
		return FieldError{}
	}
	return uniqueItems(field, *values)
}

// closedEnum rejects a value outside a closed set. Go's decoder puts any string
// into a named string type, so typing alone cannot enforce membership.
func closedEnum(field, value string, values []string, optional bool) FieldError {
	if value == "" && optional {
		return FieldError{}
	}
	if slices.Contains(values, value) {
		return FieldError{}
	}
	return FieldError{Field: field, Detail: "must be one of " + strings.Join(values, ", ")}
}

// closedEnumItems applies the same membership rule to every element of an enum
// array. Schema and TypeScript validate at the item boundary; returning the exact
// index keeps Go diagnostics equally actionable.
func closedEnumItems[Enum ~string](field string, items []Enum, values []string) FieldError {
	for index, item := range items {
		if !slices.Contains(values, string(item)) {
			return FieldError{
				Field:  fmt.Sprintf("%s[%d]", field, index),
				Detail: "must be one of " + strings.Join(values, ", "),
			}
		}
	}
	return FieldError{}
}

func unionTag(field, value string, literals []string, pattern string) FieldError {
	if slices.Contains(literals, value) {
		return FieldError{}
	}
	if matched, err := regexp.MatchString(pattern, value); err == nil && matched {
		return FieldError{}
	}
	return FieldError{Field: field, Detail: "must be a known tag or match " + pattern}
}

func requiredWhen(applies bool, field string, value any) FieldError {
	if applies && !wireFieldPresent(value, field) {
		return FieldError{Field: field, Detail: "is required"}
	}
	return FieldError{}
}

// requiredAnyWhen reports one rule-level violation when none of the alternative
// fields is present. Keeping the alternatives in one diagnostic preserves the
// fact that satisfying any one of them is sufficient.
func requiredAnyWhen(applies bool, fields []string, value any) FieldError {
	if !applies {
		return FieldError{}
	}
	for _, field := range fields {
		if wireFieldPresent(value, field) {
			return FieldError{}
		}
	}
	return FieldError{
		Field:  strings.Join(fields, "|"),
		Detail: "at least one field is required",
	}
}

func forbiddenWhen(applies bool, field string, value any) FieldError {
	if applies && wireFieldPresent(value, field) {
		return FieldError{Field: field, Detail: "must not be present here"}
	}
	return FieldError{}
}

func allowedValuesWhen(applies bool, field string, value any, allowed []string) FieldError {
	if !applies {
		return FieldError{}
	}
	actual, _, found := lookupWireValue(reflect.ValueOf(value), field)
	if !found {
		// Requiredness is a separate generated rule. Avoid reporting two different
		// diagnostics for the same absent field.
		return FieldError{}
	}
	if actual.Kind() != reflect.String || !slices.Contains(allowed, actual.String()) {
		return FieldError{Field: field, Detail: "must be one of " + strings.Join(allowed, ", ") + " here"}
	}
	return FieldError{}
}

func wireFieldEquals(value any, path, expected string) bool {
	field, _, ok := lookupWireValue(reflect.ValueOf(value), path)
	return ok && field.Kind() == reflect.String && field.String() == expected
}

func wireFieldMatches(value any, path, pattern string) bool {
	field, _, ok := lookupWireValue(reflect.ValueOf(value), path)
	if !ok || field.Kind() != reflect.String {
		return false
	}
	matched, err := regexp.MatchString(pattern, field.String())
	return err == nil && matched
}

// wireFieldPresent answers whether encoding/json would put a registered field on
// the wire. The registry has already proved every path exists; this reflection is
// only the shared runtime interpretation of presence, including nested pointers
// and omitempty collections.
func wireFieldPresent(value any, path string) bool {
	field, optional, ok := lookupWireValue(reflect.ValueOf(value), path)
	if !ok {
		return false
	}
	if !optional {
		return true
	}
	switch field.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return field.Len() > 0
	case reflect.Bool:
		return field.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return field.Float() != 0
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Pointer:
		return !field.IsNil()
	default:
		return !field.IsZero()
	}
}

func lookupWireValue(value reflect.Value, path string) (reflect.Value, bool, bool) {
	current := value
	var optional bool
	for segment := range strings.SplitSeq(path, ".") {
		for current.IsValid() && (current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer) {
			if current.IsNil() {
				return reflect.Value{}, false, false
			}
			current = current.Elem()
		}
		if !current.IsValid() || current.Kind() != reflect.Struct {
			return reflect.Value{}, false, false
		}
		wireField, found := contractshape.LookupField(current.Type(), segment)
		if !found {
			return reflect.Value{}, false, false
		}
		current = current.FieldByName(wireField.GoName)
		optional = wireField.Optional
	}
	return current, optional, current.IsValid()
}
