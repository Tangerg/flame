package settings

import (
	"strings"
	"testing"
)

func TestDefaultIsValidAndCloned(t *testing.T) {
	defaults := Default()
	if err := defaults.Validate(); err != nil {
		t.Fatalf("Default.Validate: %v", err)
	}
	clone := defaults.Clone()
	clone.Keys[ActionQuit][0] = "q"
	defaults.Plugins.Directories = []string{"one"}
	clone = defaults.Clone()
	clone.Plugins.Directories[0] = "two"
	if defaults.Keys[ActionQuit][0] != "ctrl+q" {
		t.Fatal("Clone leaked a keybinding slice")
	}
	if defaults.Plugins.Directories[0] != "one" {
		t.Fatal("Clone leaked a plugin directory slice")
	}
	limited := Default()
	limited.Run.MaxSteps = new(9)
	clone = limited.Clone()
	*clone.Run.MaxSteps = 10
	if *limited.Run.MaxSteps != 9 {
		t.Fatal("Clone leaked a run limit pointer")
	}
	if options, err := defaults.RunOptions(); err != nil || options.Provider != "" || options.Model != "" || !options.Limits.Unlimited() {
		t.Fatalf("RunOptions = %+v", options)
	}
	if defaults.Approval.Remember != RememberNone {
		t.Fatalf("default approval remember = %q", defaults.Approval.Remember)
	}
	if got := defaults.Keys[ActionManageQueue]; len(got) != 1 || got[0] != "ctrl+;" {
		t.Fatalf("manage queue bindings = %v", got)
	}
	if got := defaults.Keys[ActionTimeline]; len(got) != 1 || got[0] != "ctrl+g" {
		t.Fatalf("timeline bindings = %v", got)
	}
	if got := defaults.Keys[ActionShortcuts]; len(got) != 1 || got[0] != "ctrl+x" {
		t.Fatalf("shortcut bindings = %v", got)
	}
	if got := defaults.Keys[ActionCancelRun]; len(got) != 1 || got[0] != "ctrl+c" {
		t.Fatalf("cancel bindings = %v", got)
	}
}

func TestValidationReportsAllIndependentProblems(t *testing.T) {
	settings := Default()
	settings.Provider = "mock"
	settings.Model = ""
	settings.Run.MaxSteps = new(-1)
	settings.UI.TranscriptRetain = 0
	settings.Plugins.Directories = []string{"", "/plugins", "/plugins"}
	delete(settings.Keys, ActionShortcuts)
	settings.Keys["unknown"] = []string{""}
	err := settings.Validate()
	if err == nil {
		t.Fatal("invalid settings were accepted")
	}
	for _, want := range []string{"provider and model must be set together", "positive", "transcript-retain", "empty path", "repeats", "shortcuts is missing", "unknown", "empty binding"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validation error %q does not mention %q", err, want)
		}
	}
}
