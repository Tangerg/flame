package filewatch

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/exactint"
	"github.com/Tangerg/flame/runtime/model"
	"github.com/invopop/jsonschema"
)

// Each limit owns its value for both the generated schema and the NewTrigger
// guard. boundedText measures bytes; the projected maxLength measures
// characters, a pre-existing distinction kept deliberately here.
const (
	MaxPaths              = 32
	MaxTitleLength        = 200
	MaxWorkspaceLength    = 4096
	MaxInstructionsLength = 16384
)

var (
	ErrInvalid  = errors.New("filewatch: invalid trigger")
	ErrRevision = errors.New("filewatch: configuration revision differs")
	ErrCapacity = errors.New("filewatch: pending observation capacity reached")
	ErrNotFound = errors.New("filewatch: observation not found")
)

type Draft struct {
	Title           string   `json:"title" jsonschema:"minLength=1"`
	Workspace       string   `json:"workspace" jsonschema:"minLength=1"`
	Paths           []string `json:"paths" jsonschema:"minItems=1,uniqueItems=true"`
	Instructions    string   `json:"instructions" jsonschema:"minLength=1"`
	Provider        string   `json:"provider" jsonschema:"minLength=1"`
	Model           string   `json:"model" jsonschema:"minLength=1"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"`
}

func (Draft) JSONSchemaExtend(schema *jsonschema.Schema) {
	for field, maximum := range map[string]uint64{
		"title": MaxTitleLength, "workspace": MaxWorkspaceLength, "instructions": MaxInstructionsLength,
	} {
		if property, ok := schema.Properties.Get(field); ok {
			property.MaxLength = new(maximum)
		}
	}
	if paths, ok := schema.Properties.Get("paths"); ok {
		paths.MaxItems = new(uint64(MaxPaths))
	}
}

type Trigger struct{ draft Draft }

func NewTrigger(draft Draft) (Trigger, error) {
	if !boundedText(draft.Title, MaxTitleLength) || !boundedText(draft.Instructions, MaxInstructionsLength) ||
		!boundedText(draft.Workspace, MaxWorkspaceLength) || !filepath.IsAbs(draft.Workspace) || filepath.Clean(draft.Workspace) != draft.Workspace ||
		len(draft.Paths) == 0 || len(draft.Paths) > MaxPaths {
		return Trigger{}, ErrInvalid
	}
	selection, err := model.NewWithReasoningEffort(draft.Provider, draft.Model, draft.ReasoningEffort)
	if err != nil || selection.ValidateExact() != nil {
		return Trigger{}, errors.Join(ErrInvalid, err)
	}
	seen := make(map[string]bool, len(draft.Paths))
	for _, path := range draft.Paths {
		if !boundedText(path, 4096) || !filepath.IsLocal(path) || filepath.ToSlash(filepath.Clean(path)) != path || seen[path] {
			return Trigger{}, ErrInvalid
		}
		seen[path] = true
	}

	draft.Paths = slices.Clone(draft.Paths)
	return Trigger{draft: draft}, nil
}

func boundedText(value string, maximum int) bool {
	return utf8.ValidString(value) && len(value) <= maximum && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0)
}

func (t Trigger) Draft() Draft {
	draft := t.draft
	draft.Paths = slices.Clone(draft.Paths)
	return draft
}

type Snapshot struct {
	Revision int64  `json:"revision" jsonschema:"minimum=1"`
	Trigger  *Draft `json:"trigger,omitzero"`
}

type Configuration struct {
	revision int64
	trigger  *Trigger
}

func Initial() Configuration { return Configuration{revision: 1} }

func Restore(snapshot Snapshot) (Configuration, error) {
	if snapshot.Revision < 1 || snapshot.Revision > exactint.Maximum {
		return Configuration{}, ErrInvalid
	}
	value := Configuration{revision: snapshot.Revision}
	if snapshot.Trigger != nil {
		trigger, err := NewTrigger(*snapshot.Trigger)
		if err != nil {
			return Configuration{}, err
		}
		value.trigger = &trigger
	}
	return value, nil
}

func (c Configuration) Snapshot() Snapshot {
	value := Snapshot{Revision: c.revision}
	if c.trigger != nil {
		draft := c.trigger.Draft()
		value.Trigger = &draft
	}
	return value
}

func (c Configuration) Replace(expected int64, draft *Draft) (Configuration, error) {
	if c.revision < 1 || c.revision == exactint.Maximum {
		return Configuration{}, ErrInvalid
	}
	if expected != c.revision {
		return Configuration{}, ErrRevision
	}
	return Restore(Snapshot{Revision: c.revision + 1, Trigger: draft})
}

func (c Configuration) Trigger() (Trigger, bool) {
	if c.trigger == nil {
		return Trigger{}, false
	}
	return *c.trigger, true
}

func (c Configuration) Revision() int64 { return c.revision }
