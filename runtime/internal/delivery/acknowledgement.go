package delivery

import (
	"fmt"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type AcknowledgementComparison uint8

const (
	AcknowledgementEcho AcknowledgementComparison = iota + 1
	AcknowledgementPresence
)

// AcknowledgementMatch binds two wire fields whose separate shapes cannot prove
// that the stream acknowledges the original request.
type AcknowledgementMatch struct {
	Request string
	Result  string
	Compare AcknowledgementComparison
}

func (m MethodMeta) validateAcknowledgement() error {
	if len(m.Acknowledgement) == 0 {
		return nil
	}
	if m.Kind != KindStream || m.Result == nil || m.ResultNullable {
		return fmt.Errorf("%s: acknowledgement matching requires a stream result", m.Name)
	}
	for _, match := range m.Acknowledgement {
		request, requestOK := protocol.WireFieldNamed(m.Params, match.Request)
		result, resultOK := protocol.WireFieldNamed(m.Result, match.Result)
		if !requestOK || !resultOK || !match.accepts(request, result) {
			return fmt.Errorf("%s: invalid acknowledgement match between request %q and result %q", m.Name, match.Request, match.Result)
		}
	}
	return nil
}

func (m AcknowledgementMatch) accepts(request, result protocol.WireField) bool {
	switch m.Compare {
	case AcknowledgementEcho:
		return !request.Optional && !result.Optional && request.Type.Kind() == reflect.String && request.Type == result.Type
	case AcknowledgementPresence:
		return request.Optional && result.Optional && result.Type.Kind() == reflect.Pointer &&
			(request.Type.Kind() == reflect.Bool || request.Type.Kind() == reflect.Slice)
	default:
		return false
	}
}
