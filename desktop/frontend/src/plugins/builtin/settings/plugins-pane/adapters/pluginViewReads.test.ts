import { expect, it, vi } from "vitest";
import { createMemoryViewReads, createScheduleViewReads } from "./pluginViewReads";
it("captures release and workspace identity while exposing only cursor continuation", async () => {
  const binding = { installationId: "installation", digest: "digest", viewId: "memory" };
  const target = { scope: "project" as const, workspace: { path: "/captured" } };
  const plugins = {
    readView: vi.fn(async () => ({ html: "<!doctype html>" })),
    readMemory: vi.fn(async () => ({ data: [] })),
  };
  const reads = createMemoryViewReads(plugins, binding, target);
  binding.digest = "replacement";
  target.workspace.path = "/replacement";
  const signal = new AbortController().signal;
  await reads.read("next", signal);
  expect(plugins.readMemory).toHaveBeenCalledWith(
    {
      installationId: "installation",
      digest: "digest",
      viewId: "memory",
      scope: "project",
      workspace: { path: "/captured" },
      cursor: "next",
    },
    signal,
  );
});
it("joins an accepted memory query when the HTML read fails", async () => {
  let memorySignal!: AbortSignal;
  let settle!: () => void;
  const pending = new Promise<{ data: [] }>((resolve) => (settle = () => resolve({ data: [] })));
  const plugins = {
    readView: vi.fn(async () => {
      throw new Error("package missing");
    }),
    readMemory: vi.fn((_request: unknown, signal?: AbortSignal) => {
      memorySignal = signal!;
      return pending;
    }),
  };
  const reads = createMemoryViewReads(
    plugins,
    { installationId: "installation", digest: "digest", viewId: "memory" },
    { scope: "user" },
  );
  let settled = false;
  const loading = reads.load(new AbortController().signal).catch((error) => {
    settled = true;
    return error;
  });
  await vi.waitFor(() => expect(memorySignal.aborted).toBe(true));
  expect(settled).toBe(false);
  settle();
  await expect(loading).resolves.toMatchObject({ message: "package missing" });
});

it("binds schedule continuation to the captured release without guest-selected targets", async () => {
  const binding = { installationId: "installation", digest: "digest", viewId: "schedules" };
  const plugins = {
    readView: vi.fn(async () => ({ html: "<!doctype html>" })),
    readSchedules: vi.fn(async () => ({ data: [] })),
  };
  const reads = createScheduleViewReads(plugins, binding);
  binding.digest = "successor";
  const signal = new AbortController().signal;
  await reads.load(signal);
  await reads.read("next", signal);
  expect(plugins.readSchedules).toHaveBeenLastCalledWith(
    { installationId: "installation", digest: "digest", viewId: "schedules", cursor: "next" },
    signal,
  );
});
