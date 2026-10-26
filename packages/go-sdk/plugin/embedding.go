package plugin

import (
	"context"
	"errors"
	"math"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

type TextEmbeddings struct {
	Space   string      `json:"space"`
	Vectors [][]float32 `json:"vectors"`
}

// TextEmbeddingMethod projects vectors from the captured embedding model. The
// model adapter retains the complete provider response and endpoint evidence.
func TextEmbeddingMethod() (*Method[[]string, TextEmbeddings], error) {
	return NewMethod[[]string, TextEmbeddings](MethodConfig{Name: "flame.model/embed-texts.v2", Kind: extension.Mutation, Target: "call", Description: "Embed texts and identify the vector space captured by this exchange."})
}

func CallTextEmbedding(ctx context.Context, ref CallbackRef, id string, texts []string) (TextEmbeddings, error) {
	if len(texts) == 0 {
		return TextEmbeddings{}, errors.New("embedding call: texts are required")
	}
	method, err := TextEmbeddingMethod()
	if err != nil {
		return TextEmbeddings{}, err
	}
	vectors, err := CallHost(ctx, ref, method, id, slices.Clone(texts))
	if err != nil {
		return TextEmbeddings{}, err
	}
	if err := validateTextEmbeddings(texts, vectors); err != nil {
		return TextEmbeddings{}, err
	}
	return vectors, nil
}

// NewTextEmbeddingCallback leaves admission and budgets with the calling owner.
// Physical model evidence belongs to this child exchange, retaining its parent.
func NewTextEmbeddingCallback(call func(context.Context, Command[[]string]) (TextEmbeddings, error)) (Callback, error) {
	if call == nil {
		return Callback{}, errors.New("embedding call: model owner is required")
	}
	method, err := TextEmbeddingMethod()
	if err != nil {
		return Callback{}, err
	}
	return NewCallback(method, nil, func(ctx context.Context, command Command[[]string]) (TextEmbeddings, error) {
		if len(command.Input) == 0 {
			return TextEmbeddings{}, errors.New("embedding call: texts are required")
		}
		if parent, err := InvocationFromContext(ctx); err == nil && parent.ID == command.ID {
			return TextEmbeddings{}, errors.New("embedding call: independent invocation requires a distinct identity")
		}
		ctx, err := modelCallContext(ctx, command.ID)
		if err != nil {
			return TextEmbeddings{}, err
		}
		vectors, err := call(ctx, command)
		if err != nil {
			return TextEmbeddings{}, err
		}
		if err := validateTextEmbeddings(command.Input, vectors); err != nil {
			return TextEmbeddings{}, err
		}
		return vectors, nil
	})
}

func validateTextEmbeddings(texts []string, result TextEmbeddings) error {
	if result.Space == "" {
		return errors.New("embedding call: vector space is required")
	}
	vectors := result.Vectors
	if len(vectors) != len(texts) || len(vectors) == 0 || len(vectors[0]) == 0 {
		return errors.New("embedding call: vectors differ from the admitted texts")
	}
	for _, vector := range vectors {
		if len(vector) != len(vectors[0]) {
			return errors.New("embedding call: vector dimensions differ")
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return errors.New("embedding call: vector values must be finite")
			}
		}
	}
	return nil
}
