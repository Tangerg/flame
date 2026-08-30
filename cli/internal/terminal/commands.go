package terminal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
)

type registeredCommand struct {
	category  string
	arguments ArgumentMode
	run       func(string)
	evaluate  func(*app) CommandAvailability
}

func (r registeredCommand) availability(host *app) (availability CommandAvailability) {
	if r.evaluate == nil {
		return CommandAvailability{Enabled: true}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			availability = CommandAvailability{Reason: fmt.Sprintf("availability check panicked: %v", recovered)}
		}
	}()
	availability = r.evaluate(host)
	availability.Reason = strings.TrimSpace(availability.Reason)
	if !availability.Enabled && availability.Reason == "" {
		availability.Reason = "not available in the current context"
	}
	return availability
}

type commandCatalog struct {
	index headless.Commands[registeredCommand]
}

func newCommandCatalog() commandCatalog { return commandCatalog{} }

func (c *commandCatalog) reset() {
	for _, found := range c.index.Find("") {
		c.index.Remove(found.Command.Name)
	}
}

func (c *commandCatalog) add(owner string, descriptor CommandDescriptor, run func(string), evaluate func(*app) CommandAvailability) error {
	for _, identity := range descriptor.identities() {
		if existing, _, found := c.index.Lookup(identity); found {
			return fmt.Errorf("plugin %s command /%s conflicts with /%s", owner, descriptor.Name, existing.Name)
		}
	}
	c.index.Add(headless.Command{
		Name: descriptor.Name, Title: descriptor.Title, Aliases: descriptor.Aliases,
	}, registeredCommand{
		category: descriptor.category(), arguments: descriptor.Arguments, run: run, evaluate: evaluate,
	})
	return nil
}

func (c *commandCatalog) find(query string) []headless.Found {
	found := c.index.Find(query)
	exact, _, ok := c.index.Lookup(query)
	if !ok {
		return found
	}
	for index := range found {
		if found[index].Command.Name != exact.Name {
			continue
		}
		if index > 0 {
			exactMatch := found[index]
			copy(found[1:index+1], found[:index])
			found[0] = exactMatch
		}
		return found
	}
	return append([]headless.Found{{Command: exact}}, found...)
}

func (c *commandCatalog) lookup(identity string) (headless.Command, bool) {
	command, _, found := c.index.Lookup(identity)
	return command, found
}

func (c *commandCatalog) used(name string) {
	c.index.Used(name)
}

func (c *commandCatalog) category(name string) string {
	_, command, found := c.index.Lookup(name)
	if !found {
		return ""
	}
	return command.category
}

func (c *commandCatalog) arguments(name string) ArgumentMode {
	_, command, found := c.index.Lookup(name)
	if !found {
		return NoArguments
	}
	return command.arguments
}

func (c *commandCatalog) availability(name string, host *app) CommandAvailability {
	_, command, found := c.index.Lookup(name)
	if !found {
		return CommandAvailability{Enabled: true}
	}
	return command.availability(host)
}

func (a *app) registerCommands() {
	a.commands.reset()
	for _, local := range builtinCommands() {
		command := local
		if err := command.validate(); err != nil {
			a.message(err.Error())
			continue
		}
		if err := a.commands.add("terminal", command.Descriptor,
			func(argument string) {
				if err := runLocalCommandSafely(command, a, argument); err != nil {
					a.message(err.Error())
				}
			},
			command.Available,
		); err != nil {
			a.message(err.Error())
		}
	}
	for _, contributed := range a.registry.OwnedValues(SlashCommands) {
		command := contributed.Value
		pluginID := contributed.PluginID
		if err := command.validate(); err != nil {
			a.message("plugin " + pluginID + ": " + err.Error())
			continue
		}
		var evaluate func(*app) CommandAvailability
		if command.Available != nil {
			evaluate = func(host *app) CommandAvailability {
				request := CommandRequest{Workspace: host.session.Workspace.Path, SessionID: host.session.ID}
				return command.Available(request)
			}
		}
		if err := a.commands.add(pluginID, command.Descriptor,
			func(argument string) { a.executeCommand(pluginID, command, argument) },
			evaluate,
		); err != nil {
			a.message(err.Error())
		}
	}
}

type commandOperation struct {
	id       commandOperationID
	pluginID string
}

const pluginCommandOperationSlotPrefix operationSlot = "command:"

var errCommandOperationIdentityExhausted = errors.New("plugin command operation identity space is exhausted")

type commandOperationID uint64

func (id commandOperationID) successor() (commandOperationID, bool) {
	if id == commandOperationID(math.MaxUint64) {
		return 0, false
	}
	return id + 1, true
}

func (o commandOperation) slot() operationSlot {
	return operationSlot(fmt.Sprintf("%s%d", pluginCommandOperationSlotPrefix, o.id))
}

type commandOperationRegistry struct {
	next   commandOperationID
	active map[commandOperationID]commandOperation
}

func newCommandOperationRegistry() commandOperationRegistry {
	return commandOperationRegistry{active: make(map[commandOperationID]commandOperation)}
}

func (r *commandOperationRegistry) reserve(pluginID string) (commandOperation, error) {
	next, ok := r.next.successor()
	if !ok {
		return commandOperation{}, errCommandOperationIdentityExhausted
	}
	if r.active == nil {
		r.active = make(map[commandOperationID]commandOperation)
	}
	operation := commandOperation{id: next, pluginID: pluginID}
	r.next = next
	r.active[next] = operation
	return operation, nil
}

func (r *commandOperationRegistry) retire(operation commandOperation) {
	if current, ok := r.active[operation.id]; ok && current == operation {
		delete(r.active, operation.id)
	}
}

func (r *commandOperationRegistry) take(pluginIDs ...string) []commandOperation {
	selected := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		selected[pluginID] = struct{}{}
	}
	operations := make([]commandOperation, 0, len(r.active))
	for id, operation := range r.active {
		if len(selected) > 0 {
			if _, take := selected[operation.pluginID]; !take {
				continue
			}
		}
		operations = append(operations, operation)
		delete(r.active, id)
	}
	return operations
}

func (a *app) executeCommand(pluginID string, command SlashCommand, argument string) {
	name := command.Descriptor.Name
	a.status.note("running /" + name)
	request := CommandRequest{Argument: argument, Workspace: a.session.Workspace.Path, SessionID: a.session.ID}
	dispatcher := a.loop.Dispatcher()
	operation, err := a.commandOperations.reserve(pluginID)
	if err != nil {
		a.message("could not start /" + name + ": " + err.Error())
		return
	}
	slot := operation.slot()
	started := a.operations.GoSession(slot, false, func(ctx context.Context, lease operationLease) {
		result, err := executeCommandSafely(ctx, command, request)
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed {
				return
			}
			a.commandOperations.retire(operation)
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				a.message(err.Error())
				return
			}
			message := strings.TrimSpace(result.Message)
			if message == "" {
				message = "completed /" + name
			}
			a.message(message)
		})
	})
	if !started {
		a.commandOperations.retire(operation)
		a.message("could not start /" + name)
	}
}

func (a *app) cancelPluginCommands(pluginIDs ...string) {
	for _, operation := range a.commandOperations.take(pluginIDs...) {
		a.operations.Cancel(operation.slot())
	}
}

func executeCommandSafely(ctx context.Context, command SlashCommand, request CommandRequest) (result CommandResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("command /%s panicked: %v", command.Descriptor.Name, recovered)
		}
	}()
	return command.Execute(ctx, request)
}

func runLocalCommandSafely(command localCommand, host *app, argument string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("command /%s panicked: %v", command.Descriptor.Name, recovered)
		}
	}()
	return command.Run(host, argument)
}

func (a *app) runCommand(name, argument string) {
	command, registration, ok := a.commands.index.Lookup(name)
	if !ok || registration.run == nil {
		a.message("unknown command: /" + name)
		return
	}
	if availability := registration.availability(a); !availability.Enabled {
		a.message("/" + command.Name + " unavailable: " + availability.Reason)
		return
	}
	argument = strings.TrimSpace(argument)
	if err := registration.arguments.ValidateInvocation(command.Name, argument); err != nil {
		a.message(err.Error())
		return
	}
	a.commands.used(command.Name)
	registration.run(argument)
}
