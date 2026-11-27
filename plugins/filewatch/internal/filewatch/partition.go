package filewatch

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"unicode/utf8"
)

const PartitionReadLimit = 2048

type PartitionState string

const (
	PartitionRead        PartitionState = "read"
	PartitionMissing     PartitionState = "missing"
	PartitionUnavailable PartitionState = "unavailable"
	PartitionTruncated   PartitionState = "truncated"
)

func (PartitionState) JSONSchemaEnum() []string {
	return []string{string(PartitionRead), string(PartitionMissing), string(PartitionUnavailable), string(PartitionTruncated)}
}

type Partition struct {
	Path   string         `json:"path"`
	State  PartitionState `json:"state"`
	Text   string         `json:"text,omitempty"`
	SHA256 string         `json:"sha256,omitempty"`
}

func ReadPartition(path, text string) Partition {
	return Partition{Path: path, State: PartitionRead, Text: text, SHA256: partitionDigest(text)}
}

func partitionDigest(text string) string {
	value := sha256.Sum256([]byte(text))
	return hex.EncodeToString(value[:])
}

func validPartitions(draft Draft, values []Partition) bool {
	if !draft.CapturePartitions {
		return len(values) == 0
	}
	if len(values) != len(draft.Paths) {
		return false
	}
	for index, value := range values {
		if value.Path != draft.Paths[index] {
			return false
		}
		switch value.State {
		case PartitionRead:
			if len(value.Text) > PartitionReadLimit || !utf8.ValidString(value.Text) || value.SHA256 != partitionDigest(value.Text) {
				return false
			}
		case PartitionMissing, PartitionUnavailable, PartitionTruncated:
			if value.Text != "" || value.SHA256 != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type partitionCoverage struct {
	Expected int         `json:"expected"`
	Read     int         `json:"read"`
	Inputs   []Partition `json:"inputs"`
}

func coverage(values []Partition) *partitionCoverage {
	if len(values) == 0 {
		return nil
	}
	result := &partitionCoverage{Expected: len(values), Inputs: values}
	for _, value := range values {
		if value.State == PartitionRead {
			result.Read++
		}
	}
	return result
}

func (c Capture) RepeatsPartitionReads(previous Capture) bool {
	if c.Kind != Changed || c.Revision != previous.Revision || len(c.Partitions) == 0 || !slices.Equal(c.Partitions, previous.Partitions) {
		return false
	}
	return !slices.ContainsFunc(c.Partitions, func(value Partition) bool {
		return value.State != PartitionMissing && value.State != PartitionRead
	})
}
