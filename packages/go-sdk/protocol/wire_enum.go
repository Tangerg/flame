package protocol

import "reflect"

type wireEnum interface {
	wireEnumValues() []string
}

func WireEnumValues(owner reflect.Type) ([]string, bool) {
	if owner == nil || owner.Kind() != reflect.String {
		return nil, false
	}
	switch enum := reflect.Zero(owner).Interface().(type) {
	case interface{ Values() []string }:
		return enum.Values(), true
	case wireEnum:
		return enum.wireEnumValues(), true
	default:
		return nil, false
	}
}
