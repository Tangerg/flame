import { expect, it, vi } from "vitest";
import { createScheduleViewReads } from "./pluginViewReads";

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
