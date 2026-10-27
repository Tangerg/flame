package wire

import (
	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"
)

type ConfigureInput struct {
	ExpectedRevision int64            `json:"expectedRevision" jsonschema:"minimum=1"`
	Trigger          *filewatch.Draft `json:"trigger,omitzero"`
}

type EventInput struct {
	ID string `json:"id" jsonschema:"minLength=1,maxLength=256"`
}
