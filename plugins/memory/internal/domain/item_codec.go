package agentmemory

import (
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/frame/value"
)

type itemProjection struct {
	ID             ItemID
	Revision       uint64 `jsonschema:"minimum=1"`
	Scope          Scope
	Project        string
	Content        string `jsonschema:"pattern=\\S"`
	Origin         Origin
	Status         Status
	Pinned         bool
	SessionID      string
	Day            string
	CreatedAt      instant.Timestamp `jsonschema:"format=date-time"`
	UpdatedAt      instant.Timestamp `jsonschema:"format=date-time"`
	EmbeddingSpace string
	Embedding      []value.Number
}

// The Item owner uses native instants and binary32 search vectors. This
// projection declares their actual JSON encodings to Frame without replacing
// the Item's state or deriving a second schema for its business constraints.
func (ItemSnapshot) JSONSchemaModel() any { return itemProjection{} }

func (s ItemSnapshot) MarshalJSON() ([]byte, error) {
	type native ItemSnapshot
	return json.Marshal(native(s))
}

func (s *ItemSnapshot) UnmarshalJSON(data []byte) error {
	var projection itemProjection
	if err := value.DecodeValue(data, &projection, "memory item"); err != nil {
		return err
	}
	type native ItemSnapshot
	var decoded native
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	*s = ItemSnapshot(decoded)
	return nil
}
