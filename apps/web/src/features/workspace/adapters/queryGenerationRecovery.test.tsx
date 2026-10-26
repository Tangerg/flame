import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { QueryClientProvider } from "@tanstack/react-query";
import { act, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { createParameterizedDataQuery } from "@/extensions/dataQuery";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import {
  invalidateWorkspaceEverything,
  retireWorkspaceReadModels,
} from "@/features/workspace/adapters/queryInvalidation";
import { loadPluginsForTest } from "@/test/kernel";
const { synchronizeMountedAgentSessions } = vi.hoisted(() => ({
  synchronizeMountedAgentSessions: vi.fn(),
}));

vi.mock("@/features/agent/public/session", () => ({
  AGENT_SESSIONS_KEY: "agent-sessions",
  synchronizeMountedAgentSessions,
}));

interface FixtureState {
  readonly available: boolean;
  readonly goal: { readonly objective: string } | null;
}

const QUERY_GENERATION_FIXTURE_KEY = "query-generation-recovery-fixture";

const useGenerationFixture = createParameterizedDataQuery<{ sessionId: string }, FixtureState>(
  QUERY_GENERATION_FIXTURE_KEY,
);

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

function goal(objective: string): NonNullable<FixtureState["goal"]> {
  return { objective };
}

beforeEach(async () => {
  queryClient.clear();
  synchronizeMountedAgentSessions.mockClear();
});

afterEach(() => {
  queryClient.clear();
});

describe("workspace Runtime-generation query recovery", () => {
  it("revokes an old query before the successor event tail starts its replacement", async () => {
    const retired = Promise.withResolvers<FixtureState>();
    let firstSignal: AbortSignal | undefined;
    let calls = 0;
    const fetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      calls += 1;
      if (calls === 1) {
        firstSignal = signal;
        return retired.promise;
      }
      return Promise.resolve({ available: true, goal: goal("successor goal") });
    });
    await loadPluginsForTest(
      definePlugin({
        name: "test.goal-two-phase-generation-recovery",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: QUERY_GENERATION_FIXTURE_KEY, fetcher });
        },
      }),
    );
    const { result } = testClient.renderHook(
      () => useGenerationFixture({ sessionId: "ses_goal_generation" }),
      {
        wrapper,
      },
    );
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());

    act(() => retireWorkspaceReadModels(testClient.agent.access));

    expect(firstSignal?.aborted).toBe(true);
    expect(fetcher).toHaveBeenCalledOnce();
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith(testClient.agent.access, {
      ownership: "retire-live",
    });
    retired.resolve({ available: true, goal: goal("retired goal") });
    await act(async () => Promise.resolve());
    expect(result.current.data).toBeUndefined();
    expect(fetcher).toHaveBeenCalledOnce();

    act(() => invalidateWorkspaceEverything(testClient.agent.access));

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.data?.goal?.objective).toBe("successor goal"));
  });

  it("replaces an initial parameterized read before the old Runtime settles", async () => {
    const retired = Promise.withResolvers<FixtureState>();
    let firstSignal: AbortSignal | undefined;
    let calls = 0;
    const fetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      calls += 1;
      if (calls === 1) {
        firstSignal = signal;
        return retired.promise;
      }
      return Promise.resolve({ available: true, goal: goal("successor goal") });
    });
    await loadPluginsForTest(
      definePlugin({
        name: "test.goal-generation-recovery",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: QUERY_GENERATION_FIXTURE_KEY, fetcher });
        },
      }),
    );
    const { result } = testClient.renderHook(
      () => useGenerationFixture({ sessionId: "ses_goal_generation" }),
      {
        wrapper,
      },
    );
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());

    act(() => invalidateWorkspaceEverything(testClient.agent.access));

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.data?.goal?.objective).toBe("successor goal"));
    expect(firstSignal?.aborted).toBe(true);
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith(testClient.agent.access, {
      ownership: "replace-live",
    });

    retired.resolve({ available: true, goal: goal("retired goal") });
    await act(async () => Promise.resolve());
    expect(result.current.data?.goal?.objective).toBe("successor goal");
  });
});
