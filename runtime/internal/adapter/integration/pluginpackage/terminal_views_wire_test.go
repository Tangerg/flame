package pluginpackage

import (
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

func TestTerminalBindingsNameExactlyOneSource(t *testing.T) {
	for name, test := range map[string]struct {
		raw    string
		source extension.TerminalBindingSource
	}{
		"value":   {raw: `{"value":"project"}`, source: extension.TerminalBindingValue},
		"row":     {raw: `{"row":"ID"}`, source: extension.TerminalBindingRow},
		"context": {raw: `{"context":"workspace"}`, source: extension.TerminalBindingContext},
		"form":    {raw: `{"form":{"title":"Content","default":{"row":"Content"}}}`, source: extension.TerminalBindingForm},
		"none":    {raw: `{}`},
		"two":     {raw: `{"value":1,"row":"ID"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var wire wireBinding
			if err := json.Unmarshal([]byte(test.raw), &wire); err != nil {
				t.Fatal(err)
			}
			binding, err := wire.domain()
			if test.source == "" {
				if err == nil {
					t.Fatalf("binding %s named no single source and was accepted", test.raw)
				}
				return
			}
			if err != nil || binding.Source != test.source {
				t.Fatalf("binding %s = %+v, %v; want %s", test.raw, binding, err, test.source)
			}
		})
	}
}
