//go:build !darwin

package sandbox

import "fmt"

func checkBackend() error {
	return fmt.Errorf("%w: supported backend requires macOS Seatbelt", ErrUnavailable)
}

func platformRunner(Config) (commandRunner, error) {
	return nil, checkBackend()
}

func seatbeltCommand(string, string, []string, []string) Command {
	return Command{}
}
