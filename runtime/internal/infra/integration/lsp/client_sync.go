package lsp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/cancelread"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

const maxDocumentBytes int64 = 8 << 20

// maxOpenDocuments bounds the synchronized document set. A client is opened once
// per language server and lives as long as the runtime, so without a bound both
// this map and the server's own copy of every file the agent ever inspected grow
// for the life of the process. Evicting the least recently synced document costs
// one didOpen when it is next touched.
const maxOpenDocuments = 128

// ErrDocumentTooLarge reports a workspace document that cannot be admitted to
// the in-memory language-server synchronization boundary.
var ErrDocumentTooLarge = fmt.Errorf(
	"lsp: document exceeds the %d MiB limit", maxDocumentBytes>>20)

// ErrUnsupportedDocument reports a source that cannot be represented as one
// Language Server Protocol text document.
var ErrUnsupportedDocument = errors.New("lsp: document is not a regular UTF-8 text file")

// ensureOpen makes the server aware of abs's current on-disk content: a
// didOpen the first time, a didChange when the content has changed since we
// last synced (the agent edits files out-of-band). It returns the document
// version now in effect, which a diagnostics wait uses to recognize fresh
// pushes. A no-op (content unchanged) returns the existing version.
func (c *client) ensureOpen(ctx context.Context, abs string) (int, error) {
	text, err := readDocument(ctx, abs)
	if err != nil {
		return 0, fmt.Errorf("lsp: read %s: %w", abs, err)
	}
	uri := pathToURI(abs)
	hash := sha256.Sum256(text)

	// Hold c.mu across the Notify so the version bump and its didOpen/didChange
	// are atomic PER DOCUMENT. Two concurrent ensureOpen on the same file — calls
	// to the `lsp` operation tool share one parallel segment
	// (concurrent scheduling policy) and hit this shared client — would otherwise compute
	// v1 and v2 under the lock, release, then race the Notify: the server could
	// see didChange(v2) before didOpen(v1), or versions out of order, and desync
	// its in-memory document for the rest of the session. Notify is a buffered,
	// non-blocking write whose completion doesn't depend on the inbound
	// diagnostics handler (a separate goroutine), so holding the lock across it
	// can't deadlock. The map is updated only AFTER a successful send, so a failed
	// Notify doesn't record a version the server never saw.
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, isOpen := c.open[uri]
	if isOpen && prev.hash == hash {
		return prev.version, nil
	}
	version := prev.version + 1
	if !isOpen {
		err = c.conn.Notify(ctx, "textDocument/didOpen", didOpenParams{
			TextDocument: textDocumentItem{URI: uri, LanguageID: c.spec.LanguageID, Version: version, Text: string(text)},
		})
	} else {
		err = c.conn.Notify(ctx, "textDocument/didChange", didChangeParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
			ContentChanges: []contentChange{{Text: string(text)}},
		})
	}
	if err != nil {
		return 0, fmt.Errorf("lsp: sync %s: %w", abs, err)
	}
	c.synced++
	c.open[uri] = openDoc{version: version, hash: hash, synced: c.synced}
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
	coldest, coldestSynced := "", uint64(0)
	for uri, doc := range c.open {
		if coldest == "" || doc.synced < coldestSynced {
			coldest, coldestSynced = uri, doc.synced
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

func readDocument(ctx context.Context, path string) (_ []byte, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	file, opened, err := fileinput.Open(path, maxDocumentBytes)
	if err != nil {
		switch {
		case errors.Is(err, fileinput.ErrNotRegular):
			return nil, ErrUnsupportedDocument
		case errors.Is(err, fileinput.ErrTooLarge):
			return nil, fmt.Errorf("%w: source exceeds %d bytes", ErrDocumentTooLarge, maxDocumentBytes)
		default:
			return nil, err
		}
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	content, err := io.ReadAll(io.LimitReader(cancelread.Reader(ctx, file), maxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > int(maxDocumentBytes) {
		return nil, fmt.Errorf("%w: file grew while reading", ErrDocumentTooLarge)
	}
	if err := fileinput.VerifyPathVersion(file, opened, path); err != nil {
		if errors.Is(err, fileinput.ErrChanged) {
			return nil, errors.New("lsp: document changed while it was being read")
		}
		return nil, fmt.Errorf("lsp: verify document after reading: %w", err)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, ErrUnsupportedDocument
	}
	return content, nil
}
