import { afterEach, describe, expect, it, vi } from "vitest";
import { z } from "zod";
import { rehydrateOrDefault } from "./persistedStore";

const schema = z.object({ theme: z.string(), scale: z.number() });
const defaults = { theme: "light", scale: 1, setTheme: () => undefined };

afterEach(() => {
  vi.restoreAllMocks();
});

describe("rehydrateOrDefault", () => {
  it("takes a payload that parses", () => {
    const merge = rehydrateOrDefault("flame.test", schema);
    expect(merge({ theme: "dark", scale: 2 }, defaults)).toMatchObject({
      theme: "dark",
      scale: 2,
    });
  });

  it("keeps the live state's own members, which were never persisted", () => {
    const merge = rehydrateOrDefault("flame.test", schema);
    expect(merge({ theme: "dark", scale: 2 }, defaults).setTheme).toBe(defaults.setTheme);
  });

  it("boots from defaults with nothing stored, and says nothing about it", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const merge = rehydrateOrDefault("flame.test", schema);
    expect(merge(undefined, defaults)).toBe(defaults);
    expect(warn).not.toHaveBeenCalled();
  });

  it.each([
    ["a wrong member type", { theme: "dark", scale: "big" }],
    ["a missing member", { theme: "dark" }],
    ["a scalar where an object belongs", "dark"],
    ["null", null],
  ])("discards %s and boots from defaults", (_label, payload) => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const merge = rehydrateOrDefault("flame.test", schema);
    expect(merge(payload, defaults)).toBe(defaults);
    expect(warn).toHaveBeenCalledWith(
      "[flame.test] discarding corrupted payload:",
      expect.anything(),
    );
  });

  it("projects a durable shape onto a different live one", () => {
    const tuples = z.object({ entries: z.array(z.tuple([z.string(), z.number()])) });
    const merge = rehydrateOrDefault("flame.test", tuples, (data) => ({
      entries: new Map(data.entries),
    }));
    const restored = merge({ entries: [["a", 1]] }, { entries: new Map<string, number>() });
    expect(restored.entries.get("a")).toBe(1);
  });

  it("does not run the projection on a payload it rejected", () => {
    vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const project = vi.fn(() => ({ theme: "x", scale: 0 }));
    const merge = rehydrateOrDefault("flame.test", schema, project);
    merge({ nonsense: true }, defaults);
    expect(project).not.toHaveBeenCalled();
  });
});
