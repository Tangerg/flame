package main

import (
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"unicode/utf8"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type markdownEvent = pluginmcp.StreamValue[string, plugin.DocumentIdentity]

func renderMarkdown(ctx context.Context, _ *sdk.ServerSession, cmd pluginmcp.Command[plugin.DocumentIdentity]) iter.Seq2[markdownEvent, error] {
	return func(yield func(markdownEvent, error) bool) {
		grant, err := pluginmcp.CallbackFromContext(ctx)
		if err != nil {
			yield(markdownEvent{}, err)
			return
		}
		method, err := plugin.ReadDocumentRangeMethod()
		if err != nil {
			yield(markdownEvent{}, err)
			return
		}
		source, err := readSessionSnapshot(ctx, cmd.Input, func(r plugin.DocumentRange) (string, error) { return pluginmcp.CallHost(ctx, grant, method, "", r) })
		if err != nil {
			yield(markdownEvent{}, err)
			return
		}
		body, err := renderSessionMarkdown(source.Session, source.Items, source.ToolResults)
		if err != nil {
			yield(markdownEvent{}, err)
			return
		}
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
	numbers, _ := json.GetOption(framevalue.Numbers(), json.WithUnmarshalers)
	decoders := json.JoinUnmarshalers(numbers, json.UnmarshalFromFunc(decodeOpaqueNull))
	if err := json.Unmarshal(body, &archive, json.RejectUnknownMembers(true), json.WithUnmarshalers(decoders)); err != nil {
		return archive, err
	}
	if archive.Version != protocol.SessionArtifactVersion {
		return archive, fmt.Errorf("trajectory: unsupported archive version %d", archive.Version)
	}
	return archive, nil
}

// A present JSON null result is evidence; nil would instead be omitted by the
// public invocation codec and incorrectly read as an unrecorded result.
func decodeOpaqueNull(decoder *jsontext.Decoder, target *any) error {
	if decoder.PeekKind() != 'n' {
		return errors.ErrUnsupported
	}
	value, err := decoder.ReadValue()
	if err != nil {
		return err
	}
	*target = value.Clone()
	return nil
}
