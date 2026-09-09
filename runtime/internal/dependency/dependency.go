// Package dependency answers the one question Go's type system cannot: whether
// an interface-typed collaborator is actually there.
package dependency

import "reflect"

// Missing reports whether value is absent, including the typed nil an interface
// field cannot express. A nil concrete pointer assigned to an interface arrives
// as a non-nil interface, so a constructor comparing against nil admits it and
// the first call panics — often long after composition, where policy gates can
// delay a collaborator's first use past bootstrap and make the failure both
// hard to attribute and impossible to recover.
//
// The kinds below are exactly those that carry a nil value. Anything else is
// present by construction.
func Missing(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
