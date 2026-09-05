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

// The goldens are a claim about what the product does with what the Runtime sends. A fixture
// the Runtime could not have sent makes them a claim about nothing — and the drift is silent,
// because the generated TS type states each field optional and the requirement is
// CONDITIONAL: a settled tool call must carry `finishedAt` and `durationMillis`, which only
// `validateWire` knows. Seventeen items had gone that way, across four states, so every tool
// row in those frames was photographed without the duration production always supplies.
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

  // The live states are built from a tail of stream frames rather than from history, so a
  // snapshot-only check leaves `running`, `answer-opening` and `steer` — the three frames
  // that photograph work in flight — unguarded. Two settled tool calls in that tail had gone
  // the same way as the seventeen in the snapshots.
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

  // A preview nothing calls is a panel nothing has photographed, WCAG-audited or looked at.
  // Twenty-one tools carry one and six were exercised when this was written; the other fifteen
  // held a raw XML dump, a wire timestamp, a date truncated to "2026-0…" and a row of escape
  // codes between them. Closing that took four rounds; this keeps it closed for the next
  // preview somebody adds.
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
});
