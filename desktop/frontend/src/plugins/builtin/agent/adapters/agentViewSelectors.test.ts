// Regression: useAgentProblem / useAgentSharedMaterial must react to an
// activeSessionId switch, not just to agent-store mutations. They read the
// active session's view, and activeSessionId lives in a SEPARATE store
// (useAgentSessionStore); if the switch isn't a reactive dependency, a
// consumer keeps rendering the previous session's error / Plan until
// the agent store happens to mutate. Locking the reactive contract here keeps
// these two selectors from drifting off the useActiveAgentView pattern the
// other view selectors share.

import { act, renderHook } from "@testing-library/react";
import { navigator } from "@/lib/navigation";
import { afterEach, describe, expect, it } from "vitest";
import {
  EMPTY_AGENT_SESSION_VIEW,
  type AgentProblem,
  type Message,
} from "@/plugins/sdk/types/agentSessionView";
import { useAgentStore } from "./agentStore";
import {
  useAgentProblem,
  useAgentSharedMaterial,
  useCurrentRootRun,
  useCurrentRootRunning,
  useTranscriptRows,
} from "./agentViewSelectors";

function seed(commandError: AgentProblem | null, shared: Record<string, unknown>) {
  return {
    view: { ...EMPTY_AGENT_SESSION_VIEW, commandError, shared },
    viewEpoch: 0n,
    viewRevision: 0n,
    authoritativeRevision: 0n,
    refreshSequence: 0n,
    stop: null,
    send: null,
    resume: null,
    synchronize: null,
    cancelRun: null,
  };
}

afterEach(() => {
  useAgentStore.setState({ sessions: {} });
  navigator().go({ session: "" });
});

describe("agent view selectors react to session switch", () => {
  it("useAgentProblem follows activeSessionId", () => {
    useAgentStore.setState({
      sessions: { a: seed({ message: "A" }, {}), b: seed({ message: "B" }, {}) },
    });
    navigator().go({ session: "a" });

    const { result } = renderHook(() => useAgentProblem());
    expect(result.current?.message).toBe("A");

    act(() => navigator().go({ session: "b" }));
    expect(result.current?.message).toBe("B");
  });

  it("useAgentSharedMaterial follows activeSessionId with its exact projection generation", () => {
    useAgentStore.setState({
      sessions: {
        a: seed(null, { k: "A" }),
        b: { ...seed(null, { k: "B" }), viewEpoch: 4n },
      },
    });
    navigator().go({ session: "a" });

    const { result } = renderHook(() => useAgentSharedMaterial<string>("k"));
    expect(result.current).toEqual({ generation: 0n, value: "A" });

    act(() => navigator().go({ session: "b" }));
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
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: null,
    };
    useAgentStore.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [running.id]: running } },
        },
      },
    });
    navigator().go({ session: "a" });

    const { result, rerender } = renderHook(() => useCurrentRootRun());
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
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: "2026-01-01T00:00:01.000Z",
    };
    useAgentStore.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [finished.id]: finished } },
        },
      },
    });
    navigator().go({ session: "a" });

    let renders = 0;
    const { result } = renderHook(() => {
      renders += 1;
      return useCurrentRootRun();
    });
    expect(result.current).toBe(finished);

    act(() =>
      useAgentStore.setState((state) => {
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
      progress: { contextTokens: 100 },
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: null,
    };
    useAgentStore.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [run.id]: run } },
        },
      },
    });
    navigator().go({ session: "a" });

    let renders = 0;
    const { result } = renderHook(() => {
      renders += 1;
      return useCurrentRootRunning();
    });
    expect(result.current).toBe(true);

    for (const contextTokens of [200, 300, 400]) {
      act(() =>
        useAgentStore.setState((state) => {
          const current = state.sessions.a!;
          return {
            sessions: {
              ...state.sessions,
              a: {
                ...current,
                view: {
                  ...current.view,
                  runsById: { [run.id]: { ...run, progress: { contextTokens } } },
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
      useAgentStore.setState((state) => {
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
    useAgentStore.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, messages: [message] },
        },
      },
    });
    navigator().go({ session: "a" });

    const { result, rerender } = renderHook(() => useTranscriptRows());
    const first = result.current;
    rerender();

    expect(result.current).toBe(first);
    expect(result.current[0]?.message).toBe(message);
  });

  // The projection instance holds a per-session row cache and is rebuilt through `useMemo`
  // keyed on the session. If that key were ever dropped, the cache would answer the new
  // session with the previous one's rows — the transcript of a conversation you are not in.
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
    useAgentStore.setState({
      sessions: {
        a: { ...seed(null, {}), view: { ...EMPTY_AGENT_SESSION_VIEW, messages: [inA] } },
        b: { ...seed(null, {}), view: { ...EMPTY_AGENT_SESSION_VIEW, messages: [inB] } },
      },
    });

    navigator().go({ session: "a" });
    const { result } = renderHook(() => useTranscriptRows());
    expect(result.current.map((row) => row.message.id)).toEqual(["message-a"]);

    act(() => navigator().go({ session: "b" }));
    expect(result.current.map((row) => row.message.id)).toEqual(["message-b"]);

    act(() => navigator().go({ session: "a" }));
    expect(result.current.map((row) => row.message.id)).toEqual(["message-a"]);
  });

  it("answers an empty transcript for a session the store has never seen", () => {
    useAgentStore.setState({ sessions: {} });
    navigator().go({ session: "absent" });
    const { result } = renderHook(() => useTranscriptRows());
    expect(result.current).toEqual([]);
  });
});
