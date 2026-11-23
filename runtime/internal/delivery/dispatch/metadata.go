package dispatch

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/go-sdk/jsonrpc"
)

func extractRequestMeta(request *jsonrpc.Request) (protocol.RequestMeta, *jsonrpc.Error) {
	if request == nil || request.Params.Kind() != jsontext.KindBeginObject {
		return protocol.RequestMeta{}, nil
	}
	var parameters map[string]jsontext.Value
	if err := json.Unmarshal(request.Params, &parameters); err != nil {
		return protocol.RequestMeta{}, invalidParams(err.Error())
	}
	encoded, ok := parameters[protocol.RequestMetaMember]
	if !ok {
		return protocol.RequestMeta{}, nil
	}
	var metadata protocol.RequestMeta
	if err := decodeParams(encoded, &metadata); err != nil {
		return protocol.RequestMeta{}, errorToRPC(fmt.Errorf("%s: %w", protocol.RequestMetaMember, err))
	}
	delete(parameters, protocol.RequestMetaMember)
	if len(parameters) == 0 {
		request.Params = nil
		return metadata, nil
	}
	encodedParameters, err := json.Marshal(parameters, json.Deterministic(true))
	if err != nil {
		return protocol.RequestMeta{}, invalidParams(protocol.RequestMetaMember + ": " + err.Error())
	}
	request.Params = encodedParameters
	return metadata, nil
}
