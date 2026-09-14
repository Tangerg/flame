import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import {
  MODEL_INVOCATIONS_KEY,
  useModelInvocations,
  type ModelInvocation,
} from "./modelInvocations";

describe("model invocation progress refresh", () => {
  it("replaces an unfinished first read when committed Run progress advances", async () => {
    const call: ModelInvocation = {
      callId: "call_one",
      runId: "run_one",
      segmentId: "seg_one",
      state: "started",
      startedAt: "2026-09-14T01:00:00Z",
    };
    const pending = Promise.withResolvers<{ data: ModelInvocation[] }>();
    let completed = false;
    const fetcher = vi.fn(() =>
      completed
        ? Promise.resolve({
            data: [{ ...call, state: "completed", settledAt: "2026-09-14T01:00:02Z" }],
          })
        : pending.promise,
    );
    await loadPluginsForTest(
      definePlugin({
        name: "test.model-invocation-progress",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: MODEL_INVOCATIONS_KEY, fetcher });
        },
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const run = {
      id: "run_one",
      status: "running",
      metrics: { steps: 0 },
    } as AgentRunView;
    const { result, rerender, unmount } = renderHook(
      ({ run }) => ({ ...useModelInvocations(run) }),
      {
        initialProps: { run },
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryClientProvider client={client}>{children}</QueryClientProvider>
        ),
      },
    );
    try {
      await waitFor(() => expect(fetcher).toHaveBeenCalled());
      completed = true;
      const finished = { ...run, status: "finished", metrics: { steps: 1 } } as AgentRunView;
      rerender({ run: finished });
      await act(async () => pending.resolve({ data: [call] }));
      await waitFor(() => expect(result.current.data?.data[0]?.state).toBe("completed"));
      const reads = fetcher.mock.calls.length;
      rerender({ run: { ...finished } });
      expect(fetcher).toHaveBeenCalledTimes(reads);
      expect(client.getQueryCache().getAll()).toHaveLength(1);
    } finally {
      unmount();
      client.clear();
    }
  });
});
