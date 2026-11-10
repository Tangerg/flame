package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
)

func TestConfiguredKeysBindProductActions(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionSessions] = []string{"g s"}
	bindings, err := configuredKeyBindings(configured)
	if err != nil {
		t.Fatal(err)
	}
	var matcher keymap.Matcher
	var got keymap.Action
	for _, chord := range []input.Chord{{Rune: 'g', Code: input.Character}, {Rune: 's', Code: input.Character}} {
		matcher.Handle(bindings.application, input.Key{Code: chord.Code, Rune: chord.Rune}, nil, func(action keymap.Action) bool {
			got = action
			return true
		})
	}
	if got != showSessions {
		t.Fatalf("sequence resolved to %q, want %q", got, showSessions)
	}
}

func TestPendingKeySequenceHintShowsOnlyValidContinuations(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionSessions] = []string{"g s"}
	configured.Keys[settings.ActionShortcuts] = []string{"g ?"}
	bindings, err := configuredKeyBindings(configured)
	if err != nil {
		t.Fatal(err)
	}
	hint := pendingKeySequenceHint(bindings.application, input.Keys{{Code: input.Character, Rune: 'g'}})
	if !strings.Contains(hint, "g →") || !strings.Contains(hint, "s sessions") || !strings.Contains(hint, "? shortcuts") {
		t.Fatalf("pending hint = %q", hint)
	}
	if strings.Contains(hint, "send") {
		t.Fatalf("pending hint includes an invalid continuation: %q", hint)
	}
}

func TestConfiguredKeysExposeMultilineAndTranscriptNavigation(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		chord input.Chord
		want  keymap.Action
	}{
		{chord: input.Shift.With(input.Enter), want: base.InsertNewline},
		{chord: input.Chord{Code: input.PageUp}, want: scrollPageUp},
		{chord: input.Chord{Code: input.PageDown}, want: scrollPageDown},
		{chord: input.Ctrl.With(input.Home), want: scrollTop},
		{chord: input.Ctrl.With(input.End), want: scrollBottom},
		{chord: input.Ctrl.Rune(';'), want: manageQueue},
		{chord: input.Ctrl.Rune('g'), want: showTimeline},
		{chord: input.Ctrl.Rune('x'), want: showShortcuts},
	}
	for _, test := range tests {
		got, ok := bindings.editor.Action(test.chord)
		if !ok || got != test.want {
			t.Errorf("binding %s = %q, %v; want %q", test.chord, got, ok, test.want)
		}
	}
	if _, ok := bindings.global.Action(input.Ctrl.Rune('r')); ok {
		t.Fatal("session switching leaked into the modal-global key scope")
	}
	if got, ok := bindings.global.Action(input.Ctrl.Rune('c')); !ok || got != cancelRun {
		t.Fatalf("global cancel binding = %q, %v", got, ok)
	}
}

func TestConfiguredKeysReplaceToolkitDefaults(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionNewline] = []string{"ctrl+enter"}
	bindings, err := configuredKeyBindings(configured)
	if err != nil {
		t.Fatal(err)
	}
	for _, keys := range []*keymap.Map{bindings.editor, bindings.application} {
		if action, ok := keys.Action(input.Ctrl.With(input.Enter)); !ok || action != base.InsertNewline {
			t.Fatalf("configured newline = %q, %v", action, ok)
		}
		for _, chord := range []input.Chord{input.Shift.With(input.Enter), input.Alt.With(input.Enter)} {
			if action, ok := keys.Action(chord); ok {
				t.Errorf("replaced shortcut %s still resolves to %q", chord, action)
			}
		}
		if got := formatKeyBindings(keys, base.InsertNewline, " / "); got != "ctrl+enter" {
			t.Errorf("configured newline hint = %q", got)
		}
	}
}

func TestConfiguredKeysRejectInvalidAndDuplicateBindings(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionQuit] = []string{"not-a-key"}
	if _, err := configuredKeyBindings(configured); err == nil || !strings.Contains(err.Error(), "invalid binding") {
		t.Fatalf("invalid binding error = %v", err)
	}
	configured = settings.Default()
	configured.Keys[settings.ActionQuit] = []string{"ctrl+r"}
	if _, err := configuredKeyBindings(configured); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("duplicate binding error = %v", err)
	}
}
