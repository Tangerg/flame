import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";
import { lookupExtensionByKey, TOOL_PREVIEW } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { toolPreviewPlugins } from "@/plugins/builtin";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import {
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
  RUNTIME_AGENT_SESSION_TAIL_EVENTS,
  VISUAL_AGENT_STATES,
} from "./agentSessionSnapshots";

describe("the visual agent fixtures", () => {
  it("hold only items the Runtime could have sent", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_SNAPSHOTS[state].items.flatMap((item) =>
        validateWire("Item", item).map(
          (violation) => `${state}/${item.id}: ${violation.path} ${violation.detail}`,
        ),
      ),
    );

    expect(violations).toEqual([]);
  });

  it("hold only stream frames the Runtime could have sent", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_TAIL_EVENTS[state].flatMap((frame) =>
        validateWire("StreamEvent", frame.event).map(
          (violation) => `${state}/${frame.index}: ${violation.path} ${violation.detail}`,
        ),
      ),
    );

    expect(violations).toEqual([]);
  });

  it("hold only runs the Runtime could have sent", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_SNAPSHOTS[state].runs.flatMap((run) =>
        validateWire("RunRef", run).map(
          (violation) => `${state}/${run.id}: ${violation.path} ${violation.detail}`,
        ),
      ),
    );

    expect(violations).toEqual([]);
  });

  it("calls every tool preview the product registers", async () => {
    await loadPluginsForTest(...toolPreviewPlugins);

    const called = new Set(
      VISUAL_AGENT_STATES.flatMap((state) =>
        RUNTIME_AGENT_SESSION_SNAPSHOTS[state].items.flatMap((item) =>
          item.type === "toolCall" ? [item.tool.name] : [],
        ),
      ),
    );
    const uncalled = Object.keys(TOOL_ICON_BY_NAME).filter(
      (name) => lookupExtensionByKey(TOOL_PREVIEW, name) !== undefined && !called.has(name),
    );

    expect(called.size, "the sweep has to be looking at real calls").toBeGreaterThan(10);
    expect(uncalled, "tool previews no fixture ever renders").toEqual([]);
  });

  const DECLARED_RESULT_SHAPE: Record<
    string,
    "SearchResult" | "PatchResult" | "CommandResult" | "WebSearchResult"
  > = {
    glob: "SearchResult",
    grep: "SearchResult",
    apply_patch: "PatchResult",
    shell: "CommandResult",
    web_search: "WebSearchResult",
  };

  it("answer each declared tool with the result shape the Runtime sends", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_SNAPSHOTS[state].items.flatMap((item) => {
        if (item.type !== "toolCall" || item.status !== "completed") return [];
        const shape = DECLARED_RESULT_SHAPE[item.tool.name];
        if (shape === undefined) return [];
        const result = item.tool.result;
        if (result === undefined)
          return [`${state}/${item.id}: settled ${item.tool.name} with no result`];
        return validateWire(shape, typeof result === "string" ? JSON.parse(result) : result).map(
          (violation) => `${state}/${item.id}: ${violation.path} ${violation.detail}`,
        );
      }),
    );

    expect(violations).toEqual([]);
  });
});
