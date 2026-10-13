import { expect, it, vi } from "vitest";
import { createScheduleViewReads, createSkillViewReads } from "./pluginViewReads";

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
