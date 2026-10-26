import { expect, it, vi } from "vitest";
import { Configuration } from "./config";

it("keeps each host's configuration private and retains values by copy", () => {
  const first = new Configuration();
  const second = new Configuration();
  const input = { providers: ["original"] };
  first.set("settings", input);
  input.providers.push("caller mutation");
  const read = first.get("settings") as { providers: string[] };
  read.providers.push("reader mutation");
  expect(first.get("settings")).toEqual({ providers: ["original"] });
  expect(second.has("settings")).toBe(false);
  first.set("unset", null);
  expect(first.get("unset", "fallback")).toBeNull();
});

it("publishes private snapshots and retires only the subscription being disposed", () => {
  const config = new Configuration();
  const changed = vi.fn();
  const observing = config.onChange("settings", changed);
  config.onChange("settings", (value) => {
    (value as string[]).push("listener mutation");
  });
  config.set("settings", ["retained"]);
  expect(changed).toHaveBeenCalledExactlyOnceWith(["retained"]);
  expect(config.get("settings")).toEqual(["retained"]);
  observing.dispose();
  observing.dispose();
  config.set("settings", ["successor"]);
  expect(changed).toHaveBeenCalledOnce();
});
