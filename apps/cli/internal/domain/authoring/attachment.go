package authoring

import (
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// MaxAttachmentBytes is the largest local file one authored attachment may carry.
const MaxAttachmentBytes int64 = 20 << 20

type Attachment struct {
	ID       string
	Kind     protocol.ContentBlockType
	Name     string
	Path     string
	MimeType string
	Size     int64
}

func (a Attachment) Validate() error {
	var problems []error
	if strings.TrimSpace(a.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if !slices.Contains([]protocol.ContentBlockType{protocol.ContentBlockImage, protocol.ContentBlockText}, a.Kind) {
		problems = append(problems, fmt.Errorf("kind %q is invalid", a.Kind))
	}
	if a.Kind == protocol.ContentBlockImage && !IsImageMediaType(a.MimeType) {
		problems = append(problems, fmt.Errorf("image MIME %q is invalid", a.MimeType))
	}
	if strings.TrimSpace(a.Name) == "" {
		problems = append(problems, errors.New("name is empty"))
	}
	if a.Size < 0 {
		problems = append(problems, errors.New("size is negative"))
	} else if a.Size > MaxAttachmentBytes {
		problems = append(problems, fmt.Errorf("size exceeds %d bytes", MaxAttachmentBytes))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("attachment: %w", err)
	}
	return nil
}

// IsImageMediaType reports whether value names a well-formed image media type,
// the only media Runtime accepts as image content.
func IsImageMediaType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.HasPrefix(mediaType, "image/")
}
