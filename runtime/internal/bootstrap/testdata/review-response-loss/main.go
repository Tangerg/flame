package main

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	if len(os.Args) != 3 {
		return fmt.Errorf("response loss probe requires an evidence path and backend executable")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[2])
	command.Stderr = os.Stderr
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	defer func() {
		cancel()
		_ = input.Close()
		err = errors.Join(err, command.Wait())
	}()
	var updates sync.Map
	forwarded := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			var request struct {
				ID     jsontext.Value `json:"id"`
				Method string         `json:"method"`
				Params struct {
					Name string `json:"name"`
				} `json:"params"`
			}
			if err := json.Unmarshal(line, &request); err != nil {
				forwarded <- err
				cancel()
				return
			}
			if request.Method == "tools/call" && request.Params.Name == "update_review" {
				updates.Store(string(request.ID), jsontext.Value(line))
			}
			if _, err := fmt.Fprintln(input, string(line)); err != nil {
				forwarded <- err
				cancel()
				return
			}
		}
		forwarded <- errors.Join(scanner.Err(), input.Close())
	}()
	responses := bufio.NewScanner(output)
	for responses.Scan() {
		line := responses.Bytes()
		var response struct {
			ID jsontext.Value `json:"id"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			return err
		}
		if request, ok := updates.LoadAndDelete(string(response.ID)); ok {
			evidence, err := json.Marshal(struct {
				DataDirectory string         `json:"dataDirectory"`
				Request       jsontext.Value `json:"request"`
				Response      jsontext.Value `json:"response"`
			}{os.Getenv("PLUGIN_DATA"), request.(jsontext.Value), line})
			if err != nil {
				return err
			}
			file, err := os.OpenFile(os.Args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err == nil {
				_, writeErr := file.Write(evidence)
				if err := errors.Join(writeErr, file.Close()); err != nil {
					return err
				}
				// The real backend has returned its committed result. Retire the
				// transport before Runtime can observe any of those response bytes.
				return fmt.Errorf("response loss probe withheld the committed update")
			}
			if !errors.Is(err, os.ErrExist) {
				return err
			}
		}
		if _, err := fmt.Fprintln(os.Stdout, string(line)); err != nil {
			return err
		}
	}
	if err := responses.Err(); err != nil {
		return err
	}
	select {
	case err := <-forwarded:
		return err
	default:
		return nil
	}
}
