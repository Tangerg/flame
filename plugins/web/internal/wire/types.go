package wire

import (
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/scope/core/jsonschema"
)

type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	Title         string            `json:"title"`
	URL           string            `json:"url"`
	Snippet       string            `json:"snippet"`
	FaviconURL    string            `json:"favicon_url,omitempty"`
	PublishedTime instant.Timestamp `json:"published_time,omitzero"`
	Source        string            `json:"source,omitempty"`
}

type BrowseInput struct {
	SnapshotID string `json:"snapshotId" jsonschema:"minLength=64,maxLength=64"`
	Offset     int64  `json:"offset" jsonschema:"minimum=0"`
	Limit      int64  `json:"limit" jsonschema:"minimum=1"`
}

func (BrowseInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"limit": {Maximum: new(float64(capture.MaxPageRunes))},
	}}
}

// HTTPResponse projects Scope's byte-preserving HTTP result into the published contract.
type HTTPResponse struct {
	Status    int                      `json:"status"`
	Headers   map[string][]HTTPContent `json:"headers,omitempty"`
	Body      HTTPContent              `json:"body"`
	Truncated bool                     `json:"truncated,omitzero"`
	Duration  string                   `json:"duration"`
}

type HTTPContent struct {
	Encoding string `json:"encoding" jsonschema:"enum=utf8,enum=base64"`
	Data     string `json:"data"`
}
