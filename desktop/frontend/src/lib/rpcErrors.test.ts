import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";
import { RpcError, RpcTransportError, RPC_METHOD_NOT_FOUND } from "@/rpc";
import { en } from "./i18n/locales/en";
import {
  MAPPED_TYPES,
  describeErrorType,
  describeProblem,
  isUnsupportedMethod,
  rpcErrorText,
} from "./rpcErrors";

function isWireProblemType(type: string): boolean {
  return !validateWire("ProblemData", { type }).some(
    (violation) => violation.path === "ProblemData.type",
  );
}

describe("the protocol error copy table", () => {
  it("explains an unresolved receipt without treating it as a rejected command", () => {
    const error = new RpcError({
      message: "receipt pending",
      data: { type: "idempotency_in_progress", retryAfterSeconds: 1 },
    });
    expect(rpcErrorText(error)).toBe(en["rpcError.idempotency_in_progress"]);
  });
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

  it("does not confuse an HTTP routing failure with an unsupported RPC method", () => {
    expect(isUnsupportedMethod(new RpcTransportError("not found", 404))).toBe(false);
    expect(
      isUnsupportedMethod(
        new RpcError({
          code: RPC_METHOD_NOT_FOUND,
          message: "method not found",
          data: { type: "method_not_found" },
        }),
      ),
    ).toBe(true);
  });

  it("reads a non-error as no unsupported method", () => {
    for (const value of [undefined, null, "", 0, new Error("plain")]) {
      expect(isUnsupportedMethod(value)).toBe(false);
    }
  });
});
