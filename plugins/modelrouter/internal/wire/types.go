package wire

import (
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
	"github.com/invopop/jsonschema"
)

type ReplaceInput struct {
	ExpectedRevision int64          `json:"expectedRevision" jsonschema:"minimum=1"`
	Rules            []routing.Rule `json:"rules"`
}

func (ReplaceInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	rules, _ := schema.Properties.Get("rules")
	rules.MaxItems = new(uint64(routing.MaxRules))
}
