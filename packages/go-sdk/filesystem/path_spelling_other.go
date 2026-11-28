//go:build !darwin

package filesystem

func directorySpelling(path string) (string, error) { return path, nil }
