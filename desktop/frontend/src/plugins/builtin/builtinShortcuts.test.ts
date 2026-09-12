import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { builtinPlugins } from "./index";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { dispatchBinding } from "@/lib/combo";

beforeEach(() => {
  vi.stubGlobal("fetch", () => Promise.reject(new Error("offline in tests")));
  vi.stubGlobal(
    "EventSource",
    class {
      close() {}
      addEventListener() {}
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const DELIBERATE_OVERRIDES = new Map<string, string>();

afterEach(async () => {
  await resetKernelForTest();
});

describe("built-in shortcuts", () => {
  it("bind one key each", async () => {
    await loadPluginsForTest(...builtinPlugins);

    const bindings = new Map<string, string[]>();
    for (const command of lookupExtensionPoint(COMMAND)) {
      if (command.combo === undefined) continue;
      const key = dispatchBinding(command.combo);
      bindings.set(key, [...(bindings.get(key) ?? []), `command ${command.id}`]);
    }
    for (const shortcut of lookupExtensionPoint(SHORTCUT)) {
      const key = dispatchBinding(shortcut.key);
      bindings.set(key, [...(bindings.get(key) ?? []), `shortcut ${shortcut.key}`]);
    }

    expect(bindings.size).toBeGreaterThan(3);

    const collisions = [...bindings]
      .filter(([key, owners]) => owners.length > 1 && !DELIBERATE_OVERRIDES.has(key))
      .map(([key, owners]) => `${key} <- ${owners.join(", ")}`);
    expect(collisions).toEqual([]);
  });
});
