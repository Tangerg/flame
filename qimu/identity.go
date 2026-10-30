package qimu

import (
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

func ParseInstallationID(value string) (InstallationID, error) {
	id := InstallationID(value)
	return id, id.Validate()
}
