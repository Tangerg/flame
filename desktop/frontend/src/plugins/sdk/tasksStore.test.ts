import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { startTask, useTasksStore } from "./tasksStore";

const get = (id: string) => useTasksStore.getState().tasks.get(id);

beforeEach(() => {
  vi.useFakeTimers();
  useTasksStore.setState({ tasks: new Map() });
});
afterEach(() => vi.useRealTimers());

describe("startTask generation safety", () => {
  it("a settled task lingers then auto-removes", () => {
    const h = startTask("p", { id: "task:p:sync", label: "Sync" });
    h.succeed();
    expect(get("task:p:sync")?.status).toBe("succeeded");
    vi.runAllTimers();
    expect(get("task:p:sync")).toBeUndefined();
  });

  it("the previous settle's linger timer does not delete a restarted task", () => {
    const h1 = startTask("p", { id: "task:p:sync", label: "Sync" });
    h1.fail(new Error("boom"));

    startTask("p", { id: "task:p:sync", label: "Sync" });
    vi.runAllTimers();

    expect(get("task:p:sync")?.status).toBe("running");
  });

  it("cannot revise or re-settle a task it already settled", () => {
    const h = startTask("p", { id: "task:p:sync", label: "Sync" });
    h.succeed("done");

    h.update({ message: "late progress" });
    h.fail(new Error("late failure"));

    expect(get("task:p:sync")).toMatchObject({ status: "succeeded", message: "done" });
  });

  it("the previous handle cannot control a same-millisecond restart", () => {
    const h1 = startTask("p", { id: "task:p:sync", label: "Sync" });
    startTask("p", { id: "task:p:sync", label: "Sync" });

    h1.update({ message: "stale" });
    expect(get("task:p:sync")?.message).not.toBe("stale");

    h1.fail(new Error("late failure from the old attempt"));
    expect(get("task:p:sync")?.status).toBe("running");
  });
});
