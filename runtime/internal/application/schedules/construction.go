package schedules

import "reflect"

// dependencyMissing closes Go's typed-nil interface hole at construction. A
// dependency whose dynamic value is a nil pointer is just as absent as a nil
// interface and must not survive into a long-lived component.
func dependencyMissing(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
