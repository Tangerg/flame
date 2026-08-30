// Package commandreplay owns the CLI's model of one Runtime command replay
// store and the finite interval it promises for stable command identities.
package commandreplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Capability is the immutable replay promise published by one Runtime. Its
// zero value is invalid: store identity and retention are one indivisible fact.
type Capability struct {
	namespace string
	retention time.Duration
}

func NewCapability(namespace string, retention time.Duration) (Capability, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return Capability{}, errors.New("command replay namespace is empty")
	}
	if retention <= 0 {
		return Capability{}, errors.New("command replay retention is not positive")
	}
	if retention%time.Second != 0 {
		return Capability{}, errors.New("command replay retention is not a whole number of seconds")
	}
	return Capability{namespace: namespace, retention: retention}, nil
}

func (c Capability) Validate() error {
	if strings.TrimSpace(c.namespace) == "" || c.namespace != strings.TrimSpace(c.namespace) {
		return errors.New("command replay namespace is empty or has surrounding whitespace")
	}
	if c.retention <= 0 || c.retention%time.Second != 0 {
		return errors.New("command replay retention must be positive whole seconds")
	}
	return nil
}

func (c Capability) Namespace() string { return c.namespace }

func (c Capability) Retention() time.Duration { return c.retention }

func (c Capability) Deadline(stagedAt time.Time) (time.Time, error) {
	if err := c.Validate(); err != nil {
		return time.Time{}, err
	}
	if stagedAt.IsZero() {
		return time.Time{}, errors.New("command replay staging time is empty")
	}
	return stagedAt.UTC().Add(c.retention), nil
}

type capabilityJSON struct {
	Namespace        string `json:"namespace"`
	RetentionSeconds int64  `json:"retentionSeconds"`
}

func (c Capability) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(capabilityJSON{
		Namespace: c.namespace, RetentionSeconds: int64(c.retention / time.Second),
	})
}

func (c *Capability) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode command replay capability into nil receiver")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire capabilityJSON
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("decode command replay capability: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode command replay capability: trailing JSON value")
		}
		return fmt.Errorf("decode command replay capability trailing data: %w", err)
	}
	if wire.RetentionSeconds <= 0 || wire.RetentionSeconds > int64((time.Duration(1<<63-1))/time.Second) {
		return errors.New("command replay retentionSeconds is outside the positive duration range")
	}
	capability, err := NewCapability(
		wire.Namespace, time.Duration(wire.RetentionSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*c = capability
	return nil
}
