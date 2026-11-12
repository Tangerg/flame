package client

import (
	"fmt"
	"strings"

	"golang.org/x/net/http/httpguts"
)

const headerAuthorization = "Authorization"

func validateHeaderValue(name, value string) error {
	if !httpguts.ValidHeaderFieldValue(value) || strings.Trim(value, " \t") != value {
		return fmt.Errorf("runtime: %s header is not representable without modification", name)
	}
	return nil
}
