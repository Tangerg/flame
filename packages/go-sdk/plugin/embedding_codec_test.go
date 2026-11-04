package plugin

import (
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/frame/portable"
	"github.com/Tangerg/frame/value"
)

func TestEmbeddingWireProjectionKeepsFiniteBinary32Values(t *testing.T) {
	if _, err := portable.SchemaFor[TextEmbeddings](); err != nil {
		t.Fatal(err)
	}
	original := TextEmbeddings{Space: "original", Vectors: [][]float32{{math.SmallestNonzeroFloat32, math.MaxFloat32, -0.25}}}
	encoded, err := value.EncodeJSON(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TextEmbeddings
	if err := value.DecodeValue(encoded.Bytes(), &decoded, "embeddings"); err != nil || !reflect.DeepEqual(decoded, original) {
		t.Fatalf("binary32 coordinates changed: %+v, %v", decoded, err)
	}
	for _, raw := range []string{
		`{"space":"x","vectors":null}`,
		`{"space":"x","vectors":[null]}`,
		`{"space":"x","vectors":[[null]]}`,
		`{"space":"x","vectors":[[1e100]]}`,
		`{"space":"x","vectors":[[1]],"unknown":true}`,
		`{"vectors":[[1]]}`,
	} {
		candidate := original
		if err := value.DecodeValue([]byte(raw), &candidate, "embeddings"); err == nil {
			t.Fatalf("invalid model projection admitted: %s", raw)
		}
		if !reflect.DeepEqual(candidate, original) {
			t.Fatalf("failed decode changed the original result: %+v", candidate)
		}
	}
	for _, coordinate := range []float32{float32(math.Inf(1)), float32(math.NaN())} {
		if _, err := value.EncodeJSON(TextEmbeddings{Space: "invalid", Vectors: [][]float32{{coordinate}}}); err == nil {
			t.Fatal("non-finite model value reached JSON")
		}
	}
}
