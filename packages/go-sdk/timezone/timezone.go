package timezone

import (
	"errors"
	"os"
	"strings"
	"time"
)

var ErrUnnamed = errors.New("timezone: the host time zone has no IANA name")

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
