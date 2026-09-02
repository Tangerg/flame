package maintenance

import "reflect"

// nilDependency closes Go's typed-nil interface hole at worker construction.
// Policy gates can delay a worker's first dependency call well past bootstrap,
// making a late panic both difficult to attribute and impossible to recover.
func nilDependency(value any) bool {
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
