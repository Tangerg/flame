import { describe, expect, it, vi } from "vitest";
import {
  Backend,
  declareMethod,
  parseJSON,
  type CallContext,
  type MethodDescriptor,
} from "@flame/backend-sdk/mcp";
import { toolCommandId, toolReceipts } from "./tools";
import wire from "./protocol.generated.json";

function context(id?: string): CallContext {
  return {
    invocation:
      id === undefined
        ? undefined
        : {
            id,
            origin: "human",
            target: { scope: "session", reference: "session" },
          },
  } as CallContext;
}

describe("Flame Tool capabilities", () => {
  it("derives the command from the original invocation and refuses absent or oversized identity", () => {
    expect(toolCommandId(context("original"))).toBe(
      wire.constants.toolCommandPrefix + "original",
    );
    expect(() => toolCommandId(context())).toThrow("invocation is required");
    expect(() => toolCommandId(context("x".repeat(300)))).toThrow(
      "exceeds its bound",
    );
  });

  it("uses the published receipt result shape before replying to the host", () => {
    const method = declareMethod(
      parseJSON(wire.toolReceipt) as MethodDescriptor,
    );
    for (const response of [
      {},
      { result: { output: { content: [{ kind: "text", text: "done" }] } } },
      { result: { output: {}, isError: true } },
    ]) {
      expect(() => method.validateOutput(response)).not.toThrow();
    }
    for (const response of [
      { found: true },
      { found: false, isError: true },
      { result: {} },
      { result: { output: "null" } },
    ]) {
      expect(() => method.validateOutput(response)).toThrow();
    }
  });

  it("registers the Flame receipt on the original generic Backend with the shared identity decoder", () => {
    const backend = new Backend({ name: "flame-tool-fixture", version: "1" });
    const registration = vi.spyOn(backend, "implement");
    const handle = vi.fn(() => ({}));
    toolReceipts(backend, handle);
    expect(registration).toHaveBeenCalledOnce();
    const [method, decode, implementation] = registration.mock.calls[0]!;
    expect(method.identity).toBe(
      declareMethod(parseJSON(wire.toolReceipt) as MethodDescriptor).identity,
    );
    expect(implementation).toBe(handle);
    expect(() =>
      decode(
        { id: "original", method: "tool:read", inputDigest: "invalid" },
        "{}",
      ),
    ).toThrow("receipt identity");
    expect(handle).not.toHaveBeenCalled();
    expect(() => toolReceipts(backend, handle)).toThrow();
  });
});
