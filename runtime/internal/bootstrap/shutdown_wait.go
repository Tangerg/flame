package bootstrap

import (
	"errors"
	"time"
)

const defaultShutdownWaitTimeout = 10 * time.Second

// shutdownWaitPolicy is the caller-side wait budget for a process-owned
// shutdown generation. It is always concrete and positive: cancellation of a
// caller wait never doubles as cancellation of the underlying owner graph.
type shutdownWaitPolicy struct {
	timeout time.Duration
}

func newShutdownWaitPolicy(timeout time.Duration) (shutdownWaitPolicy, error) {
	if timeout <= 0 {
		return shutdownWaitPolicy{}, errors.New("runtime: shutdown wait timeout must be positive")
	}
	return shutdownWaitPolicy{timeout: timeout}, nil
}

func defaultShutdownWaitPolicy() shutdownWaitPolicy {
	policy, err := newShutdownWaitPolicy(defaultShutdownWaitTimeout)
	if err != nil {
		panic(err)
	}
	return policy
}

func shutdownWaitTimeout(p shutdownWaitPolicy) (time.Duration, error) {
	if p.timeout <= 0 {
		return 0, errors.New("runtime: shutdown wait policy is not configured")
	}
	return p.timeout, nil
}
