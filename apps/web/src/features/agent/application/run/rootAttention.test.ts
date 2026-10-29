import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  EMPTY_AGENT_SESSION_VIEW,
  type AgentRunView,
  type AgentSessionView,
} from "@/features/agent/public/sessionView";
import {
  type AgentSessionViewEntry,
  type AgentSessionViewPort,
} from "@/features/agent/application/session/sessionAccess";
import { subscribeAnySessionRunning } from "@/features/agent/application/run/rootAttention";
let disposePort: (() => void) | undefined;

afterEach(() => {
  disposePort?.();
  disposePort = undefined;
});

function root(
  status: AgentRunView["status"],
  outcome: AgentRunView["outcome"] = null,
): AgentRunView {
  return {
    id: "root-run",
    sessionId: "session-1",
    parentRunId: null,
    rootRunId: "root-run",
    spawnedByItemId: null,
    status,
    activeSegmentId: status === "running" ? "segment-1" : null,
    outcome,
    metrics: {
      steps: 1,
      activeDurationMillis: 1,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: null,
    contextTokens: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: status === "finished" ? "2026-01-01T00:00:01.000Z" : null,
  };
}

function view(value: AgentRunView): AgentSessionView {
  return { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [value.id]: value } };
}

function entry(value: AgentSessionView): AgentSessionViewEntry {
  return {
    view: value,
    viewEpoch: 0n,
    viewRevision: 0n,
    authoritativeRevision: 0n,
    stop: null,
    send: null,
    resume: null,
    synchronize: null,
    cancelRun: null,
    openingCommands: [],
    retryOpening: null,
  };
}

function wire(initial: AgentSessionView) {
  let sessions: Record<string, AgentSessionViewEntry> = { "session-1": entry(initial) };
  let listener: ((next: Record<string, AgentSessionViewEntry>) => void) | undefined;
  disposePort = stubInstance(testClient.agent.dependencies.view, {
    getSessions: () => sessions,
    subscribeSessions: (next: (sessions: Record<string, AgentSessionViewEntry>) => void) => {
      listener = next;
      return () => {
        listener = undefined;
      };
    },
  } as unknown as AgentSessionViewPort);
  return (next: AgentSessionView) => {
    sessions = { "session-1": entry(next) };
    listener?.(sessions);
  };
}

describe("root Run attention", () => {
  it("publishes the current any-running state immediately and only on change", () => {
    const publish = wire(view(root("running")));
    const onChange = vi.fn();
    subscribeAnySessionRunning(testClient.agent.dependencies, onChange);

    expect(onChange).toHaveBeenCalledWith(true);
    publish(view(root("running")));
    expect(onChange).toHaveBeenCalledOnce();
    publish(view(root("waiting")));
    expect(onChange).toHaveBeenLastCalledWith(false);
  });
});
