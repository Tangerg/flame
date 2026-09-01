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

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

const (
	// MaximumDocumentBytes is the complete encoded size accepted by every CLI
	// Session export/import boundary.
	MaximumDocumentBytes = 64 << 20
)

func ParseDocumentFormat(value string) (runtimeprotocol.ExportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "md", "markdown":
		return runtimeprotocol.ExportFormatMarkdown, nil
	case "json":
		return runtimeprotocol.ExportFormatJSON, nil
	default:
		return "", fmt.Errorf("export format %q is unsupported; use markdown or json", strings.TrimSpace(value))
	}
}

func documentExtension(format runtimeprotocol.ExportFormat) string {
	switch format {
	case runtimeprotocol.ExportFormatMarkdown:
		return ".md"
	case runtimeprotocol.ExportFormatJSON:
		return ".json"
	default:
		return ""
	}
}

// Document is an immutable Runtime-authored export. JSON documents are
// round-trippable; Markdown documents are human-readable projections only.
type Document struct {
	format runtimeprotocol.ExportFormat
	body   []byte
}

func NewDocument(format runtimeprotocol.ExportFormat, body []byte) (Document, error) {
	body, err := validateDocumentBody(format, body)
	if err != nil {
		return Document{}, err
	}
	return Document{format: format, body: slices.Clone(body)}, nil
}

func validateDocumentBody(format runtimeprotocol.ExportFormat, body []byte) ([]byte, error) {
	switch format {
	case runtimeprotocol.ExportFormatMarkdown, runtimeprotocol.ExportFormatJSON:
	default:
		return nil, fmt.Errorf("session document format %q is invalid", format)
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
	if format == runtimeprotocol.ExportFormatJSON && !json.Valid(body) {
		return nil, errors.New("session artifact is not valid JSON")
	}
	return body, nil
}

func (d Document) Format() runtimeprotocol.ExportFormat { return d.format }
func (d Document) Extension() string                    { return documentExtension(d.format) }
func (d Document) Bytes() []byte                        { return slices.Clone(d.body) }

func (d Document) Validate() error {
	_, err := validateDocumentBody(d.format, d.body)
	return err
}

func (d Document) Importable() bool {
	return d.format == runtimeprotocol.ExportFormatJSON && d.Validate() == nil
}

type ExportRequest struct {
	SessionID string
	Format    runtimeprotocol.ExportFormat
}

func (e ExportRequest) Validate() error {
	if err := (runtimeprotocol.ExportSessionRequest{SessionID: e.SessionID, Format: e.Format}).ValidateWire(); err != nil {
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
