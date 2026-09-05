import { describe, expect, it } from "vitest";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import { lookupExtensionByKey, TOOL_PREVIEW } from "@/plugins/sdk";
import { toolPreviewPlugins } from "./index";
import { GOAL_STANDING_TOOLS } from "./chat/goal";
import { PLAN_STANDING_TOOLS } from "./chat/plan-progress";
import { SCHEDULE_STANDING_TOOLS } from "./settings/schedules";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

// Two halves of one rule. A tool answered by a standing surface — the plan bar, the goal bar,
// the Schedules pane — has its transcript row DROPPED, so a preview for it is a component
// nobody can reach and a claim, to the next reader, that the transcript shows one. Every other
// tool must have a preview, or its row expands onto the generic inspector.
//
// The names come from the three surface owners rather than a list here: which tools a surface
// answers for is theirs to say, and a copy in this file would be a second answer that drifts.
const STANDING = new Set<string>([
  ...PLAN_STANDING_TOOLS,
  ...GOAL_STANDING_TOOLS,
  ...SCHEDULE_STANDING_TOOLS,
]);

describe("built-in tool rendering composition", () => {
  it("previews every tool the transcript draws, and none it does not", async () => {
    // One transaction: a preview plugin that fails to install takes the whole
    // boot down, which is the assertion.
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

    // Not vacuous on either side: a rule with nothing on one of them is not being tested.
    expect(STANDING.size).toBeGreaterThan(0);
    expect(drawn.length).toBeGreaterThan(STANDING.size);

    // A drawn tool gets its own component. Sharing was deliberate once — three tools whose
    // result IS the answer rendered as prose — and all three name a standing surface now.
    expect(new Set(drawn.map(({ component }) => component)).size).toBe(drawn.length);
  });
});
