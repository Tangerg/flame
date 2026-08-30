import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";
import { en } from "./i18n/locales/en";
import { MAPPED_TYPES, describeErrorType, describeProblem, isUnsupportedMethod } from "./rpcErrors";

// This table claims to be one-to-one with the protocol, and nothing checked it. It carried
// three symbols — `file_too_large`, `is_a_directory`, `no_language_server` — that the wire
// has never defined, so three locales' worth of copy could never be shown; and it was
// missing the four symbols a run most often ends on, so the banner called "you declined
// this" an unknown error. Both directions are guarded here.

// A bare `{ type }` is an INCOMPLETE variant for the symbols that carry required fields, so
// emptiness is the wrong test. The validator rejects an unknown symbol at `ProblemData.type`
// and a real-but-incomplete one only at the field it is missing.
function isWireProblemType(type: string): boolean {
  return !validateWire("ProblemData", { type }).some(
    (violation) => violation.path === "ProblemData.type",
  );
}

describe("the protocol error copy table", () => {
  it("names only symbols the wire can actually send", () => {
    const phantom = MAPPED_TYPES.filter((type) => !isWireProblemType(type));
    expect(phantom).toEqual([]);
  });

  it("is not vacuous — the check does reject a symbol the wire lacks", () => {
    expect(isWireProblemType("file_too_large")).toBe(false);
    expect(isWireProblemType("session_busy")).toBe(true);
  });

  it("has copy for every symbol it claims to map", () => {
    const keys = new Set(Object.keys(en));
    const uncovered = MAPPED_TYPES.filter((type) => !keys.has(`rpcError.${type}`));
    expect(uncovered).toEqual([]);
  });

  it("keeps no copy for a symbol it no longer maps", () => {
    const mapped = new Set(MAPPED_TYPES.map((type) => `rpcError.${type}`));
    const orphaned = Object.keys(en).filter(
      (key) => key.startsWith("rpcError.") && !mapped.has(key),
    );
    expect(orphaned).toEqual([]);
  });

  // How a run ends when it does not complete. These are the person's own decision or an
  // ordinary tool outcome, never a protocol fault, so they must always read as words.
  it.each(["denied_by_user", "tool_failed", "tool_canceled", "child_run_canceled"])(
    "explains %s rather than leaving the banner to say 'unknown'",
    (type) => {
      expect(isWireProblemType(type)).toBe(true);
      const copy = describeErrorType(type);
      expect(copy).toBeDefined();
      expect(copy).not.toBe(`rpcError.${type}`);
    },
  );

  it("answers nothing for a symbol it does not map, so callers supply their own fallback", () => {
    expect(describeErrorType(undefined)).toBeUndefined();
    expect(describeErrorType("replay_unavailable")).toBeUndefined();
    expect(describeErrorType("not_a_symbol")).toBeUndefined();
  });

  it("prefers a problem's own detail over the table, and the symbol over nothing", () => {
    expect(describeProblem({ type: "session_busy", detail: "this occurrence" })).toBe(
      "this occurrence",
    );
    expect(describeProblem({ type: "session_busy" })).toBe(describeErrorType("session_busy"));
    expect(describeProblem({ type: "replay_unavailable" })).toBe("replay_unavailable");
    expect(describeProblem(undefined)).toBeUndefined();
  });

  it("reads a non-error as no unsupported method", () => {
    for (const value of [undefined, null, "", 0, new Error("plain")]) {
      expect(isUnsupportedMethod(value)).toBe(false);
    }
  });
});
