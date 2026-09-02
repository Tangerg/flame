package fileobservation

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"os"
	"strconv"
)

type fingerprint [sha256.Size]byte

type fingerprintField string

const (
	fingerprintFieldLogicalPath  fingerprintField = "logical-path"
	fingerprintFieldPhysicalPath fingerprintField = "physical-path"
	fingerprintFieldLinkTarget   fingerprintField = "link-target"
	fingerprintFieldState        fingerprintField = "state"
	fingerprintFieldError        fingerprintField = "error"
	fingerprintFieldLogicalInfo  fingerprintField = "logical-info"
	fingerprintFieldPhysicalInfo fingerprintField = "physical-info"
	fingerprintFieldChildInfo    fingerprintField = "child-file-info"
	fingerprintFieldContent      fingerprintField = "content"
)

type fingerprintState string

const (
	fingerprintStateMissing         fingerprintState = "missing"
	fingerprintStateUnresolved      fingerprintState = "unresolved"
	fingerprintStateOutsideBoundary fingerprintState = "outside-boundary"
	fingerprintStateMissingTarget   fingerprintState = "missing-target"
	fingerprintStateTooLarge        fingerprintState = "too-large"
)

// fingerprintEncoder preserves every field boundary before hashing. This is
// deliberately not delimiter-based: paths, link targets, and error text are
// external input and may contain any legal filesystem byte.
type fingerprintEncoder struct {
	digest hash.Hash
}

func newFingerprintEncoder() *fingerprintEncoder {
	return &fingerprintEncoder{digest: sha256.New()}
}

func (e *fingerprintEncoder) field(name fingerprintField, value string) {
	e.frame([]byte(name))
	e.frame([]byte(value))
}

func (e *fingerprintEncoder) state(value fingerprintState) {
	e.field(fingerprintFieldState, string(value))
}

func (e *fingerprintEncoder) fileInfo(scope fingerprintField, info os.FileInfo) {
	e.frame([]byte(scope))
	e.frame([]byte(strconv.FormatUint(uint64(info.Mode().Type()), 10)))
	e.frame([]byte(strconv.FormatInt(info.Size(), 10)))
	e.frame([]byte(strconv.FormatInt(info.ModTime().UnixNano(), 10)))
}

func (e *fingerprintEncoder) content(reader io.Reader) error {
	contentDigest := sha256.New()
	if _, err := io.Copy(contentDigest, reader); err != nil {
		return err
	}
	e.frame([]byte(fingerprintFieldContent))
	e.frame(contentDigest.Sum(nil))
	return nil
}

func (e *fingerprintEncoder) sum() fingerprint {
	var value fingerprint
	copy(value[:], e.digest.Sum(nil))
	return value
}

func (e *fingerprintEncoder) frame(value []byte) {
	var prefix [binary.MaxVarintLen64]byte
	size := binary.PutUvarint(prefix[:], uint64(len(value)))
	_, _ = e.digest.Write(prefix[:size])
	_, _ = e.digest.Write(value)
}
