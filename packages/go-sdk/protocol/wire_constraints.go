package protocol

import (
	json "encoding/json/v2"
	"fmt"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	framevalue "github.com/Tangerg/frame/value"
)

func nonEmptyText(field, value string) FieldError {
	if value == "" {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

type wireNumber interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func positiveNumber[Number wireNumeric](field string, value Number) FieldError {
	number := float64(value)
	if number <= 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return FieldError{Field: field, Detail: "must be finite and greater than zero"}
	}
	return FieldError{}
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

func nonEmptyItems[T any](field string, values []T) FieldError {
	if len(values) == 0 {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

func minItems[T any](field string, values []T, minimum int) FieldError {
	if len(values) < minimum {
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

func maxLength(field, value string, maximum int) FieldError {
	if utf8.RuneCountInString(value) > maximum {
		return FieldError{Field: field, Detail: fmt.Sprintf("must contain at most %d characters", maximum)}
	}
	return FieldError{}
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

func identityItems[Identity ~string](field string, values []Identity) FieldError {
	for index, value := range values {
		if violation := identity(fmt.Sprintf("%s[%d]", field, index), string(value)); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func textPatternItems[Identity ~string](field string, values []Identity, pattern string) FieldError {
	for index, value := range values {
		if violation := textPattern(
			fmt.Sprintf("%s[%d]", field, index),
			string(value),
			pattern,
		); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func maxPropertyNameLength[Value any](field string, values map[string]Value, maximum int) FieldError {
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if violation := maxLength(framevalue.MapPath(field, key), key, maximum); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func identityPropertyNames[Value any](field string, values map[string]Value) FieldError {
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if violation := identity(framevalue.MapPath(field, key), key); violation.Field != "" {
			return violation
		}
	}
	return FieldError{}
}

func textPrefix(field, value, prefix string) FieldError {
	if !strings.HasPrefix(value, prefix) {
		return FieldError{Field: field, Detail: fmt.Sprintf("must start with %q", prefix)}
	}
	return FieldError{}
}

func textPattern(field, value, pattern string) FieldError {
	matched, err := regexp.MatchString(pattern, value)
	if err != nil || !matched {
		return FieldError{Field: field, Detail: fmt.Sprintf("must match %q", pattern)}
	}
	return FieldError{}
}

func nonEmptyProperties[Value any](field string, values map[string]Value) FieldError {
	if len(values) == 0 {
		return FieldError{Field: field, Detail: "must not be empty"}
	}
	return FieldError{}
}

func uniqueItems[T any](field string, values []T) FieldError {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		encoded, err := json.Marshal(value, json.Deterministic(true))
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

func unionTag(field, value string, literals []string, pattern string) FieldError {
	if slices.Contains(literals, value) {
		return FieldError{}
	}
	if matched, err := regexp.MatchString(pattern, value); err == nil && matched {
		return FieldError{}
	}
	return FieldError{Field: field, Detail: "must be a known tag or match " + pattern}
}

func required(field string, object wireObject) FieldError {
	if !object.present(field) {
		return FieldError{Field: field, Detail: "is required"}
	}
	return FieldError{}
}

func requiredWhen(applies bool, field string, object wireObject) FieldError {
	if !applies {
		return FieldError{}
	}
	return required(field, object)
}

func requiredAny(fields []string, object wireObject) FieldError {
	for _, field := range fields {
		if object.present(field) {
			return FieldError{}
		}
	}
	return FieldError{
		Field:  strings.Join(fields, "|"),
		Detail: "at least one field is required",
	}
}

func forbiddenWhen(applies bool, field string, object wireObject) FieldError {
	if applies && object.present(field) {
		return FieldError{Field: field, Detail: "must not be present here"}
	}
	return FieldError{}
}

func allowedValuesWhen(applies bool, field string, object wireObject, allowed []string) FieldError {
	if !applies {
		return FieldError{}
	}
	actual := object.field(field)
	if !actual.IsValid() {
		return FieldError{}
	}
	if actual.Kind() != reflect.String || !slices.Contains(allowed, actual.String()) {
		return FieldError{Field: field, Detail: "must be one of " + strings.Join(allowed, ", ") + " here"}
	}
	return FieldError{}
}

func (o wireObject) equals(path, expected string) bool {
	field := o.field(path)
	return field.Kind() == reflect.String && field.String() == expected
}

func (o wireObject) matches(path, pattern string) bool {
	field := o.field(path)
	if field.Kind() != reflect.String {
		return false
	}
	matched, err := regexp.MatchString(pattern, field.String())
	return err == nil && matched
}
