package plugin

import (
	json "encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/Tangerg/frame/value"
)

// Vectors are binary32 model outputs. Their wire projection contains explicit
// JSON numbers; conversion to binary32 occurs only at this model boundary.
type embeddingProjection struct {
	Space   string           `json:"space"`
	Vectors [][]value.Number `json:"vectors"`
}

func (TextEmbeddings) JSONSchemaModel() any { return embeddingProjection{} }

func (v TextEmbeddings) MarshalJSON() ([]byte, error) {
	type native TextEmbeddings
	return json.Marshal(native(v))
}

func (v *TextEmbeddings) UnmarshalJSON(data []byte) error {
	var encoded embeddingProjection
	if err := value.DecodeValue(data, &encoded, "embedding result"); err != nil {
		return err
	}
	result := TextEmbeddings{Space: encoded.Space, Vectors: make([][]float32, len(encoded.Vectors))}
	for row, numbers := range encoded.Vectors {
		result.Vectors[row] = make([]float32, len(numbers))
		for column, number := range numbers {
			coordinate, err := strconv.ParseFloat(number.String(), 32)
			if err != nil {
				return fmt.Errorf("embedding result: coordinate %d:%d: %w", row, column, err)
			}
			result.Vectors[row][column] = float32(coordinate)
		}
	}
	*v = result
	return nil
}
