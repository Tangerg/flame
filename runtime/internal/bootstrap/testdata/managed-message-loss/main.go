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
	"strings"
	"sync"
	"syscall"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	if len(os.Args) != 6 || (os.Args[5] != "request" && os.Args[5] != "response" && os.Args[5] != "durable") {
		return fmt.Errorf("message loss probe requires evidence, backend, method, tool and request or response stage")
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
		// A loss probe must not reject frames that its MCP peers admit.
		scanner.Buffer(nil, sdk.DefaultMaxLineLength)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			var request struct {
				ID     jsontext.Value `json:"id"`
				Method string         `json:"method"`
				Params struct {
					Name   string `json:"name"`
					Method string `json:"method"`
				} `json:"params"`
			}
			if err := json.Unmarshal(line, &request); err != nil {
				forwarded <- err
				cancel()
				return
			}
			semantic := request.Method
			if semantic == "io.github.tangerg.flame/operations.invoke" {
				semantic = request.Params.Method
			}
			matched := semantic == os.Args[3] && (os.Args[4] == "" || request.Params.Name == os.Args[4])
			if os.Args[5] == "durable" {
				phase, err := os.ReadFile(os.Args[1] + ".phase")
				if err != nil {
					forwarded <- err
					cancel()
					return
				}
				stage := strings.TrimSpace(string(phase))
				matched = (stage == "commit" || stage == "edited") && request.Method == "tools/call" && request.Params.Name == "create_schedule" || stage == "query" && semantic == pluginsdk.ToolReceiptMethod
			}
			if matched {
				if os.Args[5] == "request" {
					withheld, err := withhold(line, nil)
					if err != nil || withheld {
						forwarded <- errors.Join(err, errors.New("message loss probe withheld request"))
						cancel()
						return
					}
				} else {
					updates.Store(string(request.ID), jsontext.Value(line))
				}
			}
			if os.Args[5] == "durable" && semantic == pluginsdk.ToolReceiptMethod {
				updates.Store(string(request.ID), jsontext.Value(line))
			}
			if _, err := fmt.Fprintln(input, string(line)); err != nil {
				forwarded <- err
				cancel()
				return
			}
		}
		forwarded <- errors.Join(scanner.Err(), input.Close())
		if os.Args[5] == "durable" {
			cancel()
		}
	}()
	responses := bufio.NewScanner(output)
	responses.Buffer(nil, sdk.DefaultMaxLineLength)
	for responses.Scan() {
		line := responses.Bytes()
		var response struct {
			ID jsontext.Value `json:"id"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			return err
		}
		if request, ok := updates.LoadAndDelete(string(response.ID)); ok {
			if os.Args[5] == "durable" {
				phase, err := os.ReadFile(os.Args[1] + ".phase")
				if err != nil {
					return err
				}
				record, err := json.Marshal(struct {
					Request       jsontext.Value `json:"request"`
					Response      jsontext.Value `json:"response"`
					DataDirectory string         `json:"dataDirectory"`
				}{request.(jsontext.Value), jsontext.Value(line), os.Getenv("PLUGIN_DATA")})
				if err != nil {
					return err
				}
				if err := os.WriteFile(os.Args[1]+"."+strings.TrimSpace(string(phase)), record, 0600); err != nil {
					return err
				}
				if strings.TrimSpace(string(phase)) != "commit" && strings.TrimSpace(string(phase)) != "edited" && strings.TrimSpace(string(phase)) != "query" {
					if _, err := fmt.Fprintln(os.Stdout, string(line)); err != nil {
						return err
					}
					continue
				}
				ready, err := os.OpenFile(os.Args[1]+".ready", os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				_, writeErr := ready.WriteString("ready\n")
				if err := errors.Join(writeErr, ready.Close()); err != nil {
					return err
				}
				<-ctx.Done()
				return context.Cause(ctx)
			}
			withheld, err := withhold(request.(jsontext.Value), line)
			if err != nil {
				return err
			}
			if withheld {
				return errors.New("message loss probe withheld committed response")
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

func withhold(request, response jsontext.Value) (bool, error) {
	evidence, err := json.Marshal(struct {
		DataDirectory string         `json:"dataDirectory"`
		Request       jsontext.Value `json:"request"`
		Response      jsontext.Value `json:"response,omitempty"`
	}{os.Getenv("PLUGIN_DATA"), request, response})
	if err != nil {
		return false, err
	}
	file, err := os.OpenFile(os.Args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, writeErr := file.Write(evidence)
	return true, errors.Join(writeErr, file.Close())
}
