import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import type { FlameClient } from "@flame/runtime-contract/client";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
} from "@/plugins/builtin/runtime/public/services";
import { runScheduleNow } from "./application/scheduleCommands";
import { SCHEDULES_KEY } from "./application/scheduleQueries";
import { createSchedulesPlugin } from "./index";
import { rejected } from "@/test/rejected";

const { selectAgentSession } = vi.hoisted(() => ({ selectAgentSession: vi.fn() }));

vi.mock("@/plugins/builtin/agent/public/session", () => ({ selectAgentSession }));

afterEach(async () => {
  await resetKernelForTest();

  queryClient.removeQueries({ queryKey: [SCHEDULES_KEY] });
  selectAgentSession.mockReset();
});

describe("schedules plugin Runtime generation wiring", () => {
  it("retires run-now navigation when the Runtime process generation changes", async () => {
    const retired = Promise.withResolvers<{ sessionId: string; runId: string }>();
    const runNow = vi.fn(() => retired.promise);
    let runtimeClient = () => ({ schedules: { runNow } }) as unknown as FlameClient;
    let generation = RuntimeConnectionGeneration.forProcess("runtime_1");
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.runtime-generation",
      provides: { stream: RUNTIME_STREAM },
      setup() {
        return {
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            reportConnectionLoss: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(
      runtime,
      createSchedulesPlugin(() => runtimeClient()),
    );

    const command = rejected(runScheduleNow("sch_1"));
    await vi.waitFor(() => expect(runNow).toHaveBeenCalledOnce());

    const successorRun = { sessionId: "ses_successor", runId: "run_successor" };
    const successorRunNow = vi.fn().mockResolvedValue(successorRun);
    runtimeClient = () => ({ schedules: { runNow: successorRunNow } }) as unknown as FlameClient;
    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "schedule_mutation_generation_retired",
    });
    expect(selectAgentSession).not.toHaveBeenCalled();

    retired.resolve({ sessionId: "ses_retired", runId: "run_retired" });
    await expect(runScheduleNow("sch_1")).resolves.toEqual(successorRun);
    expect(successorRunNow).toHaveBeenCalledExactlyOnceWith("sch_1");
    expect(selectAgentSession).toHaveBeenCalledExactlyOnceWith("ses_successor");
    expect(runNow).toHaveBeenCalledOnce();
  });
});
