package protocol

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/flame/runtime/internal/contractshape"
)

// DecodeRequest preserves authored presence, typed nulls, and exact numbers.
// target must be a non-nil pointer; it changes only after complete validation.
func DecodeRequest(encoded []byte, target any) error {
	destination := reflect.ValueOf(target)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		return fmt.Errorf("%w: request target must be a non-nil pointer", ErrInvalidParams)
	}
	request := reflect.New(destination.Type().Elem())
	if err := contractshape.DecodeValue(jsontext.Value(encoded), request.Interface(), "params"); err != nil {
		if shape, ok := errors.AsType[*contractshape.ShapeError](err); ok {
			err = &ConstraintError{Shape: request.Elem().Type().Name(), Fields: []FieldError{{Field: shape.Field, Detail: shape.Detail}}}
		}
		return fmt.Errorf("%w: %w", ErrInvalidParams, err)
	}
	if err := ValidateWireTree(request.Interface()); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidParams, err)
	}
	destination.Elem().Set(request.Elem())
	return nil
}
