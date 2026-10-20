package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

func TestRejectedLaunchRetiresItsResourceExactlyOnce(t *testing.T) {
	for _, refusal := range []string{"invalid configuration", "canceled configuration", "canceled authorization", "shutdown", "stdio authorization"} {
		t.Run(refusal, func(t *testing.T) {
			var retired atomic.Int32
			retirementErr := errors.New("retirement failed")
			config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "must-not-start"}}
			c := &Connections{lifetime: t.Context(), client: NewClient()}
			ctx := t.Context()
			if refusal == "invalid configuration" {
				config.Command = ""
			}
			if strings.HasPrefix(refusal, "canceled") {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			if refusal == "shutdown" {
				if err := c.Shutdown(ctx); err != nil {
					t.Fatal(err)
				}
			}
			input, err := NewLaunch(config, nil, func() error { retired.Add(1); return retirementErr }, Admission{})
			if strings.HasSuffix(refusal, "authorization") {
				c.servers = []*server{{id: config.ID(), config: config}}
				if err != nil {
					t.Fatal(err)
				}
				err = authorizeLaunch(c, ctx, input)
			} else if err == nil {
				err = connectLaunch(c, ctx, input)
			}
			if !errors.Is(err, retirementErr) || retired.Load() != 1 {
				t.Fatalf("refusal lost or repeated resource retirement: %v, calls=%d", err, retired.Load())
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			if err := connectLaunch(c, t.Context(), input); err == nil {
				t.Fatal("a consumed launch initiated another attempt")
			}
			if retired.Load() != 1 {
				t.Fatal("a consumed launch retired its resource again")
			}
		})
	}
}

func TestSessionLedgerOwnsLaunchResourceThroughDetachAndShutdown(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "leased-server", Version: "v1"}, nil)
	transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
	t.Cleanup(transport.Close)
	var retired atomic.Int32
	config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}}
	input, err := NewLaunch(config, nil, func() error { retired.Add(1); return nil }, Admission{})
	if err != nil {
		t.Fatal(err)
	}
	c := &Connections{lifetime: t.Context(), client: NewClient()}
	t.Cleanup(func() { _ = c.Shutdown(context.WithoutCancel(t.Context())) })
	if err := connectLaunch(c, t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if retired.Load() != 0 {
		t.Fatal("handshake completion retired a live session's resource")
	}
	if err := c.Detach(config.ID()); err != nil {
		t.Fatal(err)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if retired.Load() != 1 {
		t.Fatalf("shutdown did not join resource retirement: %d", retired.Load())
	}
}

func TestLaunchAdmissionFailureWithdrawsSessionAndRetiresResource(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint("panic=", panics), func(t *testing.T) {
			remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "unadmitted-server"}, nil)
			transport := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return remote }, nil))
			t.Cleanup(transport.Close)
			var retired atomic.Int32
			refusal := errors.New("declaration mismatch")
			config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: transport.URL}}
			input, err := NewLaunch(config, nil, func() error { retired.Add(1); return nil }, Admission{Run: func(context.Context, *sdkmcp.ClientSession) error {
				if panics {
					panic("admission failed")
				}
				return refusal
			}})
			if err != nil {
				t.Fatal(err)
			}
			c := &Connections{lifetime: t.Context(), client: NewClient()}
			t.Cleanup(func() { _ = c.Shutdown(context.WithoutCancel(t.Context())) })
			err = connectLaunch(c, t.Context(), input)
			if err == nil || !panics && !errors.Is(err, refusal) || retired.Load() != 1 {
				t.Fatalf("failed admission did not retire unpublished resources: %v, retired=%d", err, retired.Load())
			}
			status := c.Statuses()
			if len(status) != 1 || status[0].State != mcpserver.ConnectionFailed || status[0].Failure != mcpserver.FailureConfiguration {
				t.Fatalf("failed admission was published: %+v", status)
			}
			if _, err := c.ServiceSession(t.Context(), config.Server); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("unadmitted session remains callable: %v", err)
			}
		})
	}
}

func TestStartupAdmissionFailureRetiresTheUnlaunchedResource(t *testing.T) {
	config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "must-not-start"}}
	var retired atomic.Int32
	c, err := Open(t.Context(), t.Context(), []ServerConfig{config}, nil, func(context.Context, mcpserver.ID) (*Launch, error) {
		other := config.Clone()
		other.Name = testsupport.ServerName("other")
		return NewLaunch(other, nil, func() error { retired.Add(1); return nil }, Admission{})
	}, nil, NewClient())
	if err != nil {
		t.Fatal(err)
	}
	awaitInitialConnections(c)
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if retired.Load() != 1 {
		t.Fatalf("startup admission retained its resource: %d", retired.Load())
	}
}

func TestRetirementPanicStillReleasesTheLaunchResource(t *testing.T) {
	retired := 0
	owned := &ownedSession{closeFn: func() error { return retireSession(nil, func() error { retired++; return nil }) }}
	if err := closeOwnedSession(owned); err == nil || retired != 1 {
		t.Fatalf("session panic stranded its launch resource: %v, calls=%d", err, retired)
	}
}

func TestRelocatedStdioCannotBypassConnectionInvariants(t *testing.T) {
	config := ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "server"}}
	for _, invalid := range []Stdio{
		{Command: "bad\x00command"},
		{Command: "server", Args: []string{"bad\x00argument"}},
		{Command: "server", Env: map[string]string{"LD_PRELOAD": "/tmp/inject.so"}},
		{Command: "server", Dir: "bad\x00directory"},
	} {
		retired := 0
		input, err := NewLaunch(config, &invalid, func() error { retired++; return nil }, Admission{})
		if input != nil {
			_ = input.Close()
		}
		if err == nil || input != nil || retired != 1 {
			t.Errorf("invalid relocated process admitted: err=%v, retired=%d", err, retired)
		}
	}
}

func TestLaunchFormattingRedactsExecutionCredentials(t *testing.T) {
	secret := "private-execution-credential"
	input, err := NewLaunch(ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "server", Env: map[string]string{"TOKEN": secret}}}, &Stdio{Command: "server", Env: map[string]string{"TOKEN": secret}}, func() error { return nil }, Admission{})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	for _, verb := range []string{"%v", "%+v", "%#v"} {
		if output := fmt.Sprintf(verb, input); strings.Contains(output, secret) {
			t.Fatalf("launch diagnostic exposed credentials: %s", output)
		}
	}
}

func TestCopyingALaunchDoesNotDuplicateItsClaim(t *testing.T) {
	retired := 0
	input, err := NewLaunch(ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "must-not-start"}}, nil, func() error { retired++; return nil }, Admission{})
	if err != nil {
		t.Fatal(err)
	}
	copied := *input
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	c := &Connections{lifetime: t.Context(), client: NewClient()}
	if err := connectLaunch(c, t.Context(), &copied); err == nil {
		t.Fatal("a copied launch originated a competing claim")
	}
	if err := copied.Close(); err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Fatalf("copied launch advanced retirement twice: %d", retired)
	}
}

func TestRetainedInstallationDescriptorCannotLaunchWithoutExecutionContent(t *testing.T) {
	digest := fingerprint.Strings("release")
	source, err := mcpserver.InstallationSource(testsupport.InstallationID(t), digest, digest, digest)
	if err != nil {
		t.Fatal(err)
	}
	config := ServerConfig{Server: mcpserver.Server{Source: source, Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "must-not-start"}}
	retired := 0
	if input, err := NewLaunch(config, nil, func() error { retired++; return nil }, Admission{}); err == nil || input != nil || retired != 1 {
		t.Fatalf("retained installation descriptor acquired an unverified launch: %v, retired=%d", err, retired)
	}
	if err := probe(t.Context(), config); err == nil {
		t.Fatal("probe launched a retained installation descriptor")
	}
}

func connectLaunch(c *Connections, ctx context.Context, input *Launch) error {
	return runLaunch(c, ctx, input, false)
}

func authorizeLaunch(c *Connections, ctx context.Context, input *Launch) error {
	return runLaunch(c, ctx, input, true)
}

func runLaunch(c *Connections, ctx context.Context, input *Launch, authorize bool) error {
	name := testsupport.UserMCPServer("consumed")
	if input != nil && input.claim != nil {
		input.claim.mu.Lock()
		if input.claim.pending != nil {
			name = input.claim.pending.config.ID()
		}
		input.claim.mu.Unlock()
	}
	resolve := func(context.Context, mcpserver.ID) (*Launch, error) { return input, nil }
	begin := c.BeginConnect
	if authorize {
		begin = c.BeginAuthorize
	}
	attempt, err := begin(ctx, name, resolve)
	if err != nil {
		return errors.Join(err, input.Close())
	}
	defer attempt.Close()
	return attempt.Run()
}
