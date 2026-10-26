package transcript

import (
	"errors"
	"testing"
	"time"
)

func TestParseID(t *testing.T) {
	id, err := ParseToolResultID("BLOB234")
	if err != nil || id != "BLOB234" {
		t.Fatalf("ParseID = (%q, %v)", id, err)
	}
	for _, raw := range []string{"", "A", "lowercase", "HAS-SPACE", "ABC018"} {
		if _, err := ParseToolResultID(raw); !errors.Is(err, errInvalidToolResultID) {
			t.Fatalf("ParseID(%q) error = %v, want ErrInvalidID", raw, err)
		}
	}
}

func TestToolResultBlobValidate(t *testing.T) {
	blob := ToolResultBlob{
		ID: "BLOB234", SessionID: "ses_1", Body: "body", CreatedAt: time.Now().UTC(),
	}
	if err := blob.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	blob.Body = ""
	if err := blob.Validate(); err == nil {
		t.Fatal("Validate accepted a blob without a body")
	}
}
