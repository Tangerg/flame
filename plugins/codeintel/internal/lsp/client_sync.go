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

	"github.com/Tangerg/flame/packages/go-sdk/capture"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

const maxDocumentBytes int64 = 8 << 20

const maxOpenDocuments = 128

var errDocumentTooLarge = fmt.Errorf(
	"lsp: document exceeds the %d MiB limit", maxDocumentBytes/filesystem.MiB)

var errUnsupportedDocument = errors.New("lsp: document is not a regular UTF-8 text file")

func (c *client) ensureOpen(ctx context.Context, abs string) error {
	text, err := readDocument(ctx, c.workspace, abs)
	if err != nil {
		return fmt.Errorf("lsp: read %s: %w", abs, err)
	}
	if !c.syncPolicy.OpenClose {
		return nil
	}
	_, err = c.ensureContent(ctx, abs, text)
	return err
}

func admitDocumentText(text []byte) error {
	if len(text) > int(maxDocumentBytes) {
		return errDocumentTooLarge
	}
	if !utf8.Valid(text) || bytes.IndexByte(text, 0) >= 0 {
		return errUnsupportedDocument
	}
	return nil
}

func (c *client) ensureContent(ctx context.Context, abs string, text []byte) (int, error) {
	if err := admitDocumentText(text); err != nil {
		return 0, err
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return 0, err
	}
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
	if isOpen && c.syncPolicy.Change == changeNone {
		return 0, errors.New("lsp: server did not negotiate document changes")
	}
	version := c.version + 1
	var err error
	if isOpen {
		err = c.changeDocumentLocked(ctx, uri, text, version, prev)
	} else {
		err = c.openDocumentLocked(ctx, uri, abs, text, version)
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
	return version, nil
}

func (c *client) openDocumentLocked(ctx context.Context, uri, abs string, text []byte, version int) error {
	if err := c.evictColdestDocumentLocked(ctx); err != nil {
		return err
	}
	return c.notify(ctx, "textDocument/didOpen", didOpenParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: c.spec.Languages[strings.ToLower(filepath.Ext(abs))], Version: version, Text: string(text)},
	})
}

func (c *client) changeDocumentLocked(ctx context.Context, uri string, text []byte, version int, prev openDoc) error {
	change := contentChange{Text: string(text)}
	if c.syncPolicy.Change == changeIncremental {
		change.Range = &Range{End: prev.end}
	}
	return c.notify(ctx, "textDocument/didChange", didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: []contentChange{change},
	})
}

func (c *client) evictColdestDocumentLocked(ctx context.Context) error {
	if len(c.open) < maxOpenDocuments {
		return nil
	}
	coldest, oldestVersion := "", 0
	for uri, doc := range c.open {
		if coldest == "" || doc.version < oldestVersion {
			coldest, oldestVersion = uri, doc.version
		}
	}
	if err := c.notify(ctx, "textDocument/didClose", didCloseParams{
		TextDocument: textDocumentIdentifier{URI: coldest},
	}); err != nil {
		return fmt.Errorf("lsp: close %s before document admission: %w", coldest, err)
	}
	delete(c.open, coldest)
	delete(c.diags, coldest)
	return nil
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
	contextReader, err := capture.NewReader(ctx, file)
	if err != nil {
		return nil, err
	}
	content, err := io.ReadAll(io.LimitReader(contextReader, maxDocumentBytes+1))
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
