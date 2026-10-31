package runs

import (
	"context"
	"testing"
	"time"
)

func shutdownCoordinator(c *testRuns) error {
	c.segments.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.segments.AwaitShutdown(ctx)
}

func requireCoordinatorShutdown(t testing.TB, c *testRuns) {
	t.Helper()
	if err := shutdownCoordinator(c); err != nil {
		t.Fatalf("shutdown run coordinator: %v", err)
	}
}
