package terminal

import (
	"errors"
	"strings"
)

func resolveByID[T any](items []T, identity, noun string, key func(T) string) (T, error) {
	return resolveUnique(items, identity, key,
		noun+" not found: "+identity,
		noun+" identity is ambiguous; use the full id")
}

func resolveByName[T any](items []T, identity, noun string, key func(T) string) (T, error) {
	return resolveUnique(items, identity, key,
		noun+" not found: "+identity,
		noun+" name is ambiguous; use the full name")
}

func resolveUnique[T any](items []T, identity string, key func(T) string, missing, ambiguous string) (T, error) {
	var zero T
	for _, item := range items {
		if key(item) == identity {
			return item, nil
		}
	}
	var matches []T
	for _, item := range items {
		if strings.HasPrefix(key(item), identity) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return zero, errors.New(missing)
	case 1:
		return matches[0], nil
	default:
		return zero, errors.New(ambiguous)
	}
}
