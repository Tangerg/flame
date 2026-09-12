import { describe, expect, it } from "vitest";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import { lookupExtensionByKey, TOOL_PREVIEW } from "@/plugins/sdk";
import { toolPreviewPlugins } from "./index";
import { GOAL_STANDING_TOOLS } from "./chat/goal";
import { PLAN_STANDING_TOOLS } from "./chat/plan-progress";
import { SCHEDULE_STANDING_TOOLS } from "./settings/schedules";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const STANDING = new Set<string>([
  ...PLAN_STANDING_TOOLS,
  ...GOAL_STANDING_TOOLS,
  ...SCHEDULE_STANDING_TOOLS,
]);

describe("built-in tool rendering composition", () => {
  it("previews every tool the transcript draws, and none it does not", async () => {
    await loadPluginsForTest(...toolPreviewPlugins);

    const names = Object.keys(TOOL_ICON_BY_NAME);
    const preview = (name: string) => lookupExtensionByKey(TOOL_PREVIEW, name);

    expect(
      names.filter((name) => STANDING.has(name) && preview(name) !== undefined),
      "tools answered by a standing surface must not preview",
    ).toEqual([]);
    expect(
      names.filter((name) => !STANDING.has(name) && preview(name) === undefined),
      "tools the transcript draws must preview",
    ).toEqual([]);

    const drawn = names
      .filter((name) => !STANDING.has(name))
      .map((name) => ({ name, component: preview(name) }));

    expect(STANDING.size).toBeGreaterThan(0);
    expect(drawn.length).toBeGreaterThan(STANDING.size);

    expect(new Set(drawn.map(({ component }) => component)).size).toBe(drawn.length);
  });
});
