package delivery

import (
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

// TestRuntimeEventCloneOwnsEveryReferenceField walks the struct instead of
// listing its fields. One hub publication reaches every subscription, so a
// reference the clone forgets is shared state between independent consumers —
// and the way that happens is a new field arriving after the clone was written.
// Enumerating the fields here would go stale in exactly the same way.
func TestRuntimeEventCloneOwnsEveryReferenceField(t *testing.T) {
	value := reflect.ValueOf(&protocol.RuntimeEvent{}).Elem()
	populated := 0
	for index := range value.NumField() {
		field := value.Field(index)
		switch field.Kind() {
		case reflect.Slice:
			field.Set(reflect.MakeSlice(field.Type(), 1, 1))
			populated++
		case reflect.Pointer:
			field.Set(reflect.New(field.Type().Elem()))
			populated++
		case reflect.Map, reflect.Func, reflect.Chan:
			t.Fatalf("field %s is a %s; extend this test before adding one",
				value.Type().Field(index).Name, field.Kind())
		}
	}
	if populated == 0 {
		t.Fatal("RuntimeEvent has no reference-typed fields; the clone would be pointless")
	}

	source := value.Interface().(protocol.RuntimeEvent)
	cloned := reflect.ValueOf(cloneRuntimeEvent(source))
	for index := range value.NumField() {
		name := value.Type().Field(index).Name
		original, copied := value.Field(index), cloned.Field(index)
		switch original.Kind() {
		case reflect.Slice, reflect.Pointer:
			if original.Pointer() == copied.Pointer() {
				t.Errorf("cloneRuntimeEvent shares %s with the producer", name)
			}
		}
	}
}
