package dispatch

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/go-sdk/jsonrpc"
)

func splitRequestMetadata(encoded jsontext.Value) (jsontext.Value, protocol.RequestMeta, *jsonrpc.Error) {
	if encoded.Kind() != jsontext.KindBeginObject {
		return encoded, protocol.RequestMeta{}, nil
	}
	var parameters map[string]jsontext.Value
	if err := json.Unmarshal(encoded, &parameters); err != nil {
		return nil, protocol.RequestMeta{}, invalidParams(err.Error())
	}
	rawMetadata, ok := parameters[protocol.RequestMetaMember]
	if !ok {
		return encoded, protocol.RequestMeta{}, nil
	}
	var metadata protocol.RequestMeta
	if _, err := protocol.DecodeRequest(rawMetadata, &metadata); err != nil {
		return nil, protocol.RequestMeta{}, errorToRPC(fmt.Errorf("%s: %w", protocol.RequestMetaMember, err))
	}
	delete(parameters, protocol.RequestMetaMember)
	if len(parameters) == 0 {
		return nil, metadata, nil
	}
	methodParameters, err := json.Marshal(parameters, json.Deterministic(true))
	if err != nil {
		return nil, protocol.RequestMeta{}, invalidParams(protocol.RequestMetaMember + ": " + err.Error())
	}
	return methodParameters, metadata, nil
}
