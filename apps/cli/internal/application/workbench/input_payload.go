package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const maximumInputBytes = 6 * (authoring.MaxMessageTextBytes + authoring.MaxMessageAttachments*authoring.MaxAttachmentBytes + 2*maximumStateBytes + 1024)

type inputDigest string

func (d inputDigest) validate() error {
	if d == "" {
		return nil
	}
	decoded, err := hex.DecodeString(string(d))
	if err != nil || len(decoded) != sha256.Size || string(d) != strings.ToLower(string(d)) {
		return errors.New("prepared input digest is invalid")
	}
	return nil
}

type inputOperation string

const (
	startInput  inputOperation = "start"
	resumeInput inputOperation = "resume"
	steerInput  inputOperation = "steer"
)

func inputRecordKey(sessionID string, operation inputOperation, commandID authoring.CommandID) string {
	return "input/" + sessionID + "/" + string(operation) + "/" + string(commandID)
}

type PreparedInput struct {
	owner   *Store
	message authoring.Message
	blocks  []protocol.ContentBlock
	digest  inputDigest
	encoded []byte
}

func (s *Store) PrepareInput(ctx context.Context, message authoring.Message, input []protocol.ContentBlock) (*PreparedInput, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if _, err := message.PreparedInput(input); err != nil {
		return nil, err
	}
	prepared := &PreparedInput{owner: s, message: message.Clone()}
	if len(message.Attachments) == 0 {
		return prepared, nil
	}
	prepared.blocks = slices.Clone(input)
	encoded, err := json.Marshal(prepared.blocks, stateJSONOptions)
	if err != nil {
		return nil, fmt.Errorf("encode prepared input: %w", err)
	}
	if int64(len(encoded)) > maximumInputBytes {
		return nil, errors.New("prepared input exceeds the attachment budget")
	}
	prepared.digest = inputDigest(fmt.Sprintf("%x", sha256.Sum256(encoded)))
	prepared.encoded = encoded
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (p *PreparedInput) bind(s *Store, message authoring.Message) ([]protocol.ContentBlock, inputDigest, error) {
	if p == nil {
		_, err := message.PreparedInput(nil)
		return nil, "", err
	}
	if p.owner != s || !p.message.Equal(message) {
		return nil, "", errors.New("prepared input belongs to another store or message")
	}
	return slices.Clone(p.blocks), p.digest, nil
}

func (p *PreparedInput) records(key string) []RecordChange {
	if p == nil || p.encoded == nil {
		return nil
	}
	return []RecordChange{{Key: key, Body: p.encoded}}
}

func (s *Store) readInput(key string, digest inputDigest) ([]protocol.ContentBlock, error) {
	if digest == "" {
		return nil, nil
	}
	if err := digest.validate(); err != nil {
		return nil, err
	}
	encoded, err := s.persistence.Read(key, maximumInputBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", authoring.ErrCommandInputUnavailable, key, err)
	}
	if inputDigest(fmt.Sprintf("%x", sha256.Sum256(encoded))) != digest {
		return nil, fmt.Errorf("%w: content digest mismatch", authoring.ErrCommandInputUnavailable)
	}
	var input []protocol.ContentBlock
	if err := decodeStateJSON(encoded, &input); err != nil {
		return nil, fmt.Errorf("%w: decode content: %w", authoring.ErrCommandInputUnavailable, err)
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("%w: content is empty", authoring.ErrCommandInputUnavailable)
	}
	return input, nil
}

func (p PendingRun) ReplayCommand() (authoring.StartRun, error) {
	if p.Dispatch == nil {
		return authoring.StartRun{}, errors.New("queued run has no replayable dispatch")
	}
	if p.Dispatch.inputFailure != nil {
		return authoring.StartRun{}, p.Dispatch.inputFailure
	}
	if _, err := p.Command.Message.PreparedInput(p.Command.Input); err != nil {
		return authoring.StartRun{}, err
	}
	return p.Command.Clone(), nil
}

func (p PendingResume) ReplayCommand() (conversation.ResumeRun, error) {
	if p.inputFailure != nil {
		return conversation.ResumeRun{}, p.inputFailure
	}
	if p.Command.Message != nil {
		if _, err := p.Command.Message.PreparedInput(p.Command.Input); err != nil {
			return conversation.ResumeRun{}, err
		}
	}
	return p.Command.Clone(), nil
}

func (p PendingSteer) ReplayCommand() (authoring.SteerRun, error) {
	if p.inputFailure != nil {
		return authoring.SteerRun{}, p.inputFailure
	}
	if _, err := p.command.Message.PreparedInput(p.command.Input); err != nil {
		return authoring.SteerRun{}, err
	}
	return p.command.Clone(), nil
}
