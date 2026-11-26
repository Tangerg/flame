package skillauthoring

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

type publicationState string

const (
	publicationPrepared  publicationState = "prepared"
	publicationPublished publicationState = "published"
	publicationRefused   publicationState = "refused"
)

type publicationProof struct {
	FileID   string `json:"fileId"`
	Revision string `json:"revision"`
}

type publicationRecord struct {
	Binding string            `json:"binding"`
	State   publicationState  `json:"state"`
	Proof   *publicationProof `json:"proof,omitzero"`
}

func (r publicationRecord) validate() error {
	switch r.State {
	case publicationPrepared:
		if r.Binding != "" && r.Proof != nil && r.Proof.FileID != "" && r.Proof.Revision != "" {
			return nil
		}
	case publicationPublished, publicationRefused:
		if r.Binding != "" && r.Proof == nil {
			return nil
		}
	}
	return errors.New("skill publication: invalid retained publication record")
}

type publicationReceipt struct {
	root   *os.Root
	permit string
}

func (r publicationReceipt) load() (publicationRecord, bool, error) {
	var record publicationRecord
	data, found, err := readBoundedFile(r.root, r.permit)
	if err != nil || !found {
		return record, found, err
	}
	if err := json.Unmarshal(data, &record, json.RejectUnknownMembers(true)); err != nil {
		return publicationRecord{}, false, fmt.Errorf("skill publication: decode receipt: %w", err)
	}
	return record, true, record.validate()
}

func (r publicationReceipt) save(record publicationRecord) error {
	data, err := json.Marshal(record, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return fmt.Errorf("skill publication: encode receipt: %w", err)
	}
	return replaceFile(r.root, r.permit, data, filesystem.PrivateFileMode)
}
