package httpresponse

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
)

var errTestFrameTooLarge = errors.New("test frame too large")

func TestLimitBodyAcceptsExactDocument(t *testing.T) {
	response := responseWithBody("application/json", strings.Repeat("x", 8))
	LimitBody(response, 8, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != strings.Repeat("x", 8) {
		t.Fatalf("accepted content = %q, want eight bytes", content)
	}
}

func TestLimitBodyRejectsOversizedDocumentWithoutDraining(t *testing.T) {
	response := responseWithBody("application/json", strings.Repeat("x", 9))
	LimitBody(response, 8, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if !errors.Is(err, errTestFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want size error", err)
	}
	if string(content) != strings.Repeat("x", 8) {
		t.Fatalf("accepted content = %q, want eight bytes", content)
	}
	if read, err := response.Body.Read(make([]byte, 1)); read != 0 || !errors.Is(err, errTestFrameTooLarge) {
		t.Fatalf("read after rejection = (%d, %v), want stable size error", read, err)
	}
}

func TestLimitBodyResetsAtSSEEventBoundaries(t *testing.T) {
	response := responseWithBody("text/event-stream; charset=utf-8", "data: a\r\n\r\ndata: b\r\n\r\n")
	LimitBody(response, 11, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "data: a\r\n\r\ndata: b\r\n\r\n" {
		t.Fatalf("SSE content = %q", content)
	}
}

func TestLimitBodyRejectsOversizedSSEEvent(t *testing.T) {
	response := responseWithBody("text/event-stream", "data: a\ndata: b\n\n")
	LimitBody(response, 10, errTestFrameTooLarge)
	content, err := io.ReadAll(response.Body)
	if !errors.Is(err, errTestFrameTooLarge) {
		t.Fatalf("ReadAll error = %v, want size error", err)
	}
	if string(content) != "data: a\nda" {
		t.Fatalf("accepted SSE prefix = %q, want bounded prefix", content)
	}
}

func responseWithBody(contentType, content string) *http.Response {
	return &http.Response{
		Header: http.Header{"Content-Type": {contentType}},
		Body:   io.NopCloser(strings.NewReader(content)),
	}
}

func TestLimitBodyAdmitsEverySSELineEndingAcrossReads(t *testing.T) {
	for _, ending := range []string{"\n", "\r", "\r\n"} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(ending, "\r", "CR"), "\n", "LF"), func(t *testing.T) {
			frame := "data: a" + ending + ending
			stream := strings.Repeat(frame, 3)
			for _, chunks := range []func(io.Reader) io.Reader{func(r io.Reader) io.Reader { return r }, iotest.OneByteReader, iotest.HalfReader} {
				response := responseWithBody("text/event-stream", "")
				response.Body = io.NopCloser(chunks(strings.NewReader(stream)))
				LimitBody(response, int64(len(frame)), errTestFrameTooLarge)
				got, err := io.ReadAll(response.Body)
				if err != nil || string(got) != stream {
					t.Fatalf("exact SSE stream = (%q, %v)", got, err)
				}
			}
			response := responseWithBody("text/event-stream", frame)
			LimitBody(response, int64(len(frame)-1), errTestFrameTooLarge)
			if _, err := io.ReadAll(response.Body); !errors.Is(err, errTestFrameTooLarge) {
				t.Fatalf("oversized SSE frame error = %v", err)
			}
		})
	}
}
