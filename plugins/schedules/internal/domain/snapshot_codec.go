package schedule

import (
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/frame/value"
)

type snapshotProjection struct {
	ID             string
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelchoice.Selection
	Cron           string
	TimeZone       string
	LastRunAt      instant.Timestamp `json:",omitzero" jsonschema:"format=date-time"`
	NextRunAt      instant.Timestamp `json:",omitzero" jsonschema:"format=date-time"`
	CreatedAt      instant.Timestamp `jsonschema:"format=date-time"`
	Revision       uint64
}

func (Snapshot) JSONSchemaModel() any { return snapshotProjection{} }

func (s Snapshot) MarshalJSON() ([]byte, error) {
	type native Snapshot
	return json.Marshal(native(s))
}

func (s *Snapshot) UnmarshalJSON(data []byte) error {
	var projection snapshotProjection
	if err := value.DecodeValue(data, &projection, "schedule snapshot"); err != nil {
		return err
	}
	type native Snapshot
	var decoded native
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	*s = Snapshot(decoded)
	return nil
}
