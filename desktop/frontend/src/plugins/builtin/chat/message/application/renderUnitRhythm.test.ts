import { describe, expect, it } from "vitest";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import { unitSeam, unitVoice } from "./renderUnitRhythm";

const block = (kind: ContentBlock["kind"]): MessageRenderUnit => ({
  kind: "block",
  block: { kind, status: "complete" } as ContentBlock,
  index: 0,
  superseded: false,
});

describe("unitVoice", () => {
  it("reads a fold and a tool group as process", () => {
    expect(unitVoice({ kind: "wave", units: [] })).toBe("process");
    expect(unitVoice({ kind: "toolGroup", tools: [], superseded: false })).toBe("process");
    expect(unitVoice(block("tool"))).toBe("process");
    expect(unitVoice(block("reasoning"))).toBe("process");
  });

  it("reads text as prose and everything asking for the reader as a panel", () => {
    expect(unitVoice(block("text"))).toBe("prose");
    for (const kind of ["approval", "question", "compaction", "image"] as const) {
      expect(unitVoice(block(kind))).toBe("panel");
    }
  });
});

describe("unitSeam", () => {
  it("gives the first unit no seam — the turn's own gap already placed it", () => {
    expect(unitSeam(undefined, block("text"))).toBeUndefined();
  });

  // The ratio is the whole feature: work that belongs together stays close, and a change of
  // voice opens up. Both distances are also both references' measured answer — 6px between
  // activity rows, 20px where the voice changes — and those pixels are `seamStep`'s now. What
  // this module decides, and what this asserts, is WHICH seam the pair makes.
  it("keeps consecutive process rows tight and opens up at a change of voice", () => {
    expect(unitSeam(block("tool"), block("reasoning"))).toBe("tight");
    expect(unitSeam(block("tool"), block("text"))).toBe("wide");
  });

  it("is symmetric across the prose seam", () => {
    expect(unitSeam(block("text"), block("tool"))).toBe(unitSeam(block("tool"), block("text")));
  });
});
