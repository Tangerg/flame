package protocol

import "fmt"

func DecodeRequest(encoded []byte, target any) (WireMembers, error) {
	members, err := decodeWire(encoded, target)
	if err != nil {
		return WireMembers{}, fmt.Errorf("%w: %w", ErrInvalidParams, err)
	}
	return members, nil
}
