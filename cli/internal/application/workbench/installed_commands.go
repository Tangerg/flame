package workbench

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/protocol"
)

const installedCommandsName = "installed-commands.json"

// PendingInstalledCommand retains client intent, never a business outcome.
// Recovery uses this exact request before discovering any current provider.
type PendingInstalledCommand struct {
	Intent    string
	CommandID replay.CommandID
	Request   protocol.CallPluginRequest
	Replay    replay.Guard
}

// Input is binary in the local record so formatting cannot change accepted
// request bytes. The remaining fields are projections of the captured binding.
type installedCommandRecord struct {
	Intent         string            `json:"intent"`
	CommandID      replay.CommandID  `json:"commandId"`
	InstallationID string            `json:"installationId"`
	Digest         string            `json:"digest"`
	Contribution   string            `json:"contribution"`
	Contract       string            `json:"contract"`
	Method         string            `json:"method"`
	Input          []byte            `json:"input"`
	Target         capability.Target `json:"target"`
	Replay         replay.Guard      `json:"replay"`
}

func (p PendingInstalledCommand) MarshalJSON() ([]byte, error) {
	request := p.Request
	return json.Marshal(installedCommandRecord{Intent: p.Intent, CommandID: p.CommandID, InstallationID: request.InstallationID, Digest: request.Digest, Contribution: request.Contribution, Contract: request.Contract, Method: request.Method, Input: request.Input, Target: request.Target, Replay: p.Replay})
}

func (p *PendingInstalledCommand) UnmarshalJSON(body []byte) error {
	var record installedCommandRecord
	if err := json.Unmarshal(body, &record, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	next := PendingInstalledCommand{Intent: record.Intent, CommandID: record.CommandID, Replay: record.Replay, Request: protocol.CallPluginRequest{InstallationID: record.InstallationID, Digest: record.Digest, Contribution: record.Contribution, Contract: record.Contract, Method: record.Method, Input: record.Input, Target: record.Target}}
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
	contract, err := capability.Parse([]byte(p.Request.Contract))
	if err != nil {
		return err
	}
	method, found := contract.Method(p.Request.Method)
	if !found || method.Kind() != capability.Mutation {
		return errors.New("installed command method is not a command")
	}
	if method.Descriptor().Target != p.Request.Target.Scope {
		return errors.New("installed command target does not match its method")
	}
	return nil
}

func (p PendingInstalledCommand) clone() PendingInstalledCommand {
	p.Request.Input = slices.Clone(p.Request.Input)
	return p
}

func (s *Store) loadInstalledCommands() error {
	var commands []PendingInstalledCommand
	if err := s.loadOptional(installedCommandsName, &commands); err != nil {
		return fmt.Errorf("load installed commands: %w", err)
	}
	identities := make(map[replay.CommandID]struct{}, len(commands))
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
		s.installedCommands[command.Intent] = command.clone()
	}
	return nil
}

func (s *Store) PendingInstalledCommand(intent string) (PendingInstalledCommand, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, found := s.installedCommands[intent]
	return pending.clone(), found
}

func (s *Store) PendingInstalledCommands() []PendingInstalledCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedInstalledCommands(s.installedCommands)
}

func (s *Store) StageInstalledCommand(command PendingInstalledCommand) error {
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
	next[command.Intent] = command.clone()
	if err := s.save(installedCommandsName, sortedInstalledCommands(next)); err != nil {
		return err
	}
	s.installedCommands = next
	return nil
}

func (s *Store) RetireInstalledCommand(command PendingInstalledCommand) error {
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
		a.Contract == b.Contract && a.Method == b.Method && a.Target == b.Target && bytes.Equal(a.Input, b.Input)
}

func sortedInstalledCommands(commands map[string]PendingInstalledCommand) []PendingInstalledCommand {
	keys := slices.Sorted(maps.Keys(commands))
	result := make([]PendingInstalledCommand, 0, len(keys))
	for _, key := range keys {
		result = append(result, commands[key].clone())
	}
	return result
}
