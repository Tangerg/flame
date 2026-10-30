package wire

import (
	json "encoding/json"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/invopop/jsonschema"
	"strconv"
)

type BrowseInput struct {
	SnapshotID string `json:"snapshotId" jsonschema:"minLength=64,maxLength=64"`
	Offset     int64  `json:"offset" jsonschema:"minimum=0"`
	Limit      int64  `json:"limit" jsonschema:"minimum=1"`
}

func (BrowseInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	limit, _ := schema.Properties.Get("limit")
	limit.Maximum = json.Number(strconv.Itoa(capture.MaxPageRunes))
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
