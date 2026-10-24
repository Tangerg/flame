// Command nodequalification exercises a built distribution without a source
// checkout, compiler, system Node or provider request.
package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"
	reports "github.com/Tangerg/flame/plugins/usagereports"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/protocol"
)

func main() {
	distribution := flag.String("distribution", "", "absolute built Runtime/CLI distribution")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := qualify(ctx, *distribution); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("managed Node distribution qualified on %s/%s: host callback, CLI, restart, receipts, removal and retirement\n", runtime.GOOS, runtime.GOARCH)
}

func qualify(ctx context.Context, directory string) (err error) {
	if !filepath.IsAbs(directory) {
		return errors.New("qualification: distribution must be absolute")
	}
	node, err := localruntime.NodeForExecutable(filepath.Join(directory, "flame-runtime"))
	if err != nil {
		return err
	}
	if err := node.Verify(ctx); err != nil {
		return err
	}
	state, err := os.MkdirTemp("", "flame-node-qualification-")
	if err != nil {
		return err
	}
	defer func() {
		cleanupErr := filepath.WalkDir(state, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return nil
		})
		err = errors.Join(err, cleanupErr, os.RemoveAll(state))
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return err
	}
	env := []string{"PATH=", "FLAME_HOME=" + filepath.Join(state, "home"), "FLAME_PROVIDER=anthropic", "FLAME_APIKEY=qualification-key", "FLAME_SERVER_LISTEN=" + address, "FLAME_SERVER_NOLOCALTOKEN=true"}
	if value, found := os.LookupEnv("HOME"); found {
		env = append(env, "HOME="+value)
	}
	config := filepath.Join(state, "config.yaml")
	if err := os.WriteFile(config, []byte("{}\n"), 0600); err != nil {
		return err
	}
	var process *exec.Cmd
	var exited chan error
	var client *runtimeclient.Client
	stop := func() error {
		if process == nil {
			return nil
		}
		var closeErr error
		if client != nil {
			closeErr = client.Close()
			client = nil
		}
		signalErr := process.Process.Signal(os.Interrupt)
		if errors.Is(signalErr, os.ErrProcessDone) {
			signalErr = nil
		}
		select {
		case waitErr := <-exited:
			process = nil
			return errors.Join(closeErr, signalErr, waitErr)
		case <-ctx.Done():
			_ = process.Process.Kill()
			<-exited
			process = nil
			return errors.Join(closeErr, ctx.Err())
		}
	}
	defer func() { err = errors.Join(err, stop()) }()
	start := func() error {
		process = exec.CommandContext(ctx, filepath.Join(directory, "flame-runtime"))
		process.Dir, process.Env, process.Stderr = state, env, os.Stderr
		if err := process.Start(); err != nil {
			process = nil
			return err
		}
		exited = make(chan error, 1)
		running := process
		go func() { exited <- running.Wait() }()
		client, err = runtimeclient.Connect(ctx, runtimeclient.RemoteConfig{Endpoint: "http://" + address})
		if err != nil {
			return err
		}
		for {
			if _, discoverErr := client.Discover(ctx, protocol.CallOptions{}); discoverErr == nil {
				return nil
			} else if _, connecting := errors.AsType[*net.OpError](discoverErr); !connecting {
				return discoverErr
			}
			select {
			case exitErr := <-exited:
				process = nil
				return fmt.Errorf("qualification: Runtime exited before readiness: %w", exitErr)
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	if err := start(); err != nil {
		return err
	}
	installed, err := client.ListPlugins(ctx, protocol.CallOptions{})
	if err != nil {
		return err
	}
	var usage protocol.PluginInstallation
	for _, item := range installed.Data {
		if item.Selected.Name == "flame.usagereports" {
			usage = item
		}
	}
	if usage.ID == "" || len(usage.Selected.Components) != 1 {
		return errors.New("qualification: complete managed usage package is absent")
	}
	wait := func() error {
		for {
			servers, err := client.ListMCPServers(ctx, protocol.CallOptions{})
			if err != nil {
				return err
			}
			for _, server := range servers.Data {
				if server.ID.Origin.InstallationID != usage.ID {
					continue
				}
				if server.Status.Type == protocol.MCPServerConnected {
					return nil
				}
				if server.Status.Type == protocol.MCPServerFailed {
					return errors.New("qualification: managed backend activation failed")
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	if err := wait(); err != nil {
		return err
	}
	contracts, err := reports.NewContracts[protocol.UsageSummary]()
	if err != nil {
		return err
	}
	request := protocol.CallPluginRequest{InstallationID: usage.ID, Digest: usage.Selected.Digest, Contribution: "usage-reports", Contract: contracts.Capability.Identity().String(), Method: contracts.Capture.Name(), Input: `{"period":"daily"}`, Target: capability.Target{Scope: "installation", Reference: usage.ID}}
	call := func(request protocol.CallPluginRequest, key string) (*protocol.CallPluginResponse, error) {
		response, err := client.CallPlugin(ctx, request, protocol.PluginCallOptions{IdempotencyKey: key})
		if err != nil {
			return nil, err
		}
		if response.Failure != nil {
			return nil, fmt.Errorf("qualification: business refusal %s", response.Failure.Code)
		}
		return response, nil
	}
	original, err := call(request, "original-save")
	if err != nil {
		return err
	}
	var header reports.Header
	if err := json.Unmarshal([]byte(original.Output), &header); err != nil {
		return err
	}
	cliInput, err := json.Marshal(request)
	if err != nil {
		return err
	}
	cli := exec.CommandContext(ctx, filepath.Join(directory, "flame"), "--config", config, "--runtime-url", "http://"+address, "plugins", "call", "--request", string(cliInput), "--command-id", "cli_0123456789abcdef0123456789abcdef")
	cli.Dir, cli.Env, cli.Stderr = state, env, os.Stderr
	cliOutput, err := cli.Output()
	if err != nil {
		return fmt.Errorf("qualification: CLI call: %w", err)
	}
	var cliReply protocol.CallPluginResponse
	if err := json.Unmarshal(cliOutput, &cliReply); err != nil || cliReply.Failure != nil || cliReply.Output == "" {
		return errors.Join(errors.New("qualification: CLI did not capture the installed capability"), err)
	}
	read := request
	read.Method = contracts.Get.Name()
	encoded, err := json.Marshal(reports.ReportInput{ID: header.ID})
	if err != nil {
		return err
	}
	read.Input = string(encoded)
	saved, err := call(read, "")
	if err != nil {
		return err
	}
	if err := stop(); err != nil {
		return err
	}
	if err := start(); err != nil {
		return err
	}
	if err := wait(); err != nil {
		return err
	}
	restored, err := call(read, "")
	if err != nil {
		return err
	}
	if saved.Output != restored.Output {
		return errors.New("qualification: archived accounting changed across restart")
	}
	replay, err := call(request, "original-save")
	if err != nil {
		return err
	}
	if replay.Output != original.Output {
		return errors.New("qualification: original capture replay changed")
	}
	remove := read
	remove.Method = contracts.Delete.Name()
	if _, err := call(remove, "original-remove"); err != nil {
		return err
	}
	if _, err := call(request, "original-save"); err != nil {
		return err
	}
	if _, err := client.SetPluginEnablement(ctx, protocol.SetPluginEnablementRequest{InstallationID: usage.ID, Enabled: false}, protocol.CommandOptions{}); err != nil {
		return err
	}
	return stop()
}
