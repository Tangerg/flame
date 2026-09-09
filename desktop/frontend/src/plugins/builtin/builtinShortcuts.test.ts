import { afterEach, describe, expect, it } from "vitest";
import { builtinPlugins } from "./index";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { dispatchBinding } from "@/lib/combo";

/**
 * Two registrations on one key is a command nobody can reach.
 *
 * `keymapOf` folds commands and shortcuts into a Map keyed by the dispatch form, so the last
 * registration silently replaces the earlier one — a documented rule, and the right one for
 * letting a plugin override a default. What it cannot do is tell the difference between an
 * override someone meant and two plugins that happen to want the same chord. The keymap that
 * results looks correct either way, and the shortcuts pane lists the winner.
 *
 * So the collision is checked before the fold, over the plugin set the product actually
 * ships, and any deliberate override is named here rather than resolved by load order.
 */
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

    // The product registers keys at all — a kernel that loaded nothing would pass silently.
    expect(bindings.size).toBeGreaterThan(3);

    const collisions = [...bindings]
      .filter(([key, owners]) => owners.length > 1 && !DELIBERATE_OVERRIDES.has(key))
      .map(([key, owners]) => `${key} <- ${owners.join(", ")}`);
    expect(collisions).toEqual([]);
  });
});
