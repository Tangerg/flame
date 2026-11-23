package extensions

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
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

type Capability string

const HostNamespace = "terminal"

const CommandsCapability Capability = HostNamespace + ".commands"

func ReservedPluginID(id string) bool {
	return id == HostNamespace || strings.HasPrefix(id, HostNamespace+".")
}

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

type Plugin struct {
	ID           string
	Version      string
	APIVersion   int
	Requires     []string
	Capabilities []Capability
	Setup        func(*Scope) error
}

type loadedPlugin struct {
	once        sync.Once
	disposables []disposable
	err         error
}

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
