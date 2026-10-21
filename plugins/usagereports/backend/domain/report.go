package reporting

import (
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/jsoninput"
)

const MaxReports = 64
const MaxReportBytes = 1 << 20

var (
	ErrInvalid  = errors.New("usage reports: invalid captured report")
	ErrCapacity = errors.New("usage reports: archive capacity reached; remove a saved report before capturing another")
	ErrNotFound = errors.New("usage reports: saved report was not found")
)

type Period string

const (
	Daily  Period = "daily"
	Weekly Period = "weekly"
)

func (Period) JSONSchemaEnum() []string { return []string{string(Daily), string(Weekly)} }

func (p Period) Days() (int, error) {
	switch p {
	case Daily:
		return 1, nil
	case Weekly:
		return 7, nil
	default:
		return 0, ErrInvalid
	}
}

type Snapshot struct {
	ID      string          `json:"id"`
	Command string          `json:"command"`
	Period  Period          `json:"period"`
	Value   jsoninput.Input `json:"value"`
}

type Report struct{ snapshot Snapshot }

func NewReport(command string, period Period, value jsoninput.Input) (Report, error) {
	raw := value.JSON()
	if command == "" || len(command) > 256 || !utf8.ValidString(command) || value.Validate() != nil || len(raw) > MaxReportBytes || len(raw) == 0 || raw[0] != '{' {
		return Report{}, ErrInvalid
	}
	if _, err := period.Days(); err != nil {
		return Report{}, err
	}
	encoded, err := json.Marshal(struct {
		Command string
		Period  Period
		Value   jsoninput.Input
	}{command, period, value}, json.Deterministic(true))
	if err != nil {
		return Report{}, err
	}
	return Report{snapshot: Snapshot{ID: fmt.Sprintf("%x", sha256.Sum256(encoded)), Command: command, Period: period, Value: value}}, nil
}

func Restore(snapshot Snapshot) (Report, error) {
	report, err := NewReport(snapshot.Command, snapshot.Period, snapshot.Value)
	if err != nil || report.snapshot.ID != snapshot.ID {
		return Report{}, errors.Join(ErrInvalid, err)
	}
	return report, nil
}

func (r Report) Snapshot() Snapshot { return r.snapshot }
