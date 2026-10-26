package runtime

import (
	"context"
	"iter"
	"reflect"
	"testing"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestPublicMethodsCoverExactOperationContract(t *testing.T) {
	errorType := reflect.TypeFor[error]()
	contextType := reflect.TypeFor[context.Context]()
	for _, meta := range delivery.Contract().Metas() {
		methodName := meta.GoName
		if methodName == "" {
			t.Errorf("operation %q has no public Runtime method", meta.Name)
			continue
		}
		for _, bindingType := range []reflect.Type{reflect.TypeFor[*Runtime](), reflect.TypeFor[*runtimeclient.Client]()} {
			method, ok := bindingType.MethodByName(methodName)
			if !ok {
				t.Errorf("Runtime method %s is not exported", methodName)
				continue
			}
			if method.Type.In(1) != contextType {
				t.Errorf("%s context parameter = %s", methodName, method.Type.In(1))
			}
			expectedInputs := 4
			if meta.Params == reflect.TypeFor[struct{}]() {
				expectedInputs = 3
			}
			if method.Type.NumIn() != expectedInputs {
				t.Errorf("%s inputs = %d, want %d", methodName, method.Type.NumIn(), expectedInputs)
				continue
			}
			requestIndex := 2
			if meta.Params == reflect.TypeFor[struct{}]() {
				requestIndex = -1
			} else if method.Type.In(2) != meta.Params {
				t.Errorf("%s request = %s, want %s", methodName, method.Type.In(2), meta.Params)
			}
			optionIndex := 2
			if requestIndex >= 0 {
				optionIndex = 3
			}
			if got, want := method.Type.In(optionIndex), optionType(meta); got != want {
				t.Errorf("%s options = %s, want %s", methodName, got, want)
			}

			if meta.Kind == delivery.KindStream {
				if method.Type.NumOut() != 3 || method.Type.Out(0) != meta.Result || method.Type.Out(2) != errorType {
					t.Errorf("%s stream result does not match %s", methodName, meta.Name)
					continue
				}
				assertIteratorEvent(t, methodName, method.Type.Out(1), meta.Event)
				continue
			}
			if meta.Result == nil {
				if method.Type.NumOut() != 1 || method.Type.Out(0) != errorType {
					t.Errorf("%s acknowledgement result does not match %s", methodName, meta.Name)
				}
				continue
			}
			if method.Type.NumOut() != 2 || method.Type.Out(0) != meta.Result || method.Type.Out(1) != errorType {
				t.Errorf("%s result does not match %s", methodName, meta.Name)
			}
		}
	}
}

func optionType(meta delivery.MethodMeta) reflect.Type {
	switch {
	case meta.Operation == delivery.OperationExtension:
		return reflect.TypeFor[protocol.PluginCallOptions]()
	case meta.Kind == delivery.KindStream && meta.Operation == delivery.OperationCommand && meta.ReplayCursor == delivery.ReplayCursorRun:
		return reflect.TypeFor[protocol.RunCommandOptions]()
	case meta.Operation == delivery.OperationCommand:
		return reflect.TypeFor[protocol.CommandOptions]()
	case meta.ReplayCursor == delivery.ReplayCursorRun:
		return reflect.TypeFor[protocol.RunSubscriptionOptions]()
	case meta.Kind == delivery.KindStream:
		return reflect.TypeFor[protocol.SubscriptionOptions]()
	default:
		return reflect.TypeFor[protocol.CallOptions]()
	}
}

func assertIteratorEvent(t *testing.T, methodName string, iterator, event reflect.Type) {
	t.Helper()
	want := reflect.TypeFor[iter.Seq2[struct{}, error]]()
	if iterator.Kind() != want.Kind() || iterator.NumIn() != 1 {
		t.Errorf("%s stream = %s, want iter.Seq2", methodName, iterator)
		return
	}
	yield := iterator.In(0)
	if yield.Kind() != reflect.Func || yield.NumIn() != 2 || yield.In(0) != event || yield.In(1) != reflect.TypeFor[error]() {
		t.Errorf("%s iterator event = %s, want %s", methodName, yield, event)
	}
}
