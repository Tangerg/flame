import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { missingBrowserFeatures, startBrowserApplication } from "./browserSupport";

beforeEach(() => {
  vi.spyOn(HTMLScriptElement, "supports").mockReturnValue(true);
});
afterEach(() => vi.restoreAllMocks());

function withoutFeature<T>(owner: object, key: string, inspect: () => T): T {
  const descriptor = Object.getOwnPropertyDescriptor(owner, key);
  Object.defineProperty(owner, key, { configurable: true, value: undefined });
  try {
    return inspect();
  } finally {
    if (descriptor) Object.defineProperty(owner, key, descriptor);
    else Reflect.deleteProperty(owner, key);
  }
}

describe("browser startup admission", () => {
  it.each([
    ["Promise.withResolvers", Promise, "withResolvers"],
    ["AbortSignal.any", AbortSignal, "any"],
    ["Map.groupBy", Map, "groupBy"],
    ["Array.prototype.toSorted", Array.prototype, "toSorted"],
    ["Array.prototype.toReversed", Array.prototype, "toReversed"],
    ['HTMLScriptElement.supports("importmap")', HTMLScriptElement, "supports"],
  ] as const)(
    "diagnoses missing %s before evaluating the application",
    async (label, owner, key) => {
      const load = vi.fn(async () => undefined);
      const starting = withoutFeature(owner, key, () => startBrowserApplication(load));
      expect(await starting).toEqual({ status: "unsupported", missing: [label] });
      expect(load).not.toHaveBeenCalled();
    },
  );

  it("requires actual import-map support rather than the supports method alone", async () => {
    vi.mocked(HTMLScriptElement.supports).mockReturnValue(false);
    const load = vi.fn(async () => undefined);
    expect(await startBrowserApplication(load)).toEqual({
      status: "unsupported",
      missing: ['HTMLScriptElement.supports("importmap")'],
    });
    expect(load).not.toHaveBeenCalled();
  });

  it("loads an admitted application once and waits for its module evaluation", async () => {
    const evaluation = Promise.withResolvers<void>();
    const load = vi.fn(() => evaluation.promise);
    const starting = startBrowserApplication(load);
    let settled = false;
    void starting.then(() => {
      settled = true;
    });
    await Promise.resolve();
    expect(missingBrowserFeatures()).toEqual([]);
    expect(load).toHaveBeenCalledOnce();
    expect(settled).toBe(false);
    evaluation.resolve();
    expect(await starting).toEqual({ status: "started" });
  });

  it.each(["throw", "reject"] as const)(
    "preserves the original module %s failure",
    async (kind) => {
      const cause = new Error("dependency could not be evaluated");
      const load = vi.fn(() => {
        if (kind === "throw") throw cause;
        return Promise.reject(cause);
      });
      await expect(startBrowserApplication(load)).rejects.toBe(cause);
      expect(load).toHaveBeenCalledOnce();
    },
  );
});
