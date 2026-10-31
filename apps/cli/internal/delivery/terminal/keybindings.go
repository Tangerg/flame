package terminal

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
)

type keyBindings struct {
	editor      *keymap.Map
	application *keymap.Map
	global      *keymap.Map
}

func configuredKeyBindings(configured settings.Config) (keyBindings, error) {
	bindings := keyBindings{
		editor:      headless.DefaultEditorKeys(),
		application: &keymap.Map{},
		global:      &keymap.Map{},
	}
	actions := slices.Sorted(maps.Keys(configured.Keys))
	seen := make(map[string]settings.Action)
	for _, name := range actions {
		action := keymap.Action(name)
		for _, encoded := range configured.Keys[name] {
			sequence, ok := input.ParseKeys(encoded)
			if !ok {
				return keyBindings{}, fmt.Errorf("session settings: keys.%s contains invalid binding %q", name, encoded)
			}
			identity := sequence.String()
			if owner, duplicate := seen[identity]; duplicate && owner != name {
				return keyBindings{}, fmt.Errorf("session settings: key %q is bound to both %s and %s", identity, owner, name)
			}
			seen[identity] = name
			bindings.editor.Bind(action, sequence...)
			bindings.application.Bind(action, sequence...)
			if action == cancelRun || action == quitApp {
				bindings.global.Bind(action, sequence...)
			}
		}
	}
	return bindings, nil
}

func (k keyBindings) setResolver(resolve keymap.Resolver) {
	for _, keys := range []*keymap.Map{k.editor, k.application, k.global} {
		if keys != nil {
			keys.Resolve = resolve
		}
	}
}

// keyRouter matches key sequences against the application bindings, or only
// the global ones while a modal is open. Each map keeps its own partial
// sequence, so an application prefix cannot swallow form input.
type keyRouter struct {
	application        *keymap.Map
	global             *keymap.Map
	applicationPending keymap.Matcher
	globalPending      keymap.Matcher
}

// route returns the hint for an application sequence still in progress; it is
// empty while a modal is open.
func (k *keyRouter) route(key input.Key, modal bool, handle func(keymap.Action) bool) (matched, handled bool, pending string) {
	if modal {
		k.applicationPending.Clear()
		matched, handled = k.globalPending.Handle(k.global, key, handle)
		return matched, handled, ""
	}
	k.globalPending.Clear()
	matched, handled = k.applicationPending.Handle(k.application, key, handle)
	return matched, handled, pendingKeySequenceHint(k.application, k.applicationPending.Keys())
}

func formatKeyBindings(keys *keymap.Map, action keymap.Action, separator string) string {
	sequences := keys.Keys(action)
	names := make([]string, len(sequences))
	for i, sequence := range sequences {
		names[i] = sequence.String()
	}
	return strings.Join(names, separator)
}

func pendingKeySequenceHint(keys *keymap.Map, prefix input.Keys) string {
	if keys == nil || len(prefix) == 0 {
		return ""
	}
	var continuations []string
	for _, binding := range keys.Bindings() {
		if len(binding.Keys) <= len(prefix) || !slices.Equal(binding.Keys[:len(prefix)], prefix) {
			continue
		}
		continuations = append(continuations, binding.Keys[len(prefix):].String()+" "+binding.Action.String())
	}
	slices.Sort(continuations)
	if len(continuations) == 0 {
		return ""
	}
	return prefix.String() + " → " + strings.Join(continuations, " · ")
}
