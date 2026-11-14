import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useHighlightedCode } from "./useCodeHighlight";

const mocks = vi.hoisted(() => ({
  requestHighlight: vi.fn(),
  reportHighlightFailure: vi.fn(),
  getCachedHighlight: vi.fn(),
  setCachedHighlight: vi.fn(),
  measureShikiHighlight: vi.fn(),
  scheme: "light" as "light" | "dark",
}));
vi.mock("./shiki", () => mocks);
vi.mock("./shikiCache", () => mocks);
vi.mock("@/lib/metrics", () => mocks);
vi.mock("@/lib/appearance", () => ({ useScheme: () => mocks.scheme }));

const jobs: Array<{
  input: { code: string; lang: string; theme: string };
  signal: AbortSignal;
  complete: (html: string) => void;
  reject: (error: unknown) => void;
}> = [];
beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  mocks.scheme = "light";
  mocks.getCachedHighlight.mockReturnValue(undefined);
  jobs.length = 0;
  mocks.requestHighlight.mockImplementation((_owner, input, signal) => {
    const task = Promise.withResolvers<{ html: string; lang: string; duration: number }>();
    jobs.push({
      input,
      signal,
      complete: (html) => task.resolve({ html, lang: input.lang, duration: 12 }),
      reject: task.reject,
    });
    return task.promise;
  });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

it("shows the literal first and accepts a current worker result into the existing cache", async () => {
  const { result } = renderHook(() => useHighlightedCode("const n = 1", "typescript"));
  expect(result.current).toBeNull();
  expect(jobs).toHaveLength(1);
  await act(async () => jobs[0]!.complete("<pre><code>colored</code></pre>"));
  expect(result.current).toBe("<pre><code>colored</code></pre>");
  expect(mocks.setCachedHighlight).toHaveBeenCalledExactlyOnceWith(
    "typescript",
    "github-light-high-contrast",
    "const n = 1",
    result.current,
  );
  expect(mocks.measureShikiHighlight).toHaveBeenCalledExactlyOnceWith(12, "typescript");
});

it("retires a changed source immediately and keeps its latest source debounced", async () => {
  const { result, rerender } = renderHook(({ code }) => useHighlightedCode(code, "go"), {
    initialProps: { code: "old" },
  });
  rerender({ code: "latest" });
  expect(result.current).toBeNull();
  expect(jobs[0]!.signal.aborted).toBe(true);
  await act(async () => jobs[0]!.complete("outdated"));
  expect(mocks.setCachedHighlight).not.toHaveBeenCalled();
  expect(jobs).toHaveLength(1);
  await act(() => vi.advanceTimersByTimeAsync(120));
  expect(jobs).toHaveLength(2);
  expect(jobs[1]!.input.code).toBe("latest");
  await act(async () => jobs[1]!.complete("current"));
  expect(result.current).toBe("current");
});

it("does not accept an old theme, language or retired mount even when its source returns", async () => {
  const { result, rerender, unmount } = renderHook(({ lang }) => useHighlightedCode("same", lang), {
    initialProps: { lang: "go" },
  });
  mocks.scheme = "dark";
  rerender({ lang: "rust" });
  mocks.scheme = "light";
  rerender({ lang: "go" });
  expect(jobs).toHaveLength(3);
  await act(async () => {
    jobs[0]!.complete("old identical key");
    jobs[1]!.complete("other theme");
  });
  expect(result.current).toBeNull();
  expect(mocks.setCachedHighlight).not.toHaveBeenCalled();
  unmount();
  await act(async () => jobs[2]!.complete("retired mount"));
  expect(mocks.setCachedHighlight).not.toHaveBeenCalled();
});

it("keeps unavailable highlighting explicit while leaving literal code visible", async () => {
  const failure = new Error("worker unavailable");
  const { result } = renderHook(() => useHighlightedCode("plain", "go"));
  await act(async () => jobs[0]!.reject(failure));
  expect(result.current).toBeNull();
  expect(mocks.reportHighlightFailure).toHaveBeenCalledExactlyOnceWith("grammar go", failure);
  expect(mocks.setCachedHighlight).not.toHaveBeenCalled();
});

it("uses an exact existing cache hit without creating a worker request", () => {
  mocks.getCachedHighlight.mockReturnValue("cached HTML");
  const { result } = renderHook(() => useHighlightedCode("cached source", "go"));
  expect(result.current).toBe("cached HTML");
  expect(mocks.requestHighlight).not.toHaveBeenCalled();
});
