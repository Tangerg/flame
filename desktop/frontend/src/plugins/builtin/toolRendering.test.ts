import { describe, expect, it } from "vitest";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import { lookupExtensionByKey, TOOL_PREVIEW } from "@/plugins/sdk";
import { toolPreviewPlugins } from "./index";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

// Tools whose RESULT is the answer render as prose, so one component serves all three.
// Named rather than counted: an unlisted preview collapsing onto another still fails below,
// and this list is the only place the sharing is asserted to be deliberate. The previous
// shape demanded a distinct component per tool, which is why two of these existed as
// functions whose whole body was `<PlanModeResult {...props} />`.
const PROSE_RESULT_TOOLS = new Set(["enter_plan_mode", "exit_plan_mode", "report_goal_outcome"]);

describe("built-in tool rendering composition", () => {
  it("installs a preview for every known tool, shared only where the render is", async () => {
    // One transaction: a preview plugin that fails to install takes the whole
    // boot down, which is the assertion.
    await loadPluginsForTest(...toolPreviewPlugins);

    const previews = Object.keys(TOOL_ICON_BY_NAME).map((name) => {
      const component = lookupExtensionByKey(TOOL_PREVIEW, name);
      expect(component, `${name} preview`).toBeDefined();
      return { name, component };
    });

    const prose = previews.filter(({ name }) => PROSE_RESULT_TOOLS.has(name));
    expect(prose).toHaveLength(PROSE_RESULT_TOOLS.size);
    expect(new Set(prose.map(({ component }) => component)).size).toBe(1);

    const own = previews.filter(({ name }) => !PROSE_RESULT_TOOLS.has(name));
    expect(new Set(own.map(({ component }) => component)).size).toBe(own.length);
  });
});
