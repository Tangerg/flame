package domain

import (
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	MaxDocumentBytes = 2 << 20
	MaxPageRunes     = 8192
	MaxDocuments     = 32
	MaxArchiveBytes  = 64 << 20
)

var (
	ErrDocument = errors.New("web: invalid document snapshot")
	ErrRange    = errors.New("web: invalid snapshot page range")
	ErrNotFound = errors.New("web: snapshot is absent or has been evicted")
)

type Format string

const (
	Markdown Format = "markdown"
	HTML     Format = "html"
	Text     Format = "text"
)

func (Format) JSONSchemaEnum() []string {
	return []string{string(Markdown), string(HTML), string(Text)}
}

type Content struct {
	URL    string `json:"url"`
	Format Format `json:"format"`
	Text   string `json:"text"`
}

type Snapshot struct {
	ID string `json:"id"`
	Content
}

type Page struct {
	SnapshotID string `json:"snapshotId"`
	URL        string `json:"url"`
	Format     Format `json:"format"`
	Content    string `json:"content"`
	Offset     int64  `json:"offset"`
	TotalRunes int64  `json:"totalRunes"`
	NextOffset *int64 `json:"nextOffset,omitzero"`
}

type Document struct{ snapshot Snapshot }

func NewDocument(content Content) (Document, error) {
	if err := ValidateURL(content.URL); err != nil {
		return Document{}, err
	}
	if !utf8.ValidString(content.Text) || len(content.Text) > MaxDocumentBytes {
		return Document{}, ErrDocument
	}
	switch content.Format {
	case Markdown, HTML, Text:
	default:
		return Document{}, ErrDocument
	}
	encoded, err := json.Marshal(content, json.Deterministic(true))
	if err != nil {
		return Document{}, err
	}
	return Document{snapshot: Snapshot{ID: fmt.Sprintf("%x", sha256.Sum256(encoded)), Content: content}}, nil
}

func Restore(snapshot Snapshot) (Document, error) {
	document, err := NewDocument(snapshot.Content)
	if err != nil || snapshot.ID != document.snapshot.ID {
		return Document{}, errors.Join(ErrDocument, err)
	}
	return document, nil
}

func ValidateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || !utf8.ValidString(raw) || len(raw) > 4096 || strings.TrimSpace(raw) != raw || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ErrDocument
	}
	return nil
}

func (d Document) Snapshot() Snapshot { return d.snapshot }

func (d Document) Page(offset, limit int64) (Page, error) {
	if d.snapshot.ID == "" {
		return Page{}, ErrDocument
	}
	if offset < 0 || limit < 1 || limit > MaxPageRunes {
		return Page{}, ErrRange
	}
	runes := []rune(d.snapshot.Text)
	if offset > int64(len(runes)) {
		return Page{}, ErrRange
	}
	end := min(int64(len(runes)), offset+limit)
	page := Page{SnapshotID: d.snapshot.ID, URL: d.snapshot.URL, Format: d.snapshot.Format, Content: string(runes[offset:end]), Offset: offset, TotalRunes: int64(len(runes))}
	if end < page.TotalRunes {
		page.NextOffset = new(end)
	}
	return page, nil
}
