package plugin

import (
	"encoding/base64"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
)

const SessionMarkdownReference = "flame.session-markdown.v1"
const SessionDocumentChunkBytes = 64 << 10

type DocumentIdentity struct {
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}

func IdentifyDocument(body []byte) DocumentIdentity {
	return DocumentIdentity{Digest: framevalue.Hash(body).String(), Bytes: int64(len(body))}
}

func (d DocumentIdentity) Validate() error {
	if _, err := framevalue.ParseDigest(d.Digest); err != nil {
		return err
	}
	if d.Bytes < 0 || d.Bytes > protocol.MaximumSessionArtifactBytes {
		return errors.New("session export: document exceeds its allowed range")
	}
	return nil
}

type DocumentRange struct {
	Digest string `json:"digest"`
	Offset int64  `json:"offset"`
}

func (d DocumentIdentity) Read(source []byte, r DocumentRange) (string, error) {
	if r.Digest != d.Digest || int64(len(source)) != d.Bytes || r.Offset < 0 || r.Offset >= d.Bytes || r.Offset%SessionDocumentChunkBytes != 0 {
		return "", errors.New("session export: range is not available in this snapshot")
	}
	return base64.StdEncoding.EncodeToString(source[r.Offset:min(r.Offset+SessionDocumentChunkBytes, d.Bytes)]), nil
}

func ReadDocumentRangeMethod() (*pluginmcp.Method[DocumentRange, string], error) {
	return pluginmcp.NewMethod[DocumentRange, string](frame.MethodConfig{Name: "flame.session-document/read-range.v1", Kind: frame.Query, Target: extension.ScopeCall, Description: "Read a base64 range of the original immutable session document for this call."})
}

func RenderSessionMarkdownMethod() (*pluginmcp.StreamingMethod[DocumentIdentity, string, DocumentIdentity], error) {
	return pluginmcp.NewStreamingMethod[DocumentIdentity, string, DocumentIdentity](frame.MethodConfig{Name: "flame.session-markdown/render.v1", Kind: frame.Query, Target: string(extension.ComponentScopeSession), Description: "Render a coherent session snapshot into ordered Markdown fragments and report the completed document identity."})
}

func SessionMarkdownContract() (*plugincontract.Capability, error) {
	method, err := RenderSessionMarkdownMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: SessionMarkdownReference, Composition: plugincontract.Selection, Scope: string(extension.ComponentScopeSession)}, method.Descriptor())
}

func ValidateRenderedChunk(previous, size int) error {
	if previous < 0 || size <= 0 || size > SessionDocumentChunkBytes || size > protocol.MaximumSessionArtifactBytes-previous {
		return errors.New("session export: invalid rendered chunk")
	}
	return nil
}
