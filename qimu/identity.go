package qimu

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid          = errors.New("qimu: invalid value")
	ErrClosed           = errors.New("qimu: lifetime closed")
	ErrStaleBinding     = errors.New("qimu: stale binding")
	ErrUnavailable      = errors.New("qimu: unavailable")
	ErrInUse            = errors.New("qimu: installation in use")
	ErrAmbiguous        = errors.New("qimu: ambiguous provider")
	ErrDependencyCycle  = errors.New("qimu: dependency cycle")
	ErrContractMismatch = errors.New("qimu: contract mismatch")
	ErrRecoveryRequired = errors.New("qimu: recovery required")
	ErrFenced           = errors.New("qimu: scope fenced")
	ErrCapacity         = errors.New("qimu: capacity exceeded")
	ErrResyncRequired   = errors.New("qimu: resynchronization required")
)

type InstallationID string
type UnitID string
type ScopeID string
type RealmID string
type Profile string
type Digest string

const MaxNameBytes = 256

const (
	NativeGo       Profile = "native-go"
	MCPProcess     Profile = "mcp-process"
	BrowserJS      Profile = "browser-js"
	StaticResource Profile = "static-resource"
)

func (Profile) Values() []string {
	return []string{string(NativeGo), string(MCPProcess), string(BrowserJS), string(StaticResource)}
}

func ValidName(value string) bool {
	return value != "" && len(value) <= MaxNameBytes && utf8.ValidString(value) &&
		!strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

func validateName(kind, value string) error {
	if !ValidName(value) {
		return fmt.Errorf("%w: %s", ErrInvalid, kind)
	}
	return nil
}

func (id InstallationID) Validate() error { return validateName("installation identity", string(id)) }
func (id UnitID) Validate() error         { return validateName("unit identity", string(id)) }
func (id ScopeID) Validate() error        { return validateName("scope identity", string(id)) }
func (id RealmID) Validate() error        { return validateName("realm identity", string(id)) }
func (p Profile) Validate() error         { return validateName("execution profile", string(p)) }
func (id InstallationID) String() string  { return string(id) }
func (id UnitID) String() string          { return string(id) }
func (id ScopeID) String() string         { return string(id) }
func (id RealmID) String() string         { return string(id) }
func (d Digest) String() string           { return string(d) }

func ParseInstallationID(value string) (InstallationID, error) {
	id := InstallationID(value)
	return id, id.Validate()
}

func Hash(value []byte) Digest {
	digest := sha256.Sum256(value)
	return Digest(hex.EncodeToString(digest[:]))
}

func (d Digest) Validate() error {
	if len(d) != 64 {
		return fmt.Errorf("%w: sha256 digest", ErrInvalid)
	}
	for _, b := range []byte(d) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return fmt.Errorf("%w: sha256 digest", ErrInvalid)
		}
	}
	return nil
}

func ParseDigest(value string) (Digest, error) {
	d := Digest(value)
	return d, d.Validate()
}
