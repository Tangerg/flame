package protocol

import (
	"errors"
	"fmt"
	"reflect"
)

// DecodeRequest preserves authored presence, typed nulls, and exact numbers.
// target must be a non-nil pointer; it changes only after complete validation.
func DecodeRequest(encoded []byte, target any) error {
	destination := reflect.ValueOf(target)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		return fmt.Errorf("%w: request target must be a non-nil pointer", ErrInvalidParams)
	}
	request := reflect.New(destination.Type().Elem())
	if err := DecodeWire(encoded, request.Interface()); err != nil {
		if shape, ok := errors.AsType[*ConstraintError](err); ok {
			shape.Shape = request.Elem().Type().Name()
		}
		return fmt.Errorf("%w: %w", ErrInvalidParams, err)
	}
	if err := ValidateWireTree(request.Interface()); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidParams, err)
	}
	destination.Elem().Set(request.Elem())
	return nil
}
