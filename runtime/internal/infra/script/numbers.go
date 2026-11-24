package script

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
)

// JavaScript numbers are float64. A JSON number whose decimal value a float64
// cannot reproduce reaches the script as its decimal text, so a long ID or an
// exact amount is never silently rounded.
func scriptValue(value jsontext.Value) (string, error) {
	decoder := jsontext.NewDecoder(bytes.NewReader(value))
	var encoded bytes.Buffer
	encoder := jsontext.NewEncoder(&encoded)
	for {
		token, err := decoder.ReadToken()
		if errors.Is(err, io.EOF) {
			return encoded.String(), nil
		}
		if err != nil {
			return "", err
		}
		if token.Kind() == '0' && !float64Exact(token.String()) {
			token = jsontext.String(token.String())
		}
		if err := encoder.WriteToken(token); err != nil {
			return "", err
		}
	}
}

// Compare decimal values after float64's shortest round trip, the text a
// script prints, so 0.1 stays a number.
func float64Exact(number string) bool {
	native, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return false
	}
	if native == 0 {
		coefficient, _, _ := strings.Cut(strings.ToLower(number), "e")
		// An underflow must be detected before expanding an arbitrary exponent.
		return strings.Trim(coefficient, "-0.") == ""
	}
	before, ok := new(big.Rat).SetString(number)
	if !ok {
		return false
	}
	after, ok := new(big.Rat).SetString(strconv.FormatFloat(native, 'g', -1, 64))
	return ok && before.Cmp(after) == 0
}
