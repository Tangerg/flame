import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { FlameClient, Schedule } from "@/rpc";
import {
  createSchedule,
  runScheduleNow,
  setScheduleEnabled,
  updateSchedule,
} from "../application/scheduleCommands";
import { SCHEDULES_KEY } from "../application/scheduleQueries";
import { installScheduleGateway, registerScheduleDataProvider } from "./runtimeScheduleGateway";
import { contributeForTest } from "@/plugins/sdk/testKernel";
import { lookupDataProvider } from "@/plugins/sdk/selectors";

const { selectAgentSession, runtimeCapability } = vi.hoisted(() => ({
  selectAgentSession: vi.fn(),
  runtimeCapability: vi.fn(() => true),
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({ selectAgentSession }));
// The published facade of a foreign context, which is the only surface a test here may
// reach for — the capability port behind it is Runtime's own business.
vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({ runtimeCapability }));

let installation: ReturnType<typeof installScheduleGateway> | undefined;

afterEach(() => {
  installation?.dispose();
  installation = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [SCHEDULES_KEY] });
  selectAgentSession.mockReset();
  runtimeCapability.mockReset();
  runtimeCapability.mockReturnValue(true);
});

function schedule(workspace?: { path: string }): Schedule {
  return {
    id: "schedule-1",
    title: "Review",
    instructions: "Review changes",
    ...(workspace ? { workspace } : {}),
    cron: "0 9 * * 1",
    enabled: true,
    createdAt: "2026-08-12T00:00:00Z",
    revision: 1,
  };
}

describe("runtimeScheduleGateway", () => {
  it("omits workspace when a new schedule deliberately uses the Runtime default", async () => {
    const create = vi.fn().mockResolvedValue(schedule());
    setContainer({ client: () => ({ schedules: { create } }) as unknown as FlameClient });
    installation = installScheduleGateway();

    await createSchedule({
      title: "Review",
      instructions: "Review changes",
      cwd: "",
      cron: "0 9 * * 1",
    });

    expect(create).toHaveBeenCalledWith({
      title: "Review",
      instructions: "Review changes",
      cron: "0 9 * * 1",
    });
  });

  it("uses the explicit Runtime-default mode when an edit clears a binding", async () => {
    const update = vi.fn().mockResolvedValue(schedule());
    setContainer({ client: () => ({ schedules: { update } }) as unknown as FlameClient });
    installation = installScheduleGateway();

    await updateSchedule({
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cwd: "",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    });

    expect(update).toHaveBeenCalledWith({
      id: "schedule-1",
      expectedRevision: 7,
      title: "Review",
      instructions: "Review changes",
      workspaceMode: "default",
      cron: "0 9 * * 1",
      enabled: true,
    });
  });

  it("sends a valid workspace ref when an edit sets an explicit binding", async () => {
    const update = vi.fn().mockResolvedValue(schedule({ path: "/workspace" }));
    setContainer({ client: () => ({ schedules: { update } }) as unknown as FlameClient });
    installation = installScheduleGateway();

    await updateSchedule({
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cwd: "/workspace",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    });

    expect(update).toHaveBeenCalledWith({
      id: "schedule-1",
      expectedRevision: 7,
      title: "Review",
      instructions: "Review changes",
      workspace: { path: "/workspace" },
      cron: "0 9 * * 1",
      enabled: true,
    });
  });

  it("preserves the launched session and run identities from run-now", async () => {
    const runNow = vi.fn().mockResolvedValue({ sessionId: "ses_scheduled", runId: "run_1" });
    setContainer({ client: () => ({ schedules: { runNow } }) as unknown as FlameClient });
    installation = installScheduleGateway();

    await expect(runScheduleNow("schedule-1")).resolves.toEqual({
      sessionId: "ses_scheduled",
      runId: "run_1",
    });
    expect(runNow).toHaveBeenCalledWith("schedule-1");
  });

  it("preserves the authoritative revision returned by an enablement change", async () => {
    const updated = { ...schedule(), enabled: false, revision: 8 };
    const update = vi.fn().mockResolvedValue(updated);
    setContainer({ client: () => ({ schedules: { update } }) as unknown as FlameClient });
    installation = installScheduleGateway();

    await expect(setScheduleEnabled(schedule(), false)).resolves.toMatchObject({
      enabled: false,
      revision: 8,
    });
  });

  it("does not navigate when a retired run-now response arrives after replacement", async () => {
    const retiredRun = deferred<{ sessionId: string; runId: string }>();
    const runNowRetired = vi.fn(() => retiredRun.promise);
    const runNowSuccessor = vi.fn().mockResolvedValue({
      sessionId: "ses_successor",
      runId: "run_successor",
    });
    setContainer({
      client: () => ({ schedules: { runNow: runNowRetired } }) as unknown as FlameClient,
    });
    const retiredInstallation = installScheduleGateway();
    const command = rejected(runScheduleNow("schedule-1"));
    await vi.waitFor(() => expect(runNowRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ schedules: { runNow: runNowSuccessor } }) as unknown as FlameClient,
    });
    const successorInstallation = installScheduleGateway();
    installation = {
      replaceRuntimeGeneration: () => successorInstallation.replaceRuntimeGeneration(),
      dispose() {
        successorInstallation.dispose();
        retiredInstallation.dispose();
      },
    };
    retiredRun.resolve({ sessionId: "ses_retired", runId: "run_retired" });

    await expect(command).resolves.toMatchObject({
      message: "schedule_mutation_generation_retired",
    });
    expect(runNowSuccessor).not.toHaveBeenCalled();
    expect(selectAgentSession).not.toHaveBeenCalled();
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}

describe("the schedules read", () => {
  // This translation had two authors — this provider and a second one in the defaults
  // context — so the wire shape could change under one of them silently. It has one now,
  // and this is what it promises.
  async function read(): Promise<unknown> {
    await contributeForTest((ctx) => {
      registerScheduleDataProvider(ctx);
    }, "test.schedule-data-provider");
    const fetcher = lookupDataProvider(SCHEDULES_KEY);
    expect(fetcher).toBeDefined();
    return fetcher!();
  }

  it("flattens the Runtime's nested workspace onto the config's cwd", async () => {
    const list = vi.fn(() => ({
      autoPagingToArray: () => Promise.resolve([schedule({ path: "/repo" })]),
    }));
    setContainer({ client: () => ({ schedules: { list } }) as unknown as FlameClient });

    await expect(read()).resolves.toEqual([
      expect.objectContaining({ id: "schedule-1", cwd: "/repo" }),
    ]);
    expect(await read()).not.toContainEqual(
      expect.objectContaining({ workspace: expect.anything() }),
    );
  });

  it("omits cwd entirely when the Runtime sent no workspace", async () => {
    const list = vi.fn(() => ({ autoPagingToArray: () => Promise.resolve([schedule()]) }));
    setContainer({ client: () => ({ schedules: { list } }) as unknown as FlameClient });

    const [config] = (await read()) as Record<string, unknown>[];
    expect(config).not.toHaveProperty("cwd");
  });

  it("answers empty without reaching the wire when the Runtime cannot schedule", async () => {
    runtimeCapability.mockReturnValue(false);
    const list = vi.fn();
    setContainer({ client: () => ({ schedules: { list } }) as unknown as FlameClient });

    await expect(read()).resolves.toEqual([]);
    expect(list).not.toHaveBeenCalled();
  });
});
