import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import type { AgentRunTreeNode } from "../view/runTree";
import { TRAJECTORY_KEY, useSessionTrajectory, type TrajectoryPage } from "./trajectory";

const projection = vi.hoisted(() => ({
  runs: [] as AgentRunTreeNode[],
}));
vi.mock("./runReadModel", () => ({
  useActiveSessionRunTree: () => projection.runs,
}));

describe("durable trajectory queries", () => {
  it("refreshes a silent model start when activity changes within the same step", async () => {
    const run = {
      id: "run_one",
      status: "running",
      activeSegmentId: "segment_one",
      metrics: { steps: 0 },
      progress: { step: 1, activity: "Preparing" },
    } as AgentRunTreeNode["run"];
    projection.runs = [{ run, children: [] }];
    const fetcher = vi.fn(async () => ({ data: [] }));
    await loadPluginsForTest(
      definePlugin({
        name: "test.trajectory-silent-start",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: TRAJECTORY_KEY, fetcher });
        },
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result, rerender, unmount } = renderHook(
      () => ({ ...useSessionTrajectory("session_one", true) }),
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryClientProvider client={client}>{children}</QueryClientProvider>
        ),
      },
    );
    try {
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      const reads = fetcher.mock.calls.length;
      projection.runs = [
        { run: { ...run, progress: { step: 1, activity: "Calling model" } }, children: [] },
      ];
      rerender();
      await waitFor(() => expect(fetcher.mock.calls.length).toBeGreaterThan(reads));
      const refreshed = fetcher.mock.calls.length;
      projection.runs = [
        {
          run: { ...projection.runs[0]!.run, progress: { step: 1, activity: "Calling model" } },
          children: [],
        },
      ];
      rerender();
      expect(fetcher).toHaveBeenCalledTimes(refreshed);
    } finally {
      unmount();
      client.clear();
    }
  });

  it("refreshes settled Run facts without refetching for equivalent projections or draining pages", async () => {
    const run = {
      id: "run_one",
      status: "running",
      activeSegmentId: "segment_one",
      metrics: { steps: 0 },
      progress: null,
    } as AgentRunTreeNode["run"];
    projection.runs = [{ run, children: [] }];
    const pending = Promise.withResolvers<TrajectoryPage>();
    const initial: TrajectoryPage = {
      data: [
        {
          type: "model",
          occurredAt: "2026-09-14T01:00:00Z",
          model: {
            callId: "call_one",
            runId: "run_one",
            segmentId: "segment_one",
            state: "started",
            startedAt: "2026-09-14T01:00:00Z",
          },
        },
      ],
      nextCursor: "older",
    };
    let settled = false;
    const fetcher = vi.fn((_params) => (settled ? Promise.resolve({ data: [] }) : pending.promise));
    await loadPluginsForTest(
      definePlugin({
        name: "test.session-trajectory",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: TRAJECTORY_KEY, fetcher });
        },
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result, rerender, unmount } = renderHook(
      ({ cursor }) => ({ ...useSessionTrajectory("session_one", true, cursor) }),
      {
        initialProps: { cursor: undefined as string | undefined },
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryClientProvider client={client}>{children}</QueryClientProvider>
        ),
      },
    );
    try {
      await waitFor(() => expect(fetcher).toHaveBeenCalled());
      const reads = fetcher.mock.calls.length;
      projection.runs = [{ run: { ...run }, children: [] }];
      rerender({ cursor: undefined });
      expect(fetcher).toHaveBeenCalledTimes(reads);
      settled = true;
      projection.runs = [
        { run: { ...run, status: "finished", activeSegmentId: null }, children: [] },
      ];
      rerender({ cursor: undefined });
      await act(async () => pending.resolve(initial));
      await waitFor(() => expect(result.current.data?.data).toEqual([]));
      expect(fetcher.mock.calls.every(([params]) => params.cursor === undefined)).toBe(true);
      const settledReads = fetcher.mock.calls.length;
      projection.runs = [{ run: { ...projection.runs[0]!.run }, children: [] }];
      rerender({ cursor: undefined });
      expect(fetcher).toHaveBeenCalledTimes(settledReads);
      rerender({ cursor: "older" });
      await waitFor(() =>
        expect(fetcher.mock.calls.some(([params]) => params.cursor === "older")).toBe(true),
      );
    } finally {
      unmount();
      client.clear();
    }
  });
});
