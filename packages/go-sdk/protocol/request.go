package protocol

import "fmt"

func DecodeRequest(encoded []byte, target any) error {
	if err := DecodeWire(encoded, target); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidParams, err)
	}
	return nil
}
