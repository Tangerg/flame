import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { Activity, Suspense, createElement, type ReactNode, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { createParameterizedDataQuery } from "@/extensions/dataQuery";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import {
  invalidateWorkspaceEverything,
  replaceWorkspaceServerScope,
  retireWorkspaceReadModels,
} from "@/features/workspace/adapters/queryInvalidation";
import {
  useWorkspaceReadFile,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceFileContent,
} from "@/features/workspace/application/workspaceQueries";
import { loadPluginsForTest } from "@/test/kernel";
const { synchronizeMountedAgentSessions } = vi.hoisted(() => ({
  synchronizeMountedAgentSessions: vi.fn(),
}));

vi.mock("@/features/agent/public/session", () => ({ AGENT_SESSIONS_KEY: "agent-sessions" }));
prepareTestClient(() => {
  Object.assign(testClient.agent.access, { synchronizeMountedAgentSessions });
});

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
  it.each([
    { lifetime: "Activity", evictPrevious: false },
    { lifetime: "Suspense", evictPrevious: false },
    { lifetime: "Activity", evictPrevious: true },
    { lifetime: "Suspense", evictPrevious: true },
  ])(
    "retires continuous file placeholders across server round trips in $lifetime (evictPrevious=$evictPrevious)",
    async ({ lifetime, evictPrevious }) => {
      const content = (value: string): WorkspaceFileContent => ({
        content: value,
        startLine: 1,
        endLine: 10,
        totalLines: 100,
      });
      let read = Promise.resolve(content("original source"));
      const fetcher = vi.fn((_params?: unknown) => read);
      await loadPluginsForTest(
        definePlugin({
          name: "test.continuous-file-server-replacement",
          setup(ctx) {
            ctx.contributions.add(DATA_PROVIDER, { key: WORKSPACE_READ_FILE_KEY, fetcher });
          },
        }),
      );
      function Reader() {
        const [page, setPage] = useState(0);
        const file = useWorkspaceReadFile({
          cwd: "/work",
          path: "guide.md",
          startLine: page * 10 + 1,
        });
        return (
          <div>
            <button onClick={() => setPage((current) => current + 1)}>Next page</button>
            <output>{file.data?.content ?? "Waiting for file"}</output>
          </div>
        );
      }
      const child = <Reader />;
      const pending = new Promise<never>(() => {});
      function Pending(): never {
        throw pending;
      }
      const scene = (visible: boolean) =>
        lifetime === "Activity" ? (
          <Activity mode={visible ? "visible" : "hidden"}>{child}</Activity>
        ) : (
          <Suspense fallback={<p>Waiting for reader</p>}>
            {!visible && <Pending />}
            {child}
          </Suspense>
        );
      const view = testClient.render(scene(true), { wrapper });
      let previous = "original source";
      await screen.findByText(previous);
      expect(fetcher).toHaveBeenCalledOnce();

      for (const next of ["second source", "returned source"]) {
        const retired = Promise.withResolvers<WorkspaceFileContent>();
        const successor = Promise.withResolvers<WorkspaceFileContent>();
        finishTestClient(() => {
          retired.resolve(content("retired source"));
          successor.resolve(content(next));
        });
        read = retired.promise;
        const before = fetcher.mock.calls.length;
        const previousRequest = fetcher.mock.calls.at(-1)![0];
        fireEvent.click(screen.getByRole("button", { name: "Next page" }));
        await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(before + 1));
        expect(screen.getByText(previous)).toBeTruthy();
        view.rerender(scene(false));
        const queryKey = [WORKSPACE_READ_FILE_KEY, previousRequest];
        if (evictPrevious) {
          act(() => queryClient.removeQueries({ queryKey, exact: true }));
        }
        expect(queryClient.getQueryData<WorkspaceFileContent>(queryKey)?.content).toBe(
          evictPrevious ? undefined : previous,
        );

        read = successor.promise;
        act(() => replaceWorkspaceServerScope(testClient.agent.access));
        view.rerender(scene(true));

        expect(screen.queryByText(previous)).toBeNull();
        await act(async () => successor.resolve(content(next)));
        await screen.findByText(next);
        await act(async () => retired.resolve(content("retired source")));
        expect(screen.queryByText("retired source")).toBeNull();
        expect(fetcher).toHaveBeenCalledTimes(before + 2);
        previous = next;
      }
    },
  );

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
          ctx.contributions.add(DATA_PROVIDER, { key: QUERY_GENERATION_FIXTURE_KEY, fetcher });
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
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
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
          ctx.contributions.add(DATA_PROVIDER, { key: QUERY_GENERATION_FIXTURE_KEY, fetcher });
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
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "replace-live",
    });

    retired.resolve({ available: true, goal: goal("retired goal") });
    await act(async () => Promise.resolve());
    expect(result.current.data?.goal?.objective).toBe("successor goal");
  });
});
