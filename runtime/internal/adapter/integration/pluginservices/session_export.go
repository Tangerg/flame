package pluginservices

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

func (c *CommandCalls) RenderSessionMarkdown(ctx context.Context, sessionID string, document []byte) (string, error) {
	contract, err := pluginsdk.SessionMarkdownContract()
	if err != nil {
		return "", err
	}
	location, borrow, err := contributionSession(ctx, c.framework, c.installations, c.selections, c.pool, contract)
	if err != nil {
		return "", err
	}
	defer borrow.Close()
	ctx = borrow.Context()
	actor, err := operation.Capture(ctx, portable.Target{Scope: string(extension.ComponentScopeSession), Reference: sessionID})
	if err != nil {
		return "", err
	}
	ctx, err = qimumcp.WithInvocation(ctx, actor)
	if err != nil {
		return "", err
	}
	identity := pluginsdk.IdentifyDocument(document)
	if err := identity.Validate(); err != nil {
		return "", err
	}
	read, err := pluginsdk.ReadDocumentRangeMethod()
	if err != nil {
		return "", err
	}
	callback, err := qimumcp.NewCallback(read, nil, func(ctx context.Context, req qimumcp.Command[pluginsdk.DocumentRange]) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return identity.Read(document, req.Input)
	})
	if err != nil {
		return "", err
	}
	session := borrow.Session()
	grant, release, err := c.pool.OpenCallbacks(ctx, location.ServerID, session, callback)
	if err != nil {
		return "", err
	}
	defer release()
	service, err := qimumcp.ResolveCapability(ctx, session, location.Export, contract)
	if err != nil {
		return "", err
	}
	render, err := pluginsdk.RenderSessionMarkdownMethod()
	if err != nil {
		return "", err
	}
	method, err := service.Method(render.Descriptor().Name)
	if err != nil {
		return "", err
	}
	input, err := jsonvalue.Encode(identity)
	if err != nil {
		return "", err
	}
	return readRenderedDocument(c.pool.Stream(ctx, location.ServerID, session, method, "", input, grant))
}

func readRenderedDocument(source iter.Seq2[qimumcp.StreamValue[jsontext.Value, jsontext.Value], error]) (string, error) {
	var body bytes.Buffer
	var finished bool
	for value, err := range source {
		if err != nil {
			return "", err
		}
		if finished {
			return "", qimumcp.ErrStreamUnconfirmed
		}
		switch {
		case value.Chunk != nil:
			var chunk string
			if err := json.Unmarshal(*value.Chunk, &chunk); err != nil {
				return "", err
			}
			if len(chunk) == 0 || len(chunk) > pluginsdk.SessionDocumentChunkBytes || len(chunk) > protocol.MaximumSessionArtifactBytes-body.Len() {
				return "", errors.New("session export: invalid rendered chunk")
			}
			body.WriteString(chunk)
		case value.Result != nil:
			var result pluginsdk.DocumentIdentity
			if err := json.Unmarshal(*value.Result, &result, json.RejectUnknownMembers(true)); err != nil {
				return "", err
			}
			if result != pluginsdk.IdentifyDocument(body.Bytes()) {
				return "", qimumcp.ErrStreamUnconfirmed
			}
			finished = true
		default:
			return "", qimumcp.ErrStreamUnconfirmed
		}
	}
	if !finished {
		return "", qimumcp.ErrStreamUnconfirmed
	}
	return body.String(), nil
}
