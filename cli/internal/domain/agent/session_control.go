package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// RollbackSession keeps ToRunID and every earlier root run. An empty ToRunID
// clears all history; file restoration therefore requires a concrete boundary.
type RollbackSession struct {
	CommandID CommandID
	SessionID string
	ToRunID   string
	Scope     runtimeprotocol.RestoreType
}

func (r RollbackSession) Validate() error {
	var problems []error
	if r.CommandID != "" {
		if err := r.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if err := (runtimeprotocol.RollbackSessionRequest{
		SessionID: r.SessionID, ToRunID: r.ToRunID, RestoreType: r.Scope,
	}).ValidateWire(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("rollback session: %w", err)
	}
	return nil
}

func (r RollbackSession) RestoresFiles() bool {
	return r.Scope == runtimeprotocol.RestoreFiles || r.Scope == runtimeprotocol.RestoreBoth
}

func (r RollbackSession) FilesOnly() bool { return r.Scope == runtimeprotocol.RestoreFiles }

func (r RollbackSession) HistoryOnly() bool {
	return r.Scope == "" || r.Scope == runtimeprotocol.RestoreHistory
}

// InputContent preserves a dropped run's opening input without pretending an
// inline runtime image is already a local authoring attachment.
type InputContent struct {
	Kind     runtimeprotocol.ContentBlockType
	Text     string
	MimeType string
	Data     []byte
}

func (i InputContent) Clone() InputContent {
	i.Text = strings.Clone(i.Text)
	i.MimeType = strings.Clone(i.MimeType)
	i.Data = slices.Clone(i.Data)
	return i
}

func (i InputContent) Validate() error {
	switch i.Kind {
	case runtimeprotocol.ContentBlockText:
		if strings.TrimSpace(i.Text) == "" || i.MimeType != "" || len(i.Data) != 0 {
			return errors.New("text input content is malformed")
		}
	case runtimeprotocol.ContentBlockImage:
		if strings.TrimSpace(i.MimeType) == "" || len(i.Data) == 0 || i.Text != "" {
			return errors.New("image input content is malformed")
		}
	default:
		return fmt.Errorf("input content kind %q is invalid", i.Kind)
	}
	return nil
}

type DroppedRun struct {
	RunID string
	Input []InputContent
}

func (d DroppedRun) Clone() DroppedRun {
	input := d.Input
	d.Input = make([]InputContent, len(input))
	for index, content := range input {
		d.Input[index] = content.Clone()
	}
	return d
}

func (d DroppedRun) Validate() error {
	if err := runtimeprotocol.ValidateRunID(d.RunID); err != nil {
		return fmt.Errorf("dropped run: %w", err)
	}
	for index, content := range d.Input {
		if err := content.Validate(); err != nil {
			return fmt.Errorf("dropped run input %d: %w", index+1, err)
		}
	}
	return nil
}

// OpeningText joins the first dropped root input's text blocks for restoring
// the composer. It also reports inline images that still require attachment
// materialization by the delivery layer.
func (d DroppedRun) OpeningText() (string, int) {
	parts := make([]string, 0, len(d.Input))
	images := 0
	for _, content := range d.Input {
		switch content.Kind {
		case runtimeprotocol.ContentBlockText:
			parts = append(parts, content.Text)
		case runtimeprotocol.ContentBlockImage:
			images++
		}
	}
	return strings.Join(parts, "\n\n"), images
}

type RollbackResult struct {
	Session Session
	Dropped []DroppedRun
}

func (r RollbackResult) Validate() error {
	if err := r.Session.Validate(); err != nil {
		return fmt.Errorf("rollback result: %w", err)
	}
	seen := make(map[string]struct{}, len(r.Dropped))
	for index, run := range r.Dropped {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("rollback result dropped run %d: %w", index+1, err)
		}
		if _, duplicate := seen[run.RunID]; duplicate {
			return fmt.Errorf("rollback result repeats run %q", run.RunID)
		}
		seen[run.RunID] = struct{}{}
	}
	return nil
}

// FirstOpeningInput returns the earliest dropped root input. Child and
// continuation runs carry no opening input and are skipped.
func (r RollbackResult) FirstOpeningInput() (DroppedRun, bool) {
	for _, run := range r.Dropped {
		if len(run.Input) != 0 {
			return run.Clone(), true
		}
	}
	return DroppedRun{}, false
}
