package session

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	// MaximumDocumentBytes is the complete encoded size accepted by every CLI
	// Session export/import boundary.
	MaximumDocumentBytes = 64 << 20
)

func ParseDocumentFormat(value string) (protocol.ExportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "md", "markdown":
		return protocol.ExportFormatMarkdown, nil
	case "json":
		return protocol.ExportFormatJSON, nil
	default:
		return "", fmt.Errorf("export format %q is unsupported; use markdown or json", strings.TrimSpace(value))
	}
}

func documentExtension(format protocol.ExportFormat) string {
	switch format {
	case protocol.ExportFormatMarkdown:
		return ".md"
	case protocol.ExportFormatJSON:
		return ".json"
	default:
		return ""
	}
}

// Document is an immutable Runtime-authored export. JSON documents are
// round-trippable; Markdown documents are human-readable projections only.
type Document struct {
	format protocol.ExportFormat
	body   []byte
}

func NewDocument(format protocol.ExportFormat, body []byte) (Document, error) {
	body, err := validateDocumentBody(format, body)
	if err != nil {
		return Document{}, err
	}
	return Document{format: format, body: slices.Clone(body)}, nil
}

func validateDocumentBody(format protocol.ExportFormat, body []byte) ([]byte, error) {
	switch format {
	case protocol.ExportFormatMarkdown, protocol.ExportFormatJSON:
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
	if format == protocol.ExportFormatJSON && !jsontext.Value(body).IsValid() {
		return nil, errors.New("session artifact is not valid JSON")
	}
	return body, nil
}

func (d Document) Format() protocol.ExportFormat { return d.format }
func (d Document) Extension() string             { return documentExtension(d.format) }
func (d Document) Bytes() []byte                 { return slices.Clone(d.body) }

func (d Document) Validate() error {
	_, err := validateDocumentBody(d.format, d.body)
	return err
}

func (d Document) Importable() bool {
	return d.format == protocol.ExportFormatJSON && d.Validate() == nil
}

type ExportRequest struct {
	SessionID string
	Format    protocol.ExportFormat
}

func (e ExportRequest) Validate() error {
	if err := protocol.ValidateWireTree((protocol.ExportSessionRequest{SessionID: e.SessionID, Format: e.Format})); err != nil {
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
	ImportSession(context.Context, ImportRequest) (conversation.Session, error)
}
