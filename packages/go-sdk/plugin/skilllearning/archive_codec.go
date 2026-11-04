package skilllearning

import (
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/frame/value"
)

type archiveRequestProjection struct {
	Now instant.Timestamp `json:"now"`
}

type archiveAdviceProjection struct {
	InactiveSince *instant.Timestamp `json:"inactiveSince,omitzero"`
}

// Archival uses the caller's captured native instant. The declaration exposes
// its standard JSON representation without introducing another clock.
func (ArchiveRequest) JSONSchemaModel() any { return archiveRequestProjection{} }
func (ArchiveAdvice) JSONSchemaModel() any  { return archiveAdviceProjection{} }

func (r ArchiveRequest) MarshalJSON() ([]byte, error) {
	type native ArchiveRequest
	return json.Marshal(native(r))
}

func (r *ArchiveRequest) UnmarshalJSON(data []byte) error {
	var projection archiveRequestProjection
	if err := value.DecodeValue(data, &projection, "archive request"); err != nil {
		return err
	}
	*r = ArchiveRequest{Now: projection.Now.Time}
	return nil
}

func (a ArchiveAdvice) MarshalJSON() ([]byte, error) {
	type native ArchiveAdvice
	return json.Marshal(native(a))
}

func (a *ArchiveAdvice) UnmarshalJSON(data []byte) error {
	var projection archiveAdviceProjection
	if err := value.DecodeValue(data, &projection, "archive advice"); err != nil {
		return err
	}
	decoded := ArchiveAdvice{}
	if projection.InactiveSince != nil {
		decoded.InactiveSince = new(projection.InactiveSince.Time)
	}
	*a = decoded
	return nil
}
