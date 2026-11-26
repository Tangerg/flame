package protocol

import "errors"

var ErrInvalidResponse = errors.New("runtime: invalid response")

func DecodeResponse(encoded []byte, target any) error {
	if err := DecodeWire(encoded, target); err != nil {
		return errors.Join(ErrInvalidResponse, err)
	}
	return nil
}
