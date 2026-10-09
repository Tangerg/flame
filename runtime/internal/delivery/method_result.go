package delivery

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ValueAs restores the declared Go type after validation. An opaque JSON null
// is a valid interface result even though an assertion on a nil interface fails.
func (r Result) ValueAs[Response any]() (Response, bool) {
	var zero Response
	if r.Value == nil && reflect.TypeFor[Response]().Kind() == reflect.Interface {
		return zero, true
	}
	value, ok := r.Value.(Response)
	return value, ok
}

// ValidateResult applies the catalog's result contract to an in-process value.
func (m MethodMeta) ValidateResult(value any) error {
	expected := m.resultType()
	if expected.Kind() == reflect.Interface {
		if actual := reflect.TypeOf(value); actual != nil && !actual.Implements(expected) {
			return fmt.Errorf("%s: result has type %v, want %v", m.Name, actual, expected)
		}
		return nil
	}
	if actual := reflect.TypeOf(value); actual != expected {
		return fmt.Errorf("%s: result has type %v, want %v", m.Name, actual, expected)
	}
	if expected.Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil() && !m.ResultNullable {
		return fmt.Errorf("%s: result must not be null", m.Name)
	}
	return protocol.ValidateWireTree(value)
}

// DecodeResult preserves required fields and nullability before typed decoding
// can erase them. Stored receipts and remote replies use this same contract.
func (m MethodMeta) DecodeResult(encoded jsontext.Value) (any, error) {
	if len(encoded) == 0 {
		return nil, errors.New("result is absent")
	}
	if m.ResultNullable && bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return reflect.Zero(m.Result).Interface(), nil
	}
	target := reflect.New(m.resultType())
	if err := contractshape.DecodeValue(encoded, target.Interface(), "result"); err != nil {
		return nil, err
	}
	value := target.Elem().Interface()
	if err := m.ValidateResult(value); err != nil {
		return nil, err
	}
	return value, nil
}

func (m MethodMeta) resultType() reflect.Type {
	if m.Result == nil {
		return reflect.TypeFor[struct{}]()
	}
	return m.Result
}
