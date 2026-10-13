import { expect, it, vi } from "vitest";
import {
  createMemoryViewReads,
  createScheduleViewReads,
  createSkillViewReads,
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
  await expect(reads.initial(signal)).resolves.toEqual({ type: "detail", data: detail });
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
it("keeps resource admission and initial data as separately owned operations", async () => {
  const cause = new Error("package missing");
  const plugins = {
    readView: vi.fn(async () => {
      throw cause;
    }),
    readMemory: vi.fn(async () => ({ data: [] })),
  };
  const reads = createMemoryViewReads(
    plugins,
    { installationId: "installation", digest: "digest", viewId: "memory" },
    { scope: "user" },
  );
  await expect(reads.resource(new AbortController().signal)).rejects.toBe(cause);
  expect(plugins.readMemory).not.toHaveBeenCalled();
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
  await reads.initial(signal);
  await reads.read("next", signal);
  expect(plugins.readSchedules).toHaveBeenLastCalledWith(
    { installationId: "installation", digest: "digest", viewId: "schedules", cursor: "next" },
    signal,
  );
});
