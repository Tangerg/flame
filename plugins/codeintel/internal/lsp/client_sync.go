package lsp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/frame/cancellation"
)

const maxDocumentBytes int64 = 8 << 20

// maxOpenDocuments bounds the synchronized document set. A client is opened once
// per language server and lives as long as the backend, so without a bound both
// this map and the server's own copy of every file the agent ever inspected grow
// for the life of the process. Evicting the least recently synced document costs
// one didOpen when it is next touched.
const maxOpenDocuments = 128

// errDocumentTooLarge reports a workspace document that cannot be admitted to
// the in-memory language-server synchronization boundary.
var errDocumentTooLarge = fmt.Errorf(
	"lsp: document exceeds the %d MiB limit", maxDocumentBytes>>20)

// errUnsupportedDocument reports a source that cannot be represented as one
// Language Server Protocol text document.
var errUnsupportedDocument = errors.New("lsp: document is not a regular UTF-8 text file")

// ensureOpen makes the server aware of abs's current on-disk content: a
// didOpen the first time, a didChange when the content has changed since we
// last synced (the agent edits files out-of-band). It returns the document
// version now in effect, which a diagnostics wait uses to recognize fresh
// pushes. A no-op (content unchanged) returns the existing version.
func (c *client) ensureOpen(ctx context.Context, abs string) (int, error) {
	text, err := readDocument(ctx, c.workspace, abs)
	if err != nil {
		return 0, fmt.Errorf("lsp: read %s: %w", abs, err)
	}
	if !c.syncPolicy.OpenClose {
		// File queries can use the server's on-disk view after root-confined admission.
		// Captured diagnostics use ensureContent directly and require a proved version.
		return 0, nil
	}
	return c.ensureContent(ctx, abs, text)
}

func (c *client) ensureContent(ctx context.Context, abs string, text []byte) (int, error) {
	if len(text) > int(maxDocumentBytes) {
		return 0, errDocumentTooLarge
	}
	if !utf8.Valid(text) || bytes.IndexByte(text, 0) >= 0 {
		return 0, errUnsupportedDocument
	}
	if err := cancellation.Err(ctx); err != nil {
		return 0, err
	}
	var err error
	uri := pathToURI(abs)
	hash := sha256.Sum256(text)

	// Serialize version allocation and delivery so the server cannot observe
	// a newer notification before the one that authorizes it.
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, isOpen := c.open[uri]
	if isOpen && prev.hash == hash {
		return prev.version, nil
	}
	if !c.syncPolicy.OpenClose {
		return 0, errors.New("lsp: server cannot synchronize a document snapshot")
	}
	if c.version == math.MaxInt32 {
		return 0, errors.New("lsp: document version space exhausted")
	}
	version := c.version + 1
	if !isOpen {
		err = c.conn.Notify(ctx, "textDocument/didOpen", didOpenParams{
			TextDocument: textDocumentItem{URI: uri, LanguageID: c.spec.Languages[strings.ToLower(filepath.Ext(abs))], Version: version, Text: string(text)},
		})
	} else {
		change := contentChange{Text: string(text)}
		switch c.syncPolicy.Change {
		case changeNone:
			return 0, errors.New("lsp: server did not negotiate document changes")
		case changeIncremental:
			change.Range = &Range{End: prev.end}
		}
		err = c.conn.Notify(ctx, "textDocument/didChange", didChangeParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
			ContentChanges: []contentChange{change},
		})
	}
	if err != nil {
		return 0, fmt.Errorf("lsp: sync %s: %w", abs, err)
	}
	c.version = version
	doc := openDoc{version: version, hash: hash}
	if c.syncPolicy.Change == changeIncremental {
		doc.end = documentEnd(text)
	}
	c.open[uri] = doc
	c.evictColdestDocumentLocked(ctx)
	return version, nil
}

// evictColdestDocumentLocked keeps the synchronized set within
// [maxOpenDocuments] by closing the least recently synced document. The didClose
// releases the server's copy too, which is the larger of the two; a failed
// notify leaves the document recorded so the next sync retries rather than
// desynchronizing the two views.
func (c *client) evictColdestDocumentLocked(ctx context.Context) {
	if len(c.open) <= maxOpenDocuments {
		return
	}
	coldest, oldestVersion := "", 0
	for uri, doc := range c.open {
		if coldest == "" || doc.version < oldestVersion {
			coldest, oldestVersion = uri, doc.version
		}
	}
	if err := c.conn.Notify(ctx, "textDocument/didClose", didCloseParams{
		TextDocument: textDocumentIdentifier{URI: coldest},
	}); err != nil {
		return
	}
	delete(c.open, coldest)
	delete(c.diags, coldest)
}

func documentEnd(text []byte) Position {
	var end Position
	for len(text) > 0 {
		r, size := utf8.DecodeRune(text)
		text = text[size:]
		switch r {
		case '\r', '\n':
			if r == '\r' && len(text) > 0 && text[0] == '\n' {
				text = text[1:]
			}
			end.Line++
			end.Character = 0
		default:
			end.Character += utf16.RuneLen(r)
		}
	}
	return end
}

func readDocument(ctx context.Context, root *os.Root, path string) (_ []byte, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	if root == nil {
		return nil, errors.New("lsp: workspace root is closed")
	}
	name, err := filepath.Rel(root.Name(), path)
	if err != nil {
		return nil, err
	}
	file, opened, err := filesystem.OpenFileAt(root, name, maxDocumentBytes)
	if err != nil {
		switch {
		case errors.Is(err, filesystem.ErrNotRegular):
			return nil, fmt.Errorf("%w: %w", errUnsupportedDocument, err)
		case errors.Is(err, filesystem.ErrFileTooLarge):
			return nil, fmt.Errorf("%w: %w", errDocumentTooLarge, err)
		default:
			return nil, err
		}
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	content, err := io.ReadAll(io.LimitReader(cancellation.Reader(ctx, file), maxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > int(maxDocumentBytes) {
		return nil, fmt.Errorf("%w: file grew while reading", errDocumentTooLarge)
	}
	if err := filesystem.VerifyAtVersion(file, opened, root, name); err != nil {
		return nil, fmt.Errorf("lsp: verify document after reading: %w", err)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, errUnsupportedDocument
	}
	return content, nil
}
