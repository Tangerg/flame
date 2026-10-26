package workbench

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const installedCommandsName = "installed-commands.json"

// PendingInstalledCommand retains client intent, never a business outcome.
// Recovery uses this exact request before discovering any current provider.
type PendingInstalledCommand struct {
	Intent    string
	CommandID authoring.CommandID
	Request   protocol.CallPluginRequest
	Replay    authoring.ReplayGuard
}

// Input is binary in the local record so formatting cannot change accepted
// request bytes. The remaining fields are projections of the captured binding.
type installedCommandRecord struct {
	Intent         string                `json:"intent"`
	CommandID      authoring.CommandID   `json:"commandId"`
	InstallationID string                `json:"installationId"`
	Digest         string                `json:"digest"`
	Contribution   string                `json:"contribution"`
	Contract       string                `json:"contract"`
	Method         string                `json:"method"`
	Input          []byte                `json:"input"`
	Target         extension.Target      `json:"target"`
	Replay         authoring.ReplayGuard `json:"replay"`
}

func (p PendingInstalledCommand) MarshalJSON() ([]byte, error) {
	request := p.Request
	return json.Marshal(installedCommandRecord{Intent: p.Intent, CommandID: p.CommandID, InstallationID: request.InstallationID, Digest: request.Digest, Contribution: request.Contribution, Contract: request.Contract, Method: request.Method, Input: []byte(request.Input), Target: request.Target, Replay: p.Replay})
}

func (p *PendingInstalledCommand) UnmarshalJSON(body []byte) error {
	var record installedCommandRecord
	if err := json.Unmarshal(body, &record, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	next := PendingInstalledCommand{Intent: record.Intent, CommandID: record.CommandID, Replay: record.Replay, Request: protocol.CallPluginRequest{InstallationID: record.InstallationID, Digest: record.Digest, Contribution: record.Contribution, Contract: record.Contract, Method: record.Method, Input: string(record.Input), Target: record.Target}}
	if err := next.validate(); err != nil {
		return err
	}
	*p = next
	return nil
}

func (p PendingInstalledCommand) validate() error {
	if strings.TrimSpace(p.Intent) == "" || p.Intent != strings.TrimSpace(p.Intent) {
		return errors.New("installed command intent is empty or padded")
	}
	if err := errors.Join(p.CommandID.Validate(), p.Replay.Validate(), protocol.ValidateWireTree(p.Request), p.Request.Target.Validate()); err != nil {
		return err
	}
	contract, err := extension.ParseCapability([]byte(p.Request.Contract))
	if err != nil {
		return err
	}
	method, found := contract.Method(p.Request.Method)
	if !found || method.Kind() != extension.Mutation {
		return errors.New("installed command method is not a command")
	}
	if method.Descriptor().Target != p.Request.Target.Scope {
		return errors.New("installed command target does not match its method")
	}
	return nil
}

func (s *Store) loadInstalledCommands() error {
	var commands []PendingInstalledCommand
	if err := s.loadOptional(installedCommandsName, &commands); err != nil {
		return fmt.Errorf("load installed commands: %w", err)
	}
	identities := make(map[authoring.CommandID]struct{}, len(commands))
	for _, command := range commands {
		if err := command.validate(); err != nil {
			return fmt.Errorf("load installed command: %w", err)
		}
		if _, duplicate := s.installedCommands[command.Intent]; duplicate {
			return errors.New("duplicate installed command intent")
		}
		if _, duplicate := identities[command.CommandID]; duplicate {
			return errors.New("duplicate installed command identity")
		}
		identities[command.CommandID] = struct{}{}
		s.installedCommands[command.Intent] = command
	}
	return nil
}

func (s *Store) PendingInstalledCommand(intent string) (PendingInstalledCommand, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, found := s.installedCommands[intent]
	return pending, found
}

func (s *Store) PendingInstalledCommands() []PendingInstalledCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedInstalledCommands(s.installedCommands)
}

func (s *Store) stageInstalledCommand(command PendingInstalledCommand) error {
	if err := command.validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, found := s.installedCommands[command.Intent]; found {
		if installedCommandEqual(existing, command) {
			return nil
		}
		return errors.New("another installed command is already pending for this intent")
	}
	for _, existing := range s.installedCommands {
		if existing.CommandID == command.CommandID {
			return errors.New("installed command identity belongs to another intent")
		}
	}
	next := maps.Clone(s.installedCommands)
	next[command.Intent] = command
	if err := s.save(installedCommandsName, sortedInstalledCommands(next)); err != nil {
		return err
	}
	s.installedCommands = next
	return nil
}

func (s *Store) retireInstalledCommand(command PendingInstalledCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, found := s.installedCommands[command.Intent]
	if !found {
		return nil
	}
	if !installedCommandEqual(existing, command) {
		return errors.New("installed command does not match the acknowledged intent")
	}
	next := maps.Clone(s.installedCommands)
	delete(next, command.Intent)
	if err := s.save(installedCommandsName, sortedInstalledCommands(next)); err != nil {
		return err
	}
	s.installedCommands = next
	return nil
}

func installedCommandEqual(left, right PendingInstalledCommand) bool {
	a, b := left.Request, right.Request
	return left.Intent == right.Intent && left.CommandID == right.CommandID && left.Replay == right.Replay &&
		a.InstallationID == b.InstallationID && a.Digest == b.Digest && a.Contribution == b.Contribution &&
		a.Contract == b.Contract && a.Method == b.Method && a.Target == b.Target && a.Input == b.Input
}

func sortedInstalledCommands(commands map[string]PendingInstalledCommand) []PendingInstalledCommand {
	keys := slices.Sorted(maps.Keys(commands))
	result := make([]PendingInstalledCommand, 0, len(keys))
	for _, key := range keys {
		result = append(result, commands[key])
	}
	return result
}

type InstalledCaller interface {
	CallPlugin(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error)
}

// ExecuteInstalledCommand stages the complete request before I/O. An unresolved intent always
// resumes its original request, including after restart or provider replacement.
func ExecuteInstalledCommand[T any](ctx context.Context, caller InstalledCaller, store *Store, intent string, policy mutation.ReplayPolicy, prepare func(context.Context) (protocol.CallPluginRequest, error), decode func(*protocol.CallPluginResponse) (T, error)) (T, error) {
	return executeInstalledCommand(ctx, caller, store, intent, policy, prepare, nil, decode)
}

func executeInstalledCommand[T any](ctx context.Context, caller InstalledCaller, store *Store, intent string, policy mutation.ReplayPolicy, prepare func(context.Context) (protocol.CallPluginRequest, error), admit func(protocol.CallPluginRequest) error, decode func(*protocol.CallPluginResponse) (T, error)) (T, error) {
	var zero T
	if store == nil {
		return zero, ErrUnavailable
	}
	pending, exists := store.PendingInstalledCommand(intent)
	if !exists {
		request, err := prepare(ctx)
		if err != nil {
			return zero, err
		}
		if admit != nil {
			if err := admit(request); err != nil {
				return zero, err
			}
		}
		guard, err := policy.NewGuard()
		if err != nil {
			return zero, err
		}
		pending = PendingInstalledCommand{Intent: intent, CommandID: mutation.NewCommandID(), Request: request, Replay: guard}
		if err := store.stageInstalledCommand(pending); err != nil {
			return zero, fmt.Errorf("stage installed command: %w", err)
		}
		pending, exists = store.PendingInstalledCommand(intent)
		if !exists {
			return zero, errors.New("staged installed command is absent")
		}
	} else if admit != nil {
		if err := admit(pending.Request); err != nil {
			return zero, err
		}
	}
	var ownerRefusal *extension.Refusal
	result, err := mutation.ConfirmAdmitted(ctx, mutation.AcknowledgementBackoff(), mutation.ReplayAdmission(policy, pending.Replay), func(ctx context.Context) (T, error) {
		request := pending.Request
		reply, err := caller.CallPlugin(ctx, request, pending.CommandID)
		ownerRefusal = nil
		if err != nil {
			errors.As(err, &ownerRefusal)
			return zero, err
		}
		if reply == nil {
			return zero, errors.New("installed command acknowledgement is absent")
		}
		if reply.Failure != nil {
			ownerRefusal = &extension.Refusal{Code: reply.Failure.Code, Message: reply.Failure.Message}
			return zero, ownerRefusal
		}
		return decode(reply)
	})
	if err != nil && ownerRefusal == nil {
		return zero, fmt.Errorf("installed command outcome remains unknown: %w", errors.Join(authoring.ErrCommandOutcomeUnknown, err))
	}
	if retireErr := store.retireInstalledCommand(pending); retireErr != nil {
		return zero, errors.Join(err, fmt.Errorf("retire installed command intent: %w", retireErr))
	}
	return result, err
}
