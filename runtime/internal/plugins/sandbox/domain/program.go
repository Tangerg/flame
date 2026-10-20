package domain

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxScriptBytes = 64 << 10
const MaxTimeoutMillis = 30_000

var ErrInvalidProgram = errors.New("sandbox: script must be nonblank UTF-8 without NUL, at most 65536 bytes, with a timeout between 1 and 30000 milliseconds")

type Program struct {
	script  string
	timeout time.Duration
}

func NewProgram(script string, timeoutMillis int) (Program, error) {
	if strings.TrimSpace(script) == "" || len(script) > MaxScriptBytes || !utf8.ValidString(script) || strings.ContainsRune(script, 0) || timeoutMillis < 1 || timeoutMillis > MaxTimeoutMillis {
		return Program{}, ErrInvalidProgram
	}
	return Program{script: script, timeout: time.Duration(timeoutMillis) * time.Millisecond}, nil
}

func (p Program) Script() string         { return p.script }
func (p Program) Timeout() time.Duration { return p.timeout }

type Encoding string

const (
	UTF8   Encoding = "utf8"
	Base64 Encoding = "base64"
)

func (Encoding) JSONSchemaEnum() []string { return []string{string(UTF8), string(Base64)} }

type Output struct {
	Encoding Encoding `json:"encoding"`
	Content  string   `json:"content"`
}

func Capture(raw []byte) Output {
	if utf8.Valid(raw) {
		return Output{Encoding: UTF8, Content: string(raw)}
	}
	return Output{Encoding: Base64, Content: base64.StdEncoding.EncodeToString(raw)}
}

type Status string

const (
	Completed Status = "completed"
	TimedOut  Status = "timed_out"
)

func (Status) JSONSchemaEnum() []string { return []string{string(Completed), string(TimedOut)} }

type Result struct {
	Status         Status `json:"status"`
	ExitCode       int    `json:"exitCode"`
	DurationMillis int64  `json:"durationMillis" jsonschema:"minimum=0"`
	Stdout         Output `json:"stdout"`
	Stderr         Output `json:"stderr"`
}

func (r Result) Validate() error {
	if (r.Status != Completed && r.Status != TimedOut) || r.DurationMillis < 0 {
		return errors.New("sandbox: invalid captured execution result")
	}
	for _, output := range []Output{r.Stdout, r.Stderr} {
		var raw []byte
		switch output.Encoding {
		case UTF8:
			raw = []byte(output.Content)
		case Base64:
			var err error
			raw, err = base64.StdEncoding.DecodeString(output.Content)
			if err != nil {
				return errors.New("sandbox: invalid captured output encoding")
			}
		default:
			return errors.New("sandbox: invalid captured output encoding")
		}
		if Capture(raw) != output {
			return errors.New("sandbox: noncanonical captured output")
		}
	}
	return nil
}
