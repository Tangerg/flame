import { renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useCodeHighlighter } from "@/lib/highlight/useCodeHighlight";

const highlighter = vi.hoisted(() => ({
  getHighlighter: vi.fn(),
  reportHighlightFailure: vi.fn(),
}));
vi.mock("@/lib/highlight/shiki", () => ({ ...highlighter, resolveLang: vi.fn() }));
vi.mock("@/lib/appearance", () => ({ useScheme: () => "light" }));

it("keeps plain code available when the highlighter cannot load", async () => {
  const failure = new Error("chunk unavailable");
  highlighter.getHighlighter.mockRejectedValue(failure);
  const { result } = renderHook(() => useCodeHighlighter());
  await waitFor(() =>
    expect(highlighter.reportHighlightFailure).toHaveBeenCalledExactlyOnceWith(
      "highlighter unavailable",
      failure,
    ),
  );
  expect(result.current.highlighter).toBeNull();
});
