package runtimebinding

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/fileinput"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

const maximumAttachmentBytes = 20 << 20

type attachmentTooLargeError struct {
	maximumBytes int64
}

func (a attachmentTooLargeError) Error() string {
	return fmt.Sprintf("file exceeds %d bytes", a.maximumBytes)
}

type attachmentLoader func(context.Context, string, int64) ([]byte, error)

func loadAttachmentFile(ctx context.Context, path string, maximumBytes int64) (_ []byte, err error) {
	if causeErr := context.Cause(ctx); causeErr != nil {
		return nil, causeErr
	}
	source, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := validateAttachmentSource(source, maximumBytes); err != nil {
		return nil, err
	}
	file, opened, err := fileinput.OpenExpected(path, source, maximumBytes)
	if err != nil {
		switch {
		case errors.Is(err, fileinput.ErrChanged):
			return nil, errors.New("file changed while it was being opened")
		case errors.Is(err, fileinput.ErrNotRegular):
			return nil, errors.New("file is not a regular file")
		case errors.Is(err, fileinput.ErrTooLarge):
			return nil, attachmentTooLargeError{maximumBytes: maximumBytes}
		default:
			return nil, err
		}
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	if err := validateAttachmentSource(opened, maximumBytes); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(contextReader{Context: ctx, Reader: file}, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if int64(len(data)) > maximumBytes {
		return nil, attachmentTooLargeError{maximumBytes: maximumBytes}
	}
	if err := fileinput.VerifyPathVersion(file, opened, path); err != nil {
		if errors.Is(err, fileinput.ErrChanged) {
			return nil, errors.New("file changed while it was being read")
		}
		return nil, fmt.Errorf("verify file after reading: %w", err)
	}
	return data, nil
}

func validateAttachmentSource(info os.FileInfo, maximumBytes int64) error {
	if !info.Mode().IsRegular() {
		return errors.New("file is not a regular file")
	}
	if info.Size() > maximumBytes {
		return attachmentTooLargeError{maximumBytes: maximumBytes}
	}
	return nil
}

type contextReader struct {
	context.Context
	io.Reader
}

func (c contextReader) Read(buffer []byte) (int, error) {
	if err := context.Cause(c.Context); err != nil {
		return 0, err
	}
	return c.Reader.Read(buffer)
}

func (r *Connection) projectInput(ctx context.Context, message agent.Message) ([]protocol.ContentBlock, error) {
	if err := message.Validate(); err != nil {
		return nil, err
	}
	if err := r.requireInputCapabilities(message); err != nil {
		return nil, err
	}
	blocks := make([]protocol.ContentBlock, 0, 1+len(message.Attachments))
	if message.HasText() {
		blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentBlockText, Text: message.Text})
	}
	for _, attachment := range message.Attachments {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		data, err := r.loadAttachment(ctx, attachment.Path, maximumAttachmentBytes)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", attachment.Name, err)
		}
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		switch attachment.Kind {
		case protocol.ContentBlockText:
			if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
				return nil, fmt.Errorf("read attachment %q: content is not valid text", attachment.Name)
			}
			blocks = append(blocks, protocol.ContentBlock{
				Type: protocol.ContentBlockText,
				Text: fmt.Sprintf("--- attached file: %q ---\n%s\n--- end attached file ---", attachment.Name, data),
			})
		case protocol.ContentBlockImage:
			blocks = append(blocks, protocol.ContentBlock{
				Type: protocol.ContentBlockImage, Mime: attachment.MimeType,
				Data: base64.StdEncoding.EncodeToString(data),
			})
		default:
			return nil, fmt.Errorf("attachment %q has unsupported kind %q", attachment.Name, attachment.Kind)
		}
	}
	return blocks, nil
}

func (r *Connection) requireInputCapabilities(message agent.Message) error {
	for _, attachment := range message.Attachments {
		if attachment.Kind == protocol.ContentBlockImage {
			return r.requireFeature(protocol.FeatureMultimodal)
		}
	}
	return nil
}

func projectContent(itemID string, content []protocol.ContentBlock) (string, []agent.Attachment, error) {
	projected, err := projectContentValue(itemID, content)
	return projected.text, projected.attachments, err
}

func projectAssistantContent(itemID string, content []protocol.ContentBlock) (string, []agent.InlineImage, error) {
	projected, err := projectContentValue(itemID, content)
	return projected.text, projected.images, err
}

type contentProjection struct {
	text        string
	attachments []agent.Attachment
	images      []agent.InlineImage
}

func projectContentValue(itemID string, content []protocol.ContentBlock) (contentProjection, error) {
	textParts := make([]string, 0, len(content))
	attachments := make([]agent.Attachment, 0, len(content))
	images := make([]agent.InlineImage, 0, len(content))
	for index, block := range content {
		switch block.Type {
		case protocol.ContentBlockText:
			textParts = append(textParts, block.Text)
		case protocol.ContentBlockImage:
			data, err := base64.StdEncoding.DecodeString(block.Data)
			if err != nil {
				return contentProjection{}, fmt.Errorf("item %s image %d: decode base64: %w", itemID, index+1, err)
			}
			name := "image"
			if extensions, _ := mime.ExtensionsByType(block.Mime); len(extensions) != 0 {
				name += extensions[0]
			} else if subtype := strings.TrimPrefix(block.Mime, "image/"); subtype != block.Mime && subtype != "" {
				name += "." + filepath.Base(subtype)
			}
			attachments = append(attachments, agent.Attachment{
				ID: fmt.Sprintf("%s:image:%d", itemID, index), Kind: protocol.ContentBlockImage,
				Name: name, MimeType: block.Mime, Size: int64(len(data)),
			})
			images = append(images, agent.InlineImage{
				ID: fmt.Sprintf("%s:image:%d", itemID, index), Name: name, MIMEType: block.Mime, Data: data,
			})
		default:
			return contentProjection{}, fmt.Errorf("item %s content %d has unsupported type %q", itemID, index+1, block.Type)
		}
	}
	return contentProjection{
		text: strings.Join(textParts, "\n\n"), attachments: attachments, images: images,
	}, nil
}
