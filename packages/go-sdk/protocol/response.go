package protocol

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"reflect"
)

var ErrInvalidResponse = errors.New("runtime: invalid response")

// DecodeResponse preserves required fields, typed nulls and exact numbers before
// assigning a validated value. Nullable root results are admitted by method metadata.
func DecodeResponse(encoded []byte, target any) error {
	destination := reflect.ValueOf(target)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		return fmt.Errorf("%w: response target must be a non-nil pointer", ErrInvalidResponse)
	}
	value := reflect.New(destination.Type().Elem())
	if err := jsonvalue.DecodeValue(jsontext.Value(encoded), value.Interface(), "result"); err != nil {
		return errors.Join(ErrInvalidResponse, err)
	}
	if err := ValidateWireTree(value.Elem().Interface()); err != nil {
		return errors.Join(ErrInvalidResponse, err)
	}
	destination.Elem().Set(value.Elem())
	return nil
}
