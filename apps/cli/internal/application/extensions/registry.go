// Package extensions owns discovered slash commands and their plugin lifetimes.
package extensions

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
)

var (
	errScopeClosed                   = errors.New("extensions: plugin scope is closed")
	errRegistrationSequenceExhausted = errors.New("extensions: registration sequence is exhausted")
)

type registrationSequence uint64

func (s registrationSequence) successor() (registrationSequence, bool) {
	if s == registrationSequence(math.MaxUint64) {
		return 0, false
	}
	return s + 1, true
}

// Capability names a permission explicitly declared by an executable plugin.
type Capability string

const CommandsCapability Capability = "terminal.commands"

type commandRegistry struct {
	mu      sync.RWMutex
	entries map[string]commandEntry
	plugins map[string]registrationSequence
	next    registrationSequence
}
type commandEntry struct {
	plugin  string
	seq     registrationSequence
	command SlashCommand
}
type disposable interface{ dispose() error }

type disposal struct {
	once sync.Once
	do   func() error
	err  error
}

func (d *disposal) dispose() error {
	d.once.Do(func() { d.err = d.do() })
	return d.err
}

// Scope owns command registrations made during one plugin setup transaction.
// The scope is sealed when setup returns; plugins cannot attach ownership to a
// completed or rolling-back installation transaction.
type Scope struct {
	mu              sync.Mutex
	plugin          string
	registry        *commandRegistry
	commandsAllowed bool
	disposables     []disposable
	open            bool
}

func (s *Scope) seal() []disposable {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = false
	return slices.Clone(s.disposables)
}

// Plugin is one cohesive set of contributions.
type Plugin struct {
	ID           string
	Version      string
	APIVersion   int
	Requires     []string
	Capabilities []Capability
	Setup        func(*Scope) error
}

// loadedPlugin is a successfully initialized plugin. dispose unloads it in reverse
// registration order.
type loadedPlugin struct {
	once        sync.Once
	disposables []disposable
	err         error
}

// dispose unloads a plugin. Every cleanup runs even when another returns an
// error or panics; the joined result is stable across repeated calls.
func (l *loadedPlugin) dispose() error {
	l.once.Do(func() {
		var failures []error
		for _, disposable := range slices.Backward(l.disposables) {
			if err := disposeSafely(disposable); err != nil {
				failures = append(failures, err)
			}
		}
		l.err = errors.Join(failures...)
	})
	return l.err
}

func disposeSafely(disposable disposable) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin cleanup panicked: %v", recovered)
		}
	}()
	return disposable.dispose()
}

// load validates and initializes one plugin. Setup is transactional: a failure
// rolls back everything contributed before the error.
func load(registry *commandRegistry, plugin Plugin) (*loadedPlugin, error) {
	if registry == nil {
		return nil, errors.New("extensions: registry is required")
	}
	if err := ValidateManifest(plugin); err != nil {
		return nil, err
	}
	release, err := registry.claim(plugin.ID)
	if err != nil {
		return nil, err
	}
	scope := &Scope{
		plugin: plugin.ID, registry: registry,
		commandsAllowed: slices.Contains(plugin.Capabilities, CommandsCapability),
		disposables:     []disposable{release},
		open:            true,
	}
	setupErr := setupSafely(plugin.Setup, scope)
	disposables := scope.seal()
	if setupErr != nil {
		loaded := &loadedPlugin{disposables: disposables}
		if rollbackErr := loaded.dispose(); rollbackErr != nil {
			setupErr = errors.Join(setupErr, fmt.Errorf("rollback plugin %q: %w", plugin.ID, rollbackErr))
		}
		return nil, fmt.Errorf("load plugin %q: %w", plugin.ID, setupErr)
	}
	return &loadedPlugin{disposables: disposables}, nil
}

func setupSafely(setup func(*Scope) error, scope *Scope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin setup panicked: %v", recovered)
		}
	}()
	return setup(scope)
}

func (r *commandRegistry) claim(plugin string) (disposable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[plugin]; exists {
		return nil, fmt.Errorf("extensions: plugin %q is already loaded", plugin)
	}
	next, ok := r.next.successor()
	if !ok {
		return nil, errRegistrationSequenceExhausted
	}
	if r.plugins == nil {
		r.plugins = make(map[string]registrationSequence)
	}
	r.next = next
	token := next
	r.plugins[plugin] = token
	return &disposal{do: func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.plugins[plugin] == token {
			delete(r.plugins, plugin)
		}
		return nil
	}}, nil
}

// ContributeCommand attaches one validated command to this plugin's lifetime.
func (s *Scope) ContributeCommand(command SlashCommand) error {
	if s == nil || s.registry == nil {
		return errors.New("extensions: plugin scope is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.open {
		return errScopeClosed
	}
	if !s.commandsAllowed {
		return fmt.Errorf("extensions: plugin %q needs capability %q", s.plugin, CommandsCapability)
	}
	if err := command.Validate(); err != nil {
		return err
	}
	sequence, err := s.registry.insert(s.plugin, command)
	if err != nil {
		return err
	}
	s.disposables = append(s.disposables, &disposal{do: func() error {
		s.registry.remove(command.Descriptor.Name, sequence)
		return nil
	}})
	return nil
}

func (r *commandRegistry) insert(plugin string, command SlashCommand) (registrationSequence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, identity := range command.Descriptor.Identities() {
		for _, previous := range r.entries {
			if slices.Contains(previous.command.Descriptor.Identities(), identity) {
				return 0, fmt.Errorf("extensions: plugin %q cannot contribute command identity %q; owned by %q", plugin, identity, previous.plugin)
			}
		}
	}
	sequence, ok := r.next.successor()
	if !ok {
		return 0, errRegistrationSequenceExhausted
	}
	if r.entries == nil {
		r.entries = make(map[string]commandEntry)
	}
	command.Descriptor.Aliases = slices.Clone(command.Descriptor.Aliases)
	r.entries[command.Descriptor.Name] = commandEntry{plugin: plugin, seq: sequence, command: command}
	r.next = sequence
	return sequence, nil
}

func (r *commandRegistry) remove(name string, sequence registrationSequence) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, exists := r.entries[name]; exists && current.seq == sequence {
		delete(r.entries, name)
	}
}

// OwnedCommand retains the provider identity needed to cancel execution before unload.
type OwnedCommand struct {
	PluginID string
	Command  SlashCommand
}

func (r *commandRegistry) commands() []OwnedCommand {
	r.mu.RLock()
	entries := make([]commandEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		entries = append(entries, entry)
	}
	r.mu.RUnlock()
	slices.SortFunc(entries, func(a, b commandEntry) int { return cmp.Compare(a.seq, b.seq) })
	result := make([]OwnedCommand, 0, len(entries))
	for _, entry := range entries {
		command := entry.command
		command.Descriptor.Aliases = slices.Clone(command.Descriptor.Aliases)
		result = append(result, OwnedCommand{PluginID: entry.plugin, Command: command})
	}
	return result
}
