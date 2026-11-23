package protocol

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrInvalidResponse = errors.New("runtime: invalid response")

func DecodeResponse(encoded []byte, target any) error {
	destination := reflect.ValueOf(target)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		return fmt.Errorf("%w: response target must be a non-nil pointer", ErrInvalidResponse)
	}
	value := reflect.New(destination.Type().Elem())
	if err := DecodeWire(encoded, value.Interface()); err != nil {
		return errors.Join(ErrInvalidResponse, err)
	}
	if err := ValidateWireTree(value.Elem().Interface()); err != nil {
		return errors.Join(ErrInvalidResponse, err)
	}
	destination.Elem().Set(value.Elem())
	return nil
}
