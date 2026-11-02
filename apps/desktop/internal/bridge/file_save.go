package bridge

import (
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaximumExportChunkBytes = 1 << 20

// DecodeFileSave refines the native IPC input before any save panel is opened.
// The suggested name never confers authority over a filesystem destination.
func DecodeFileSave(suggestedFilename, contentBase64 string) ([]byte, error) {
	if err := ValidateFileName(suggestedFilename); err != nil {
		return nil, err
	}
	return DecodeExportChunk(contentBase64)
}
func ValidateFileName(suggestedFilename string) error {
	if !utf8.ValidString(suggestedFilename) {
		return errors.New("filename is not valid UTF-8")
	}
	if suggestedFilename == "" || suggestedFilename == "." || suggestedFilename == ".." {
		return errors.New("filename must name a file")
	}
	if strings.ContainsAny(suggestedFilename, `/\:`) {
		return errors.New("filename must be a basename")
	}
	if strings.IndexFunc(suggestedFilename, unicode.IsControl) >= 0 {
		return errors.New("filename contains control characters")
	}
	return nil
}
func DecodeExportChunk(contentBase64 string) ([]byte, error) {
	if len(contentBase64) > base64.StdEncoding.EncodedLen(MaximumExportChunkBytes) {
		return nil, errors.New("file exceeds native export capacity")
	}
	contents, err := base64.StdEncoding.Strict().DecodeString(contentBase64)
	if err != nil || len(contents) > MaximumExportChunkBytes || strings.ContainsAny(contentBase64, "\r\n") {
		return nil, errors.New("contents must be canonical base64")
	}
	return contents, nil
}
