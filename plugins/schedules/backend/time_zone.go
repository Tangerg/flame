package main

import (
	"errors"
	"os"
	"strings"
	"time"
)

// hostTimeZone names the host's IANA zone, the person's own when the Runtime
// runs on their machine. Go loads that zone as Local without its name, so the
// name is read from where the platform records it.
func hostTimeZone() (string, error) {
	if name := strings.TrimPrefix(os.Getenv("TZ"), ":"); name != "" {
		if _, err := time.LoadLocation(name); err == nil {
			return name, nil
		}
	}
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return "", errors.New("the host time zone has no IANA name; set time_zone")
	}
	_, name, found := strings.Cut(target, "zoneinfo/")
	if !found {
		return "", errors.New("the host time zone has no IANA name; set time_zone")
	}
	if _, err := time.LoadLocation(name); err != nil {
		return "", errors.New("the host time zone has no IANA name; set time_zone")
	}
	return name, nil
}
