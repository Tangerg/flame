package bridge

import (
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DecodeFileSave refines the native IPC input before any save panel is opened.
// The suggested name never confers authority over a filesystem destination.
func DecodeFileSave(suggestedFilename, contentBase64 string) ([]byte, error) {
	if !utf8.ValidString(suggestedFilename) {
		return nil, errors.New("filename is not valid UTF-8")
	}
	if suggestedFilename == "" || suggestedFilename == "." || suggestedFilename == ".." {
		return nil, errors.New("filename must name a file")
	}
	if strings.ContainsAny(suggestedFilename, `/\:`) {
		return nil, errors.New("filename must be a basename")
	}
	if strings.IndexFunc(suggestedFilename, unicode.IsControl) >= 0 {
		return nil, errors.New("filename contains control characters")
	}
	contents, err := base64.StdEncoding.Strict().DecodeString(contentBase64)
	if err != nil || base64.StdEncoding.EncodeToString(contents) != contentBase64 {
		return nil, errors.New("contents must be canonical base64")
	}
	return contents, nil
}
