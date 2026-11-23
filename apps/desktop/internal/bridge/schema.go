package bridge

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
)

type RuleKind string

const (
	Nonempty    RuleKind = "nonempty"
	Nonnegative RuleKind = "nonnegative"
	URL         RuleKind = "url"
)

type Rule struct {
	Type  string
	Field string
	Kind  RuleKind
}

func Rules() []Rule {
	chrome, connection, notification := typeName[WindowChrome](), typeName[LocalRuntimeConnection](), typeName[DesktopNotification]()
	return []Rule{
		{Type: chrome, Field: "ControlsCentreY", Kind: Nonnegative},
		{Type: chrome, Field: "ControlsInlineEnd", Kind: Nonnegative},
		{Type: connection, Field: "Endpoint", Kind: URL},
		{Type: connection, Field: "LocalToken", Kind: Nonempty},
		{Type: notification, Field: "ID", Kind: Nonempty},
		{Type: notification, Field: "Title", Kind: Nonempty},
		{Type: typeName[NotificationTarget](), Kind: Nonempty},
	}
}

func typeName[T any]() string { return reflect.TypeFor[T]().Name() }

func validate(value any) error {
	root := reflect.ValueOf(value)
	for _, rule := range Rules() {
		if rule.Type != root.Type().Name() {
			continue
		}
		field := root
		if rule.Field != "" {
			field = root.FieldByName(rule.Field)
			if !field.IsValid() {
				return fmt.Errorf("bridge: %s has no %s field", rule.Type, rule.Field)
			}
		}
		valid := false
		switch rule.Kind {
		case Nonempty:
			valid = field.Kind() == reflect.String && field.Len() > 0
		case Nonnegative:
			valid = field.Kind() == reflect.Float64 && field.Float() >= 0 && !math.IsInf(field.Float(), 0) && !math.IsNaN(field.Float())
		case URL:
			if field.Kind() == reflect.String {
				parsed, err := url.Parse(field.String())
				valid = err == nil && parsed.IsAbs()
			}
		}
		if !valid {
			return fmt.Errorf("bridge: %s.%s violates %s", rule.Type, rule.Field, rule.Kind)
		}
	}
	return nil
}
