import { describe, expect, it, vi } from "vitest";
import {
  Backend,
  Method,
  parseJSON,
  type CallContext,
  type MethodDescriptor,
} from "@qimu/node";
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

  it("registers the Flame receipt on the original generic Backend with the shared identity decoder", () => {
    const backend = new Backend({ name: "flame-tool-fixture", version: "1" });
    const registration = vi.spyOn(backend, "implement");
    const handle = vi.fn(() => ({ found: false }));
    toolReceipts(backend, handle);
    expect(registration).toHaveBeenCalledOnce();
    const [method, decode, implementation] = registration.mock.calls[0]!;
    expect(method.identity).toBe(
      new Method(parseJSON(wire.toolReceipt) as MethodDescriptor).identity,
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
