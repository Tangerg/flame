import { expect, it, vi } from "vitest";
import {
  createMemoryViewReads,
  createScheduleViewReads,
  createUsageViewReads,
  createSkillViewReads,
  createDiffViewReads,
} from "./pluginViewReads";
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

it("captures Skill document identity and grants refresh without guest-selected names or targets", async () => {
  const binding = { installationId: "installation", digest: "digest", viewId: "skills" };
  const target = { type: "detail" as const, workspace: { path: "/captured" }, name: "verify" };
  const detail = {
    name: "verify",
    scope: "project" as const,
    path: "/captured/verify/SKILL.md",
    revision: "digest",
    instructions: "Read",
  };
  const plugins = {
    readView: vi.fn(async () => ({ html: "<!doctype html>" })),
    readSkill: vi.fn(async () => detail),
    readSkills: vi.fn(async () => ({ skills: [], diagnostics: [] })),
    readSkillLibrary: vi.fn(async () => ({ data: [] })),
    readSkillProposals: vi.fn(async () => ({ data: [] })),
  };
  const reads = createSkillViewReads(plugins, binding, target);
  binding.digest = "replacement";
  target.workspace.path = "/replacement";
  target.name = "other";
  const signal = new AbortController().signal;
  await expect(reads.load(signal)).resolves.toEqual({
    html: "<!doctype html>",
    initial: { type: "detail", data: detail },
  });
  await reads.read(undefined, signal);
  expect(plugins.readSkill).toHaveBeenLastCalledWith(
    {
      installationId: "installation",
      digest: "digest",
      viewId: "skills",
      workspace: { path: "/captured" },
      name: "verify",
    },
    signal,
  );
  await expect(reads.read("unbound", signal)).rejects.toThrow("no cursor");
  expect(plugins.readSkill).toHaveBeenCalledTimes(2);
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

it("captures the usage period and passes through the canonical report without pagination", async () => {
  const binding = { installationId: "installation", digest: "digest", viewId: "usage" };
  const period = { sinceDays: 7 };
  const report = { total: {}, sessions: 1, runs: 1, byProvider: [{ key: "provider", runs: 1 }] };
  const plugins = {
    readView: vi.fn(async () => ({ html: "<!doctype html>" })),
    readUsage: vi.fn(async () => report),
  };
  const reads = createUsageViewReads(plugins, binding, period);
  binding.digest = "successor";
  period.sinceDays = 30;
  const signal = new AbortController().signal;
  await expect(reads.load(signal)).resolves.toEqual({ html: "<!doctype html>", initial: report });
  await expect(reads.read(undefined, signal)).resolves.toBe(report);
  expect(plugins.readUsage).toHaveBeenLastCalledWith(
    { installationId: "installation", digest: "digest", viewId: "usage", sinceDays: 7 },
    signal,
  );
  await expect(reads.read("unbound", signal)).rejects.toThrow("no cursor");
  expect(plugins.readUsage).toHaveBeenCalledTimes(2);
});

it("captures Diff workspace, baseline and focus while refusing guest continuation", async () => {
  const binding = { installationId: "installation", digest: "digest", viewId: "diff" };
  const target = {
    workspace: { path: "/captured" },
    mode: "worktree" as const,
    focusPath: "src/a.ts",
  };
  const diff = { baseline: { type: "emptyTree" as const }, truncated: true };
  const plugins = {
    readView: vi.fn(async () => ({ html: "<!doctype html>" })),
    readDiff: vi.fn(async () => diff),
  };
  const reads = createDiffViewReads(plugins, binding, target);
  binding.digest = "replacement";
  target.workspace.path = "/replacement";
  target.focusPath = "other.ts";
  const signal = new AbortController().signal;
  await expect(reads.load(signal)).resolves.toEqual({
    html: "<!doctype html>",
    initial: { diff, focusPath: "src/a.ts" },
  });
  await reads.read(undefined, signal);
  expect(plugins.readDiff).toHaveBeenLastCalledWith(
    {
      installationId: "installation",
      digest: "digest",
      viewId: "diff",
      workspace: { path: "/captured" },
      mode: "worktree",
      format: "rows",
    },
    signal,
  );
  await expect(reads.read("unbound", signal)).rejects.toThrow("no cursor");
  expect(plugins.readDiff).toHaveBeenCalledTimes(2);
});
