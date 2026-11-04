package instant

import (
	"testing"
	"time"

	"github.com/Tangerg/frame/portable"
	"github.com/Tangerg/frame/value"
)

func TestPortableTimestampKeepsNativeNanosecondCodec(t *testing.T) {
	if _, err := portable.SchemaFor[Timestamp](); err != nil {
		t.Fatal(err)
	}
	original := Timestamp{Time: time.Date(2026, 11, 4, 6, 7, 8, 123456789, time.UTC)}
	encoded, err := value.EncodeJSON(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Timestamp
	if err := value.DecodeValue(encoded.Bytes(), &decoded, "timestamp"); err != nil || !decoded.Time.Equal(original.Time) {
		t.Fatalf("timestamp precision changed: %s, %v", encoded.String(), err)
	}
	for _, raw := range []string{`null`, `{}`, `"not an instant"`, `"2026-02-30T00:00:00Z"`} {
		candidate := original
		if err := value.DecodeValue([]byte(raw), &candidate, "timestamp"); err == nil {
			t.Fatalf("invalid native timestamp admitted: %s", raw)
		}
	}
}
