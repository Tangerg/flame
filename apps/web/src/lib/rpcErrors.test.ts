import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-client/validation";
import { PROBLEM_CODES, PROBLEM_RECOVERY, RUN_PROBLEM_RECOVERY } from "@flame/runtime-client/wire";
import { OperationFailure, RpcError, RpcTransportError } from "@flame/runtime-client";
import { failureText as componentFailureText } from "@flame/component-sdk";
import { en } from "@/lib/i18n/locales/en";
import {
  MAPPED_TYPES,
  describeErrorType,
  isUnsupportedMethod,
  rpcErrorText,
} from "@/lib/rpcErrors";
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

  it("explains every problem the Runtime hands to the person", () => {
    const promptUser = Object.entries({ ...PROBLEM_RECOVERY, ...RUN_PROBLEM_RECOVERY })
      .filter(([, recovery]) => recovery === "promptUser")
      .map(([type]) => type);
    expect(promptUser.length).toBeGreaterThan(0);
    expect(promptUser.filter((type) => !MAPPED_TYPES.includes(type))).toEqual([]);
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

  it.each([
    "denied_by_user",
    "tool_failed",
    "tool_canceled",
    "child_run_canceled",
    "checkpoint_conflict",
    "prompt_source_too_large",
  ])("explains %s rather than leaving the banner to say 'unknown'", (type) => {
    expect(isWireProblemType(type)).toBe(true);
    const copy = describeErrorType(type);
    expect(copy).toBeDefined();
    expect(copy).not.toBe(`rpcError.${type}`);
  });

  it("answers nothing for a symbol it does not map, so callers supply their own fallback", () => {
    expect(describeErrorType(undefined)).toBeUndefined();
    expect(describeErrorType("replay_unavailable")).toBeUndefined();
    expect(describeErrorType("not_a_symbol")).toBeUndefined();
  });

  it("does not confuse an HTTP routing failure with an unsupported RPC method", () => {
    expect(isUnsupportedMethod(new RpcTransportError("not found", 404))).toBe(false);
    expect(
      isUnsupportedMethod(
        new RpcError({
          code: PROBLEM_CODES.method_not_found,
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

describe("the portable component failure text", () => {
  const internalError = (detail?: string) =>
    new RpcError({
      code: -32603,
      message: "internal_error",
      data: detail === undefined ? { type: "internal_error" } : { type: "internal_error", detail },
    });

  it("names a typed Runtime problem by its detail rather than its machine symbol", () => {
    expect(componentFailureText(internalError("schedules backend offline"), "fallback")).toBe(
      "schedules backend offline",
    );
  });

  it("keeps the RPC message when the problem carries no readable detail", () => {
    expect(componentFailureText(internalError(), "fallback")).toBe("internal_error");
    expect(componentFailureText(internalError(""), "fallback")).toBe("internal_error");
  });

  it("keeps the message of every other error", () => {
    const shaped = Object.assign(new Error("plain failure"), {
      data: { type: "internal_error", detail: "shaped detail" },
    });
    for (const [error, message] of [
      [new Error("plain failure"), "plain failure"],
      [new RpcTransportError("gateway unavailable", 502), "gateway unavailable"],
      [new OperationFailure("schedule_conflict", "schedule changed"), "schedule changed"],
      [shaped, "plain failure"],
    ] as const) {
      expect(componentFailureText(error, "fallback")).toBe(message);
    }
  });

  it("uses the caller's fallback for a thrown non-error, including an RPC lookalike", () => {
    const lookalike = {
      name: "RpcError",
      code: -32603,
      message: "internal_error",
      data: { type: "internal_error", detail: "untrusted detail" },
    };
    for (const value of [undefined, "offline", lookalike]) {
      expect(componentFailureText(value, "Schedules are unavailable.")).toBe(
        "Schedules are unavailable.",
      );
    }
  });
});
