package main

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
	qimumcp "github.com/Tangerg/qimu/mcp"
)

type markdownEvent = qimumcp.StreamValue[string, plugin.DocumentIdentity]

func renderMarkdown(ctx context.Context, _ *sdk.ServerSession, cmd qimumcp.Command[plugin.DocumentIdentity]) iter.Seq2[markdownEvent, error] {
	return func(yield func(markdownEvent, error) bool) {
		grant, err := qimumcp.CallbackFromContext(ctx)
		if err != nil {
			yield(markdownEvent{}, err)
			return
		}
		method, err := plugin.ReadDocumentRangeMethod()
		if err != nil {
			yield(markdownEvent{}, err)
			return
		}
		source, err := readSessionSnapshot(ctx, cmd.Input, func(r plugin.DocumentRange) (string, error) { return qimumcp.CallHost(ctx, grant, method, "", r) })
		if err != nil {
			yield(markdownEvent{}, err)
			return
		}
		body := renderSessionMarkdown(source.Session, source.Items)
		result := plugin.IdentifyDocument([]byte(body))
		if err = result.Validate(); err != nil {
			yield(markdownEvent{}, err)
			return
		}
		for len(body) > 0 {
			if err := ctx.Err(); err != nil {
				yield(markdownEvent{}, err)
				return
			}
			count := min(len(body), plugin.SessionDocumentChunkBytes)
			for count < len(body) && !utf8.RuneStart(body[count]) {
				count--
			}
			chunk := body[:count]
			if !yield(markdownEvent{Chunk: &chunk}, nil) {
				return
			}
			body = body[count:]
		}
		yield(markdownEvent{Result: &result}, nil)
	}
}

func readSessionSnapshot(ctx context.Context, id plugin.DocumentIdentity, read func(plugin.DocumentRange) (string, error)) (protocol.SessionArtifact, error) {
	var archive protocol.SessionArtifact
	if err := id.Validate(); err != nil {
		return archive, err
	}
	body := make([]byte, 0, int(id.Bytes))
	for int64(len(body)) < id.Bytes {
		if err := ctx.Err(); err != nil {
			return archive, err
		}
		encoded, err := read(plugin.DocumentRange{Digest: id.Digest, Offset: int64(len(body))})
		if err != nil {
			return archive, err
		}
		chunk, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return archive, err
		}
		if int64(len(chunk)) != min(int64(plugin.SessionDocumentChunkBytes), id.Bytes-int64(len(body))) {
			return archive, errors.New("trajectory: incomplete snapshot range")
		}
		body = append(body, chunk...)
	}
	if plugin.IdentifyDocument(body) != id {
		return archive, errors.New("trajectory: original snapshot digest mismatch")
	}
	if err := json.Unmarshal(body, &archive, json.RejectUnknownMembers(true)); err != nil {
		return archive, err
	}
	if archive.Version != protocol.SessionArtifactVersion {
		return archive, fmt.Errorf("trajectory: unsupported archive version %d", archive.Version)
	}
	return archive, nil
}
