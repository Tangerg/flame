// Package timezone names the host's IANA time zone for clients that record a
// zone on the person's behalf.
package timezone

import (
	"errors"
	"os"
	"strings"
	"time"
)

// ErrUnnamed reports a host whose zone cannot be named; callers must ask for one.
var ErrUnnamed = errors.New("timezone: the host time zone has no IANA name")

// Host returns the IANA name of the host zone. Go loads that zone as Local
// without its name, so the name is read from where the platform records it.
func Host() (string, error) {
	if name := strings.TrimPrefix(os.Getenv("TZ"), ":"); name != "" {
		if _, err := time.LoadLocation(name); err == nil {
			return name, nil
		}
	}
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return "", ErrUnnamed
	}
	_, name, found := strings.Cut(target, "zoneinfo/")
	if !found {
		return "", ErrUnnamed
	}
	if _, err := time.LoadLocation(name); err != nil {
		return "", ErrUnnamed
	}
	return name, nil
}
