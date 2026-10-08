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

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

func TestRejectedLaunchRetiresItsResourceExactlyOnce(t *testing.T) {
	for _, refusal := range []string{"invalid configuration", "canceled configuration", "canceled authorization", "shutdown", "stdio authorization"} {
		t.Run(refusal, func(t *testing.T) {
			var retired atomic.Int32
			retirementErr := errors.New("retirement failed")
			config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: TransportStdio, Command: "must-not-start"}
			c := &Connections{lifetime: t.Context(), client: newClient()}
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
			input, err := NewLaunch(config, nil, func() error { retired.Add(1); return retirementErr })
			if strings.HasSuffix(refusal, "authorization") {
				c.servers = []*server{{id: config.ID(), config: config}}
				if err != nil {
					t.Fatal(err)
				}
				err = c.Authorize(ctx, input)
			} else if err == nil {
				err = c.Configure(ctx, input)
			}
			if !errors.Is(err, retirementErr) || retired.Load() != 1 {
				t.Fatalf("refusal lost or repeated resource retirement: %v, calls=%d", err, retired.Load())
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			if err := c.Configure(t.Context(), input); err == nil {
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
	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: TransportHTTP, Endpoint: transport.URL}
	input, err := NewLaunch(config, nil, func() error { retired.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	c := &Connections{lifetime: t.Context(), client: newClient()}
	t.Cleanup(func() { _ = c.Shutdown(context.WithoutCancel(t.Context())) })
	if err := c.Configure(t.Context(), input); err != nil {
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

func TestStartupAdmissionFailureRetiresTheUnlaunchedResource(t *testing.T) {
	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: TransportStdio, Command: "must-not-start"}
	var retired atomic.Int32
	c, err := Dial(t.Context(), t.Context(), []ServerConfig{config}, nil, func(context.Context, mcpserver.ID) (*Launch, error) {
		other := config.Clone()
		other.Name = testsupport.ServerName("other")
		return NewLaunch(other, nil, func() error { retired.Add(1); return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
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

func TestLaunchFormattingRedactsExecutionCredentials(t *testing.T) {
	secret := "private-execution-credential"
	input, err := NewLaunch(ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: TransportStdio, Command: "server", Env: []string{"TOKEN=" + secret}}, &Stdio{Command: "server", Env: []string{"TOKEN=" + secret}}, func() error { return nil })
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
	input, err := NewLaunch(ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: TransportStdio, Command: "must-not-start"}, nil, func() error { retired++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	copied := *input
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	c := &Connections{lifetime: t.Context(), client: newClient()}
	if err := c.Configure(t.Context(), &copied); err == nil {
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
	config := ServerConfig{Source: source, Name: testsupport.ServerName("server"), Transport: TransportStdio, Command: "must-not-start"}
	retired := 0
	if input, err := NewLaunch(config, nil, func() error { retired++; return nil }); err == nil || input != nil || retired != 1 {
		t.Fatalf("retained installation descriptor acquired an unverified launch: %v, retired=%d", err, retired)
	}
	if err := probe(t.Context(), config); err == nil {
		t.Fatal("probe launched a retained installation descriptor")
	}
}
