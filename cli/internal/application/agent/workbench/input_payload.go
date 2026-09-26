package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

// Payloads are separate from the 16 MiB authoring record: one supported image
// alone may contain 20 MiB before base64. The bound covers all supported input,
// JSON's worst-case escaping, and attachment names already bounded by a record.
const maximumInputBytes = 6 * (agent.MaxMessageTextBytes + agent.MaxMessageAttachments*agent.MaxAttachmentBytes + 2*maximumStateBytes + 1024)

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

func (d inputDigest) name() string { return filepath.Join("inputs", string(d)+".json") }

// PreparedInput is immutable content already saved in this Store. Its private
// fields bind the bytes to the complete source message, so publication cannot
// accidentally attach an earlier preparation to an edited queue entry.
// A nil value is sufficient only for a message without filesystem attachments.
type PreparedInput struct {
	owner   *Store
	message agent.Message
	blocks  []protocol.ContentBlock
	digest  inputDigest
}

// PrepareInput writes the immutable content before a session record publishes
// its digest with the command ID and replay guard. Expensive encoding and blob
// I/O run outside the authoring lock and can be owned by a background operation.
// Cancellation or a failed later record write leaves only an unreferenced blob.
func (s *Store) PrepareInput(ctx context.Context, message agent.Message, input []protocol.ContentBlock) (*PreparedInput, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if _, err := message.PreparedInput(input); err != nil {
		return nil, err
	}
	prepared := &PreparedInput{owner: s, message: message.Clone(), blocks: slices.Clone(input)}
	if input == nil {
		return prepared, nil
	}
	encoded, err := json.Marshal(prepared.blocks, stateJSONOptions)
	if err != nil {
		return nil, fmt.Errorf("encode prepared input: %w", err)
	}
	if int64(len(encoded)) > maximumInputBytes {
		return nil, errors.New("prepared input exceeds the attachment budget")
	}
	prepared.digest = inputDigest(fmt.Sprintf("%x", sha256.Sum256(encoded)))
	s.mu.Lock()
	writable, err := s.writable()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if writable {
		if err := s.persistence.Replace(prepared.digest.name(), encoded); err != nil {
			return nil, fmt.Errorf("save prepared input: %w", err)
		}
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (p *PreparedInput) bind(s *Store, message agent.Message) ([]protocol.ContentBlock, inputDigest, error) {
	if p == nil {
		_, err := message.PreparedInput(nil)
		return nil, "", err
	}
	if p.owner != s || !p.message.Equal(message) {
		return nil, "", errors.New("prepared input belongs to another store or message")
	}
	return slices.Clone(p.blocks), p.digest, nil
}

func (s *Store) readInput(digest inputDigest) ([]protocol.ContentBlock, error) {
	if digest == "" {
		return nil, nil
	}
	if err := digest.validate(); err != nil {
		return nil, err
	}
	encoded, err := s.persistence.Read(digest.name(), maximumInputBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", agent.ErrCommandInputUnavailable, digest.name(), err)
	}
	if inputDigest(fmt.Sprintf("%x", sha256.Sum256(encoded))) != digest {
		return nil, fmt.Errorf("%w: content digest mismatch", agent.ErrCommandInputUnavailable)
	}
	var input []protocol.ContentBlock
	if err := decodeStateJSON(encoded, &input); err != nil {
		return nil, fmt.Errorf("%w: decode content: %w", agent.ErrCommandInputUnavailable, err)
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("%w: content is empty", agent.ErrCommandInputUnavailable)
	}
	return input, nil
}

func (p PendingRun) ReplayCommand() (agent.StartRun, error) {
	if p.inputFailure != nil {
		return agent.StartRun{}, p.inputFailure
	}
	if _, err := p.Command.Message.PreparedInput(p.Command.Input); err != nil {
		return agent.StartRun{}, err
	}
	return p.Command.Clone(), nil
}

func (p PendingResume) ReplayCommand() (agent.ResumeRun, error) {
	if p.inputFailure != nil {
		return agent.ResumeRun{}, p.inputFailure
	}
	if p.Command.Message != nil {
		if _, err := p.Command.Message.PreparedInput(p.Command.Input); err != nil {
			return agent.ResumeRun{}, err
		}
	}
	return p.Command.Clone(), nil
}

func (p PendingSteer) ReplayCommand() (agent.SteerRun, error) {
	if p.inputFailure != nil {
		return agent.SteerRun{}, p.inputFailure
	}
	if _, err := p.command.Message.PreparedInput(p.command.Input); err != nil {
		return agent.SteerRun{}, err
	}
	return p.command.Clone(), nil
}
