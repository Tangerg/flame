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

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
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

const readinessPollInterval = 20 * time.Millisecond

type distribution struct {
	binaryDir string
	stateDir  string
	endpoint  string
	env       []string
	process   *exec.Cmd
	exited    <-chan error
	client    *runtimeclient.Client
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
	defer func() { err = errors.Join(err, removeState(state)) }()
	dist, err := newDistribution(directory, state)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, dist.stop(ctx)) }()
	if err := dist.start(ctx); err != nil {
		return err
	}
	return dist.checkReports(ctx)
}

func removeState(state string) error {
	err := filepath.WalkDir(state, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0700)
		}
		return nil
	})
	return errors.Join(err, os.RemoveAll(state))
}

func newDistribution(directory, state string) (*distribution, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return nil, err
	}
	env := []string{"PATH=", "FLAME_HOME=" + filepath.Join(state, "home"), "FLAME_PROVIDER=anthropic", "FLAME_MODEL=claude-opus-5", "FLAME_APIKEY=qualification-key", "FLAME_SERVER_LISTEN=" + address, "FLAME_SERVER_NOLOCALTOKEN=true"}
	if home, ok := os.LookupEnv("HOME"); ok {
		env = append(env, "HOME="+home)
	}
	if err := os.WriteFile(filepath.Join(state, "config.yaml"), []byte("{}\n"), 0600); err != nil {
		return nil, err
	}
	return &distribution{binaryDir: directory, stateDir: state, endpoint: "http://" + address, env: env}, nil
}

func (d *distribution) start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, filepath.Join(d.binaryDir, "flame-runtime"))
	cmd.Dir, cmd.Env, cmd.Stderr = d.stateDir, d.env, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	d.process, d.exited = cmd, exited
	go func() { exited <- cmd.Wait() }()
	client, err := runtimeclient.Connect(ctx, runtimeclient.RemoteConfig{Endpoint: d.endpoint})
	if err != nil {
		return err
	}
	d.client = client
	for {
		_, err := client.Discover(ctx, protocol.CallOptions{})
		if err == nil {
			return nil
		}
		if _, connecting := errors.AsType[*net.OpError](err); !connecting {
			return err
		}
		select {
		case err := <-exited:
			d.process = nil
			if err == nil {
				return errors.New("qualification: Runtime exited before readiness")
			}
			return fmt.Errorf("qualification: Runtime exited before readiness: %w", err)
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readinessPollInterval):
		}
	}
}

func (d *distribution) stop(ctx context.Context) error {
	var err error
	if d.client != nil {
		err = d.client.Close()
		d.client = nil
	}
	if d.process == nil {
		return err
	}
	signalErr := d.process.Process.Signal(os.Interrupt)
	if errors.Is(signalErr, os.ErrProcessDone) {
		signalErr = nil
	}
	select {
	case waitErr := <-d.exited:
		d.process = nil
		return errors.Join(err, signalErr, waitErr)
	case <-ctx.Done():
		killErr := d.process.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		waitErr := <-d.exited
		d.process = nil
		return errors.Join(err, signalErr, ctx.Err(), killErr, waitErr)
	}
}

func (d *distribution) waitBackend(ctx context.Context, id string) error {
	for {
		servers, err := d.client.ListMCPServers(ctx, protocol.CallOptions{})
		if err != nil {
			return err
		}
		for _, server := range servers.Data {
			if server.ID.Origin.InstallationID != id {
				continue
			}
			switch server.Status.Type {
			case protocol.MCPServerConnected:
				return nil
			case protocol.MCPServerFailed:
				return errors.New("qualification: managed backend activation failed")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readinessPollInterval):
		}
	}
}

func (d *distribution) captureRequest(ctx context.Context) (protocol.CallPluginRequest, error) {
	installed, err := d.client.ListPlugins(ctx, protocol.CallOptions{})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	for _, usage := range installed.Data {
		if usage.Selected.Name != "flame.usagereports" {
			continue
		}
		if len(usage.Selected.Components) != 1 {
			break
		}
		contract, err := plugincontract.ParseCapability([]byte(UsageReportsCapabilityJSON))
		if err != nil {
			return protocol.CallPluginRequest{}, err
		}
		input, err := json.Marshal(UsageReportsCaptureInput{Period: UsageReportsCaptureInputPeriodDaily})
		if err != nil {
			return protocol.CallPluginRequest{}, err
		}
		return protocol.CallPluginRequest{InstallationID: usage.ID, Digest: usage.Selected.Digest, Contribution: "usage-reports", Contract: contract.Identity().String(), Method: UsageReportsCaptureV1Method, Input: string(input), Target: portable.Target{Scope: "installation", Reference: usage.ID}}, nil
	}
	return protocol.CallPluginRequest{}, errors.New("qualification: complete managed usage package is absent")
}

func (d *distribution) call(ctx context.Context, request protocol.CallPluginRequest, key string) (*protocol.CallPluginResponse, error) {
	response, err := d.client.CallPlugin(ctx, request, protocol.PluginCallOptions{IdempotencyKey: key})
	if err != nil {
		return nil, err
	}
	if response.Failure != nil {
		return nil, fmt.Errorf("qualification: business refusal %s", response.Failure.Code)
	}
	return response, nil
}

func (d *distribution) captureWithCLI(ctx context.Context, request protocol.CallPluginRequest) error {
	input, err := json.Marshal(request)
	if err != nil {
		return err
	}
	cli := exec.CommandContext(ctx, filepath.Join(d.binaryDir, "flame"), "--config", filepath.Join(d.stateDir, "config.yaml"), "--runtime-url", d.endpoint, "plugins", "call", "--request", string(input), "--command-id", "cli_0123456789abcdef0123456789abcdef")
	cli.Dir, cli.Env, cli.Stderr = d.stateDir, d.env, os.Stderr
	output, err := cli.Output()
	if err != nil {
		return fmt.Errorf("qualification: CLI call: %w", err)
	}
	var reply protocol.CallPluginResponse
	if err := json.Unmarshal(output, &reply); err != nil || reply.Failure != nil || reply.Output == "" {
		return errors.Join(errors.New("qualification: CLI did not capture the installed capability"), err)
	}
	return nil
}

func (d *distribution) checkReports(ctx context.Context) error {
	request, err := d.captureRequest(ctx)
	if err != nil {
		return err
	}
	if err := d.waitBackend(ctx, request.InstallationID); err != nil {
		return err
	}
	original, err := d.call(ctx, request, "original-save")
	if err != nil {
		return err
	}
	if err := d.captureWithCLI(ctx, request); err != nil {
		return err
	}
	var header UsageReportsCaptureV1OutputRpcHeader
	if err := json.Unmarshal([]byte(original.Output), &header); err != nil {
		return err
	}
	encoded, err := json.Marshal(UsageReportsGetV1InputRpcReportInput{Id: header.Id})
	if err != nil {
		return err
	}
	read := request
	read.Method, read.Input = UsageReportsGetV1Method, string(encoded)
	saved, err := d.call(ctx, read, "")
	if err != nil {
		return err
	}
	if err := d.stop(ctx); err != nil {
		return err
	}
	if err := d.start(ctx); err != nil {
		return err
	}
	if err := d.waitBackend(ctx, request.InstallationID); err != nil {
		return err
	}
	if err := d.verifyOriginal(ctx, read, "", saved.Output); err != nil {
		return err
	}
	if err := d.verifyOriginal(ctx, request, "original-save", original.Output); err != nil {
		return err
	}
	remove := read
	encoded, err = json.Marshal(UsageReportsDeleteV1InputRpcReportInput{Id: header.Id})
	if err != nil {
		return err
	}
	remove.Method, remove.Input = UsageReportsDeleteV1Method, string(encoded)
	if _, err := d.call(ctx, remove, "original-remove"); err != nil {
		return err
	}
	if err := d.verifyOriginal(ctx, request, "original-save", original.Output); err != nil {
		return err
	}
	_, err = d.client.SetPluginEnablement(ctx, protocol.SetPluginEnablementRequest{InstallationID: request.InstallationID, Enabled: false}, protocol.CommandOptions{})
	return err
}

func (d *distribution) verifyOriginal(ctx context.Context, request protocol.CallPluginRequest, key, output string) error {
	reply, err := d.call(ctx, request, key)
	if err != nil {
		return err
	}
	if reply.Output != output {
		return fmt.Errorf("qualification: %s result changed across restart", request.Method)
	}
	return nil
}
