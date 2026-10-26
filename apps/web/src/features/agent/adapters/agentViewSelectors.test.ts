import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act } from "@testing-library/react";

import { afterEach, describe, expect, it } from "vitest";
import {
  EMPTY_AGENT_SESSION_VIEW,
  type AgentProblem,
  type Message,
} from "@/features/agent/public/sessionView";

function seed(commandError: AgentProblem | null, shared: Record<string, unknown>) {
  return {
    view: { ...EMPTY_AGENT_SESSION_VIEW, shared },
    problem: { commandError, dismissedRunId: null },
    viewEpoch: 0n,
    viewRevision: 0n,
    authoritativeRevision: 0n,
    refreshSequence: 0n,
    stop: null,
    send: null,
    resume: null,
    synchronize: null,
    cancelRun: null,
    openingCommands: [],
    retryOpening: null,
  };
}

afterEach(() => {
  testClient.agent.projection.store.setState({ sessions: {} });
  testClient.navigation.go({ session: "" });
});

describe("agent view selectors react to session switch", () => {
  it("testClient.selectors.useAgentProblem follows activeSessionId", () => {
    testClient.agent.projection.store.setState({
      sessions: { a: seed({ message: "A" }, {}), b: seed({ message: "B" }, {}) },
    });
    testClient.navigation.go({ session: "a" });

    const { result } = testClient.renderHook(() => testClient.selectors.useAgentProblem());
    expect(result.current?.message).toBe("A");

    act(() => testClient.navigation.go({ session: "b" }));
    expect(result.current?.message).toBe("B");
  });

  it("testClient.selectors.useAgentSharedMaterial follows activeSessionId with its exact projection generation", () => {
    testClient.agent.projection.store.setState({
      sessions: {
        a: seed(null, { k: "A" }),
        b: { ...seed(null, { k: "B" }), viewEpoch: 4n },
      },
    });
    testClient.navigation.go({ session: "a" });

    const { result } = testClient.renderHook(() =>
      testClient.selectors.useAgentSharedMaterial<string>("k"),
    );
    expect(result.current).toEqual({ generation: 0n, value: "A" });

    act(() => testClient.navigation.go({ session: "b" }));
    expect(result.current).toEqual({ generation: 4n, value: "B" });
  });

  it("keeps the exact root Run referentially stable between unchanged renders", () => {
    const running = {
      id: "run-1",
      sessionId: "a",
      parentRunId: null,
      rootRunId: "run-1",
      spawnedByItemId: null,
      status: "running" as const,
      activeSegmentId: "segment-1",
      outcome: null,
      metrics: {
        steps: 0,
        activeDurationMillis: 0,
        usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
      },
      progress: null,
      contextTokens: null,
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: null,
    };
    testClient.agent.projection.store.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [running.id]: running } },
        },
      },
    });
    testClient.navigation.go({ session: "a" });

    const { result, rerender } = testClient.renderHook(() =>
      testClient.selectors.useCurrentRootRun(),
    );
    const first = result.current;
    rerender();

    expect(result.current).toBe(first);
    expect(result.current).toBe(running);
  });

  it("does not re-render the exact root Run subscriber for unrelated projection writes", () => {
    const outcome = { type: "completed" as const };
    const finished = {
      id: "run-1",
      sessionId: "a",
      parentRunId: null,
      rootRunId: "run-1",
      spawnedByItemId: null,
      status: "finished" as const,
      activeSegmentId: null,
      outcome,
      metrics: {
        steps: 1,
        activeDurationMillis: 10,
        usage: { inputTokens: 3, outputTokens: 2, cacheReadTokens: 0 },
      },
      progress: null,
      contextTokens: null,
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: "2026-01-01T00:00:01.000Z",
    };
    testClient.agent.projection.store.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [finished.id]: finished } },
        },
      },
    });
    testClient.navigation.go({ session: "a" });

    let renders = 0;
    const { result } = testClient.renderHook(() => {
      renders += 1;
      return testClient.selectors.useCurrentRootRun();
    });
    expect(result.current).toBe(finished);

    act(() =>
      testClient.agent.projection.store.setState((state) => {
        const current = state.sessions.a!;
        return {
          sessions: {
            ...state.sessions,
            a: {
              ...current,
              view: { ...current.view, shared: { unrelated: true } },
              viewRevision: current.viewRevision + 1n,
            },
          },
        };
      }),
    );

    expect(renders).toBe(1);
    expect(result.current).toBe(finished);
  });

  it("holds an attention subscriber still while the same Run reports progress", () => {
    const run = {
      id: "run-1",
      sessionId: "a",
      parentRunId: null,
      rootRunId: "run-1",
      spawnedByItemId: null,
      status: "running" as const,
      activeSegmentId: "segment-1",
      outcome: null,
      metrics: {
        steps: 1,
        activeDurationMillis: 10,
        usage: { inputTokens: 3, outputTokens: 2, cacheReadTokens: 0 },
      },
      progress: null,
      contextTokens: 100,
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: null,
    };
    testClient.agent.projection.store.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [run.id]: run } },
        },
      },
    });
    testClient.navigation.go({ session: "a" });

    let renders = 0;
    const { result } = testClient.renderHook(() => {
      renders += 1;
      return testClient.selectors.useCurrentRootRunning();
    });
    expect(result.current).toBe(true);

    for (const contextTokens of [200, 300, 400]) {
      act(() =>
        testClient.agent.projection.store.setState((state) => {
          const current = state.sessions.a!;
          return {
            sessions: {
              ...state.sessions,
              a: {
                ...current,
                view: {
                  ...current.view,
                  runsById: { [run.id]: { ...run, contextTokens } },
                },
                viewRevision: current.viewRevision + 1n,
              },
            },
          };
        }),
      );
    }

    expect(renders).toBe(1);

    act(() =>
      testClient.agent.projection.store.setState((state) => {
        const current = state.sessions.a!;
        return {
          sessions: {
            ...state.sessions,
            a: {
              ...current,
              view: {
                ...current.view,
                runsById: { [run.id]: { ...run, status: "finished" as const } },
              },
              viewRevision: current.viewRevision + 1n,
            },
          },
        };
      }),
    );

    expect(result.current).toBe(false);
    expect(renders).toBe(2);
  });

  it("keeps the transcript collection snapshot stable when every row is unchanged", () => {
    const message: Message = {
      id: "message-1",
      runId: null,
      role: "user",
      blocks: [{ kind: "text", text: "hello", status: "complete" }],
    };
    testClient.agent.projection.store.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, messages: [message] },
        },
      },
    });
    testClient.navigation.go({ session: "a" });

    const { result, rerender } = testClient.renderHook(() =>
      testClient.selectors.useTranscriptRows(),
    );
    const first = result.current;
    rerender();

    expect(result.current).toBe(first);
    expect(result.current[0]?.message).toBe(message);
  });

  it("shows the switched-to session's transcript, not the cached one it came from", () => {
    const inA: Message = {
      id: "message-a",
      runId: null,
      role: "user",
      blocks: [{ kind: "text", text: "from A", status: "complete" }],
    };
    const inB: Message = {
      id: "message-b",
      runId: null,
      role: "user",
      blocks: [{ kind: "text", text: "from B", status: "complete" }],
    };
    testClient.agent.projection.store.setState({
      sessions: {
        a: { ...seed(null, {}), view: { ...EMPTY_AGENT_SESSION_VIEW, messages: [inA] } },
        b: { ...seed(null, {}), view: { ...EMPTY_AGENT_SESSION_VIEW, messages: [inB] } },
      },
    });

    testClient.navigation.go({ session: "a" });
    const { result } = testClient.renderHook(() => testClient.selectors.useTranscriptRows());
    expect(result.current.map((row) => row.message.id)).toEqual(["message-a"]);

    act(() => testClient.navigation.go({ session: "b" }));
    expect(result.current.map((row) => row.message.id)).toEqual(["message-b"]);

    act(() => testClient.navigation.go({ session: "a" }));
    expect(result.current.map((row) => row.message.id)).toEqual(["message-a"]);
  });

  it("answers an empty transcript for a session the store has never seen", () => {
    testClient.agent.projection.store.setState({ sessions: {} });
    testClient.navigation.go({ session: "absent" });
    const { result } = testClient.renderHook(() => testClient.selectors.useTranscriptRows());
    expect(result.current).toEqual([]);
  });
});
