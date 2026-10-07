import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RpcError } from "@flame/runtime-contract/client";
import { t } from "@/lib/i18n";
import { useAsyncFeedback } from "./useAsyncFeedback";

describe("useAsyncFeedback", () => {
  it("run -> ok reflects success", async () => {
    const { result } = renderHook(() => useAsyncFeedback());
    await act(async () => {
      await result.current.run(async () => ({ ok: true }), "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "ok" });
  });

  it("run -> not-ok surfaces the result error, falling back when absent", async () => {
    const { result } = renderHook(() => useAsyncFeedback());
    await act(async () => {
      await result.current.run(async () => ({ ok: false, error: "bad key" }), "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "error", reason: "bad key" });
    await act(async () => {
      await result.current.run(async () => ({ ok: false }), "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "error", reason: "fallback" });
  });

  it("run -> thrown Error surfaces its message", async () => {
    const { result } = renderHook(() => useAsyncFeedback());
    await act(async () => {
      await result.current.run(async () => {
        throw new Error("network down");
      }, "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "error", reason: "network down" });
  });

  it("run -> thrown Runtime problem surfaces its product copy, not the problem symbol", async () => {
    const { result } = renderHook(() => useAsyncFeedback());
    await act(async () => {
      await result.current.run(async () => {
        throw new RpcError({
          code: -32000,
          message: "mcp_server_already_exists",
          data: { type: "mcp_server_already_exists" },
        });
      }, "fallback");
    });
    expect(result.current.feedback).toEqual({
      state: "error",
      reason: t("rpcError.mcp_server_already_exists"),
    });
    expect(t("rpcError.mcp_server_already_exists")).not.toBe("mcp_server_already_exists");
  });

  it("returns an ignored lifecycle settlement to idle", async () => {
    const { result } = renderHook(() => useAsyncFeedback());
    const retired = new Error("generation retired");
    await act(async () => {
      await result.current.run(
        async () => {
          throw retired;
        },
        "fallback",
        (error) => error === retired,
      );
    });
    expect(result.current.feedback).toEqual({ state: "idle" });
  });

  it("drops a superseded run's result", async () => {
    const { result } = renderHook(() => useAsyncFeedback());
    const slow = Promise.withResolvers<{ ok: boolean; error?: string }>();
    let firstDone!: Promise<void>;
    act(() => {
      firstDone = result.current.run(() => slow.promise, "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "busy" });
    await act(async () => {
      await result.current.run(async () => ({ ok: true }), "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "ok" });
    await act(async () => {
      slow.resolve({ ok: false, error: "stale" });
      await firstDone;
    });
    expect(result.current.feedback).toEqual({ state: "ok" });
  });

  it("reset invalidates an in-flight run and returns to idle", async () => {
    const { result } = renderHook(() => useAsyncFeedback());
    const slow = Promise.withResolvers<{ ok: boolean; error?: string }>();
    let done!: Promise<void>;
    act(() => {
      done = result.current.run(() => slow.promise, "fallback");
    });
    act(() => result.current.reset());
    expect(result.current.feedback).toEqual({ state: "idle" });
    await act(async () => {
      slow.resolve({ ok: true });
      await done;
    });
    expect(result.current.feedback).toEqual({ state: "idle" });
  });

  it("retires completed and in-flight feedback when its material generation changes", async () => {
    const slow = Promise.withResolvers<{ ok: boolean; error?: string }>();
    const { result, rerender } = renderHook(({ generation }) => useAsyncFeedback(generation), {
      initialProps: { generation: 1 },
    });

    await act(async () => {
      await result.current.run(async () => ({ ok: true }), "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "ok" });

    rerender({ generation: 2 });
    expect(result.current.feedback).toEqual({ state: "idle" });

    let done!: Promise<void>;
    act(() => {
      done = result.current.run(() => slow.promise, "fallback");
    });
    expect(result.current.feedback).toEqual({ state: "busy" });

    rerender({ generation: 3 });
    expect(result.current.feedback).toEqual({ state: "idle" });
    await act(async () => {
      slow.resolve({ ok: false, error: "retired result" });
      await done;
    });
    expect(result.current.feedback).toEqual({ state: "idle" });
  });
});
