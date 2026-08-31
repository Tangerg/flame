package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/cli/internal/agent"
	cliidentity "github.com/Tangerg/flame/cli/internal/identity"
)

type DocumentFormat string

const (
	MarkdownFormat DocumentFormat = "md"
	JSONFormat     DocumentFormat = "json"

	// MaximumDocumentBytes is the complete encoded size accepted by every CLI
	// Session export/import boundary.
	MaximumDocumentBytes = 64 << 20
)

func ParseDocumentFormat(value string) (DocumentFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "md", "markdown":
		return MarkdownFormat, nil
	case "json":
		return JSONFormat, nil
	default:
		return "", fmt.Errorf("export format %q is unsupported; use markdown or json", strings.TrimSpace(value))
	}
}

func (f DocumentFormat) Extension() string {
	switch f {
	case MarkdownFormat:
		return ".md"
	case JSONFormat:
		return ".json"
	default:
		return ""
	}
}

func (f DocumentFormat) Validate() error {
	if f != MarkdownFormat && f != JSONFormat {
		return fmt.Errorf("session document format %q is invalid", f)
	}
	return nil
}

// Document is an immutable Runtime-authored export. JSON documents are
// round-trippable; Markdown documents are human-readable projections only.
type Document struct {
	format DocumentFormat
	body   []byte
}

func NewDocument(format DocumentFormat, body []byte) (Document, error) {
	body, err := validateDocumentBody(format, body)
	if err != nil {
		return Document{}, err
	}
	return Document{format: format, body: slices.Clone(body)}, nil
}

func validateDocumentBody(format DocumentFormat, body []byte) ([]byte, error) {
	if err := format.Validate(); err != nil {
		return nil, err
	}
	if len(body) > MaximumDocumentBytes {
		return nil, fmt.Errorf("session document exceeds %d bytes", MaximumDocumentBytes)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, errors.New("session document is empty")
	}
	if !utf8.Valid(body) {
		return nil, errors.New("session document is not valid UTF-8")
	}
	if format == JSONFormat && !json.Valid(body) {
		return nil, errors.New("session artifact is not valid JSON")
	}
	return body, nil
}

func (d Document) Format() DocumentFormat { return d.format }
func (d Document) Bytes() []byte          { return slices.Clone(d.body) }

func (d Document) Validate() error {
	_, err := validateDocumentBody(d.format, d.body)
	return err
}

func (d Document) Importable() bool {
	return d.format == JSONFormat && d.Validate() == nil
}

type ExportRequest struct {
	SessionID string
	Format    DocumentFormat
}

func (e ExportRequest) Validate() error {
	if err := cliidentity.ValidateSession(e.SessionID); err != nil {
		return fmt.Errorf("export session: %w", err)
	}
	if err := e.Format.Validate(); err != nil {
		return fmt.Errorf("export session: %w", err)
	}
	return nil
}

type ImportRequest struct{ Artifact Document }

func (i ImportRequest) Validate() error {
	if err := i.Artifact.Validate(); err != nil {
		return fmt.Errorf("import session: %w", err)
	}
	if !i.Artifact.Importable() {
		return errors.New("import session: only JSON session artifacts are importable")
	}
	return nil
}

type TransferService interface {
	ExportSession(context.Context, ExportRequest) (Document, error)
	ImportSession(context.Context, ImportRequest) (agent.Session, error)
}
