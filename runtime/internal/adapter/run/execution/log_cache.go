package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strconv"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	scoperun "github.com/Tangerg/scope/agent/run"
)

// A cache borrows only an immutable, acknowledged log. Every read still checks
// the durable head; it cannot grant a writer or advance an execution. Public
// Store reads clone records so a consumer cannot mutate the cached owner.
type executionLogCache struct {
	mu          sync.Mutex
	digest      string
	log         scoperun.Log
	view        *scoperun.TreeInspection
	entriesJSON string
}

func (c *executionLogCache) decode(head runs.ExecutionTreeHead, root scoperun.ProcessID) (scoperun.Log, error) {
	if scoperun.ComputeDigest(head.Payload).String() != head.Digest {
		return scoperun.Log{}, errors.New("execution: execution log storage digest mismatch")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.digest == head.Digest && c.log.Root == root && uint64(c.log.Head()) == head.Sequence && strconv.FormatUint(uint64(c.log.Epoch()), 10) == head.Writer {
		return c.log, nil
	}
	history, err := decodeExecutionTree(head, root)
	if err != nil {
		return scoperun.Log{}, err
	}
	c.digest, c.log, c.view = head.Digest, history, nil
	c.entriesJSON = ""
	return history, nil
}

func (c *executionLogCache) remember(head runs.ExecutionTreeHead, history scoperun.Log, entriesJSON string) {
	c.mu.Lock()
	c.digest, c.log, c.view = head.Digest, history, nil
	c.entriesJSON = entriesJSON
	c.mu.Unlock()
}

func (c *executionLogCache) encodedEntries(head runs.ExecutionTreeHead, history scoperun.Log) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.digest == head.Digest && c.log.Root == history.Root && c.log.Head() == history.Head() && c.log.Epoch() == history.Epoch()
	if current && c.entriesJSON != "" {
		return c.entriesJSON, nil
	}
	encoded, err := json.Marshal(history.Entries, json.Deterministic(true))
	immutable := string(encoded)
	if err == nil && current {
		c.entriesJSON = immutable
	}
	return immutable, err
}

func cloneLogEntry(entry scoperun.Entry) (scoperun.Entry, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return scoperun.Entry{}, err
	}
	var cloned scoperun.Entry
	err = json.Unmarshal(data, &cloned)
	return cloned, err
}

func (i *interactionSession) inspectCommittedTree(ctx context.Context) (scoperun.TreeInspection, error) {
	history, err := i.committedTree(ctx, i.processRootID())
	if err != nil {
		return scoperun.TreeInspection{}, err
	}
	return i.logCache.inspect(history)
}

// All callers borrow this read-only projection of one acknowledged log cut.
// Reconciliation may inspect the same cut repeatedly while callbacks run.
func (c *executionLogCache) inspect(history scoperun.Log) (scoperun.TreeInspection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.log.Root == history.Root && c.log.Head() == history.Head() && c.log.Epoch() == history.Epoch()
	if current && c.view != nil {
		return *c.view, nil
	}
	view, err := history.Inspect()
	if err == nil && current {
		c.view = &view
	}
	return view, err
}
