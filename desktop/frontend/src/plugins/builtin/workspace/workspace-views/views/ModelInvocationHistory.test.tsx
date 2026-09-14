import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
const query = vi.hoisted(() => ({ read: vi.fn(), refetch: vi.fn() }));
vi.mock("@/plugins/builtin/agent/public/run", () => ({ useModelInvocations: query.read }));
import { ModelInvocationHistory } from "./ModelInvocationHistory";

describe("Model invocation history", () => {
  it("does not present recovery settlement as measured execution and pages only on request", () => {
    query.read.mockImplementation((_run, cursor) => ({
      data: cursor
        ? { data: [] }
        : {
            data: [
              {
                callId: "call_unknown",
                runId: "run_one",
                segmentId: "seg_one",
                state: "unknown",
                startedAt: "2026-09-14T01:00:00Z",
                settledAt: "2026-09-14T02:00:00Z",
              },
              {
                callId: "call_done",
                firstOutputLatencyMillis: 0,
                usage: {
                  inputTokens: 123,
                  outputTokens: 0,
                  cacheReadTokens: 31,
                  cacheWriteTokens: 0,
                  reasoningTokens: 0,
                },
                runId: "run_one",
                segmentId: "seg_one",
                state: "completed",
                startedAt: "2026-09-14T01:00:00Z",
                settledAt: "2026-09-14T01:00:02Z",
              },
            ],
            nextCursor: "older",
          },
      isLoading: false,
      isError: false,
      refetch: query.refetch,
    }));
    const run = { id: "run_one" } as AgentRunView;
    render(<ModelInvocationHistory run={run} />);
    expect(screen.getByText("Outcome unknown")).toBeTruthy();
    expect(screen.getByText("—")).toBeTruthy();
    expect(screen.getByText("2s")).toBeTruthy();
    expect(screen.getByText("First output 0ms")).toBeTruthy();
    expect(screen.getByText("↑123")).toBeTruthy();
    expect(screen.getByText("↓0")).toBeTruthy();
    expect(screen.getByText("↑—")).toBeTruthy();
    expect(screen.getByText("↓—")).toBeTruthy();
    expect(screen.getByText("cache read 31")).toBeTruthy();
    expect(screen.queryByText("1h 00m")).toBeNull();
    expect(query.read).toHaveBeenLastCalledWith(run, undefined);
    fireEvent.click(screen.getByRole("button", { name: "Older calls" }));
    expect(query.read).toHaveBeenLastCalledWith(run, "older");
    expect(screen.getByText("No retained model call records")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Latest calls" }));
    expect(query.read).toHaveBeenLastCalledWith(run, undefined);
    fireEvent.click(screen.getByRole("button", { name: "Refresh model calls" }));
    expect(query.refetch).toHaveBeenCalledTimes(1);
  });
});
