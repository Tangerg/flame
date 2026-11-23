package schedule

import (
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/frame/value"
)

type snapshotProjection struct {
	ID             string               `json:"id"`
	Title          string               `json:"title"`
	Instructions   string               `json:"instructions"`
	CWD            string               `json:"cwd"`
	ModelSelection flamemodel.Selection `json:"modelSelection"`
	Cron           string               `json:"cron"`
	TimeZone       string               `json:"timeZone"`
	LastRunAt      instant.Timestamp    `json:"lastRunAt,omitzero" jsonschema:"format=date-time"`
	NextRunAt      instant.Timestamp    `json:"nextRunAt,omitzero" jsonschema:"format=date-time"`
	CreatedAt      instant.Timestamp    `json:"createdAt" jsonschema:"format=date-time"`
	Revision       uint64               `json:"revision"`
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
