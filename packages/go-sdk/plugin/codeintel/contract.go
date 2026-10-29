// Package codeintel publishes the optional workspace analysis consumer contract.
// Publishers compute results; only the caller may commit a file change.
package codeintel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

// Worst-case JSON escaping still fits the standard 16 MiB MCP frame, including
// the bounded diagnostic baseline and invocation envelope.
const MaxDocumentBytes = 2 << 20
const MaxProblems = 4096
const MaxSummaryBytes = 64 << 10

type Document struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Version   string `json:"version" jsonschema:"pattern=^[a-f0-9]{64}$"`
	Content   string `json:"content"`
}

func NewDocument(workspace, path, content string) (Document, error) {
	document := Document{Workspace: workspace, Path: path, Content: content, Version: fmt.Sprintf("%x", sha256.Sum256([]byte(content)))}
	return document, document.Validate()
}

func (d Document) Validate() error {
	if !filepath.IsAbs(d.Workspace) || !filepath.IsLocal(d.Path) || d.Path == "." ||
		len(d.Workspace) > 4096 || len(d.Path) > 4096 ||
		!validText(d.Workspace) || !validText(d.Path) {
		return errors.New("code intelligence: an absolute workspace and local file path are required")
	}
	if err := validateContent(d.Content); err != nil {
		return err
	}
	if d.Version != fmt.Sprintf("%x", sha256.Sum256([]byte(d.Content))) {
		return errors.New("code intelligence: document version does not identify its content")
	}
	return nil
}

type Transformation struct {
	Version string  `json:"version" jsonschema:"pattern=^[a-f0-9]{64}$"`
	Content *string `json:"content,omitzero"`
}

func (t Transformation) Validate(source Document) error {
	if t.Version != source.Version {
		return errors.New("code intelligence: transformation belongs to another source version")
	}
	if t.Content != nil {
		return validateContent(*t.Content)
	}
	return nil
}

// Baseline contains opaque, position-independent problem identities. Repeated
// identities preserve multiplicity; the publisher owns comparison and rendering.
type Inspection struct {
	Document Document `json:"document"`
	Baseline []string `json:"baseline"`
}

func (i Inspection) Validate() error {
	return errors.Join(i.Document.Validate(), validateProblems(i.Baseline))
}

type Report struct {
	Version  string   `json:"version" jsonschema:"pattern=^[a-f0-9]{64}$"`
	Problems []string `json:"problems"`
	Summary  string   `json:"summary"`
}

func (r Report) Validate(source Document) error {
	if r.Version != source.Version {
		return errors.New("code intelligence: diagnostics belong to another source version")
	}
	if !validText(r.Summary) || len(r.Summary) > MaxSummaryBytes {
		return errors.New("code intelligence: invalid diagnostic summary")
	}
	return validateProblems(r.Problems)
}

func validateProblems(values []string) error {
	if len(values) > MaxProblems {
		return errors.New("code intelligence: diagnostic capacity exceeded")
	}
	for _, value := range values {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != value {
			return errors.New("code intelligence: invalid diagnostic identity")
		}
	}
	return nil
}

func validText(value string) bool { return utf8.ValidString(value) && !strings.ContainsRune(value, 0) }

func validateContent(value string) error {
	if len(value) > MaxDocumentBytes {
		return fmt.Errorf("code intelligence: document exceeds the %d MiB limit", MaxDocumentBytes>>20)
	}
	if !validText(value) {
		return errors.New("code intelligence: document must contain UTF-8 text without NUL")
	}
	return nil
}

func FormatMethod() (*qimumcp.Method[Document, Transformation], error) {
	return qimumcp.NewMethod[Document, Transformation](qimumcp.MethodConfig{Name: "flame.code-intelligence/format.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Compute a transformation of the supplied exact source version without writing workspace files. Absent content means unsupported formatting."})
}

func InspectMethod() (*qimumcp.Method[Inspection, Report], error) {
	return qimumcp.NewMethod[Inspection, Report](qimumcp.MethodConfig{Name: "flame.code-intelligence/inspect.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Analyze the supplied source version and render newly introduced problems against the captured baseline. Unavailable or stale diagnostics are explicit failures."})
}

func Contract() (*portable.Capability, error) {
	format, err := FormatMethod()
	if err != nil {
		return nil, err
	}
	inspect, err := InspectMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.code-intelligence.v1", Composition: portable.Selection, Scope: extension.ScopeInstallation}, format.Descriptor(), inspect.Descriptor())
}
