package extensions

import (
	"strings"
	"testing"
)

func TestCommandDescriptorValidatesItsIdentityNamespace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		descriptor CommandDescriptor
		want       string
	}{
		{name: "valid", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"look"}}},
		{name: "missing name", descriptor: CommandDescriptor{Title: "inspect workspace"}, want: "has no name"},
		{name: "invalid name", descriptor: CommandDescriptor{Name: "in spect", Title: "inspect workspace"}, want: "invalid name"},
		{name: "missing title", descriptor: CommandDescriptor{Name: "inspect"}, want: "has no title"},
		{name: "invalid arguments", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Arguments: ArgumentMode("invalid")}, want: "argument mode"},
		{name: "invalid alias", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"bad alias"}}, want: "invalid alias"},
		{name: "duplicate alias", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"look", "look"}}, want: "repeats name or alias"},
		{name: "alias repeats name", descriptor: CommandDescriptor{Name: "inspect", Title: "inspect workspace", Aliases: []string{"inspect"}}, want: "repeats name or alias"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.descriptor.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestArgumentModeValidatesInvocations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mode     ArgumentMode
		argument string
		want     string
	}{
		{name: "none empty", mode: NoArguments},
		{name: "none populated", mode: NoArguments, argument: "surprise", want: "does not accept"},
		{name: "optional empty", mode: OptionalArguments},
		{name: "optional populated", mode: OptionalArguments, argument: "value"},
		{name: "required empty", mode: RequiredArguments, want: "needs an argument"},
		{name: "required populated", mode: RequiredArguments, argument: "value"},
		{name: "invalid", mode: ArgumentMode("invalid"), want: "invalid argument contract"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.mode.ValidateInvocation("inspect", test.argument)
			if test.want == "" && err != nil {
				t.Fatalf("ValidateInvocation() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateInvocation() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

// TestCommandAvailabilityCannotSayOneThingAndMeanAnother pins the pairing the
// two-field shape used to allow: an enabled command carrying a reason it is
// not, and a disabled one carrying nothing to show the operator.
func TestCommandAvailabilityCannotSayOneThingAndMeanAnother(t *testing.T) {
	available := CommandAvailable()
	if !available.Enabled() || available.Reason() != "" {
		t.Fatalf("available = (%v, %q)", available.Enabled(), available.Reason())
	}
	for _, reason := range []string{"", "   ", "\n\t"} {
		blank := CommandUnavailable(reason)
		if blank.Enabled() || blank.Reason() != "not available in the current context" {
			t.Fatalf("CommandUnavailable(%q) = (%v, %q)", reason, blank.Enabled(), blank.Reason())
		}
	}
	stated := CommandUnavailable("  an active run owns this session  ")
	if stated.Enabled() || stated.Reason() != "an active run owns this session" {
		t.Fatalf("stated = (%v, %q)", stated.Enabled(), stated.Reason())
	}
}
