package plugin

import (
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/capability"
)

// TextEmbeddingMethod projects vectors from the captured embedding model. The
// model adapter retains the complete provider response and endpoint evidence.
func TextEmbeddingMethod() (*Method[[]string, [][]float32], error) {
	return NewMethod[[]string, [][]float32](MethodConfig{Name: "flame.model/embed-texts.v1", Kind: capability.Mutation, Target: "call", Description: "Embed texts with the model captured by this call."})
}

func CallTextEmbedding(ctx context.Context, ref CallbackRef, id string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errors.New("embedding call: texts are required")
	}
	method, err := TextEmbeddingMethod()
	if err != nil {
		return nil, err
	}
	vectors, err := CallHost(ctx, ref, method, id, slices.Clone(texts))
	if err != nil {
		return nil, err
	}
	if err := validateTextEmbeddings(texts, vectors); err != nil {
		return nil, err
	}
	return vectors, nil
}

// NewTextEmbeddingCallback leaves admission and budgets with the calling owner.
// Physical model evidence belongs to this child exchange, retaining its parent.
func NewTextEmbeddingCallback(call func(context.Context, Command[[]string]) ([][]float32, error)) (Callback, error) {
	if call == nil {
		return Callback{}, errors.New("embedding call: model owner is required")
	}
	method, err := TextEmbeddingMethod()
	if err != nil {
		return Callback{}, err
	}
	return NewCallback(method, nil, func(ctx context.Context, command Command[[]string]) ([][]float32, error) {
		if len(command.Input) == 0 {
			return nil, errors.New("embedding call: texts are required")
		}
		ctx, err := modelCallContext(ctx, command.ID)
		if err != nil {
			return nil, err
		}
		vectors, err := call(ctx, command)
		if err != nil {
			return nil, err
		}
		if err := validateTextEmbeddings(command.Input, vectors); err != nil {
			return nil, err
		}
		return vectors, nil
	})
}

func validateTextEmbeddings(texts []string, vectors [][]float32) error {
	if len(vectors) != len(texts) || len(vectors) == 0 || len(vectors[0]) == 0 {
		return errors.New("embedding call: vectors differ from the admitted texts")
	}
	for _, vector := range vectors {
		if len(vector) != len(vectors[0]) {
			return errors.New("embedding call: vector dimensions differ")
		}
	}
	return nil
}
