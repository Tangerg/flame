import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { activeMention, useFileMentions } from "./fileMentions";
import { draftMentions } from "./draftContext";

const workspaceFiles = vi.hoisted(() => ({
  current: [] as Array<{ path: string }>,
}));

vi.mock("@/plugins/builtin/workspace/public/queries", () => ({
  useWorkspaceListFiles: () => ({ data: workspaceFiles.current }),
}));

beforeEach(() => {
  workspaceFiles.current = [
    { path: "src/apple.ts" },
    { path: "src/banana.ts" },
    { path: "src/cherry.ts" },
  ];
});

describe("activeMention", () => {
  it("detects a bare @ at the start", () => {
    expect(activeMention("@", 1)).toEqual({ query: "", start: 0, end: 1 });
  });

  it("detects a mid-text mention after whitespace", () => {
    expect(activeMention("see @comp", 9)).toEqual({ query: "comp", start: 4, end: 9 });
  });

  it("does not trigger on an @ inside a word (e.g. an email)", () => {
    expect(activeMention("user@host", 9)).toBeNull();
  });

  it("ends the mention at whitespace", () => {
    expect(activeMention("@a ", 3)).toBeNull();
    expect(activeMention("@a b", 4)).toBeNull();
  });

  it("returns null when there's no @ before the caret", () => {
    expect(activeMention("hello", 5)).toBeNull();
    expect(activeMention("", 0)).toBeNull();
  });
});

describe("useFileMentions", () => {
  it("keys the highlighted row to the current mention candidates", () => {
    const apply = vi.fn();
    const { result, rerender } = renderHook(
      ({ value, caret }) => useFileMentions({ value, caret, cwd: "/repo", apply }),
      { initialProps: { value: "@", caret: 1 } },
    );

    act(() => result.current.setIndex(2));
    expect(result.current.index).toBe(2);

    rerender({ value: "@s", caret: 2 });
    expect(result.current.index).toBe(0);

    act(() => result.current.setIndex(1));
    rerender({ value: "@s", caret: 2 });
    expect(result.current.index).toBe(1);

    workspaceFiles.current = [{ path: "src/delta.ts" }, { path: "src/echo.ts" }];
    rerender({ value: "@s", caret: 2 });
    expect(result.current.index).toBe(0);
  });

  it("leaves the @ in place, because that token is what the chip row reads back", () => {
    const apply = vi.fn();
    workspaceFiles.current = [{ path: "src/alpha.ts" }];
    const { result } = renderHook(
      ({ value, caret }) => useFileMentions({ value, caret, cwd: "/repo", apply }),
      { initialProps: { value: "see @al", caret: 7 } },
    );

    act(() => result.current.accept("src/alpha.ts"));
    expect(apply).toHaveBeenCalledWith("see @src/alpha.ts ", 18);
    expect(draftMentions("see @src/alpha.ts ")).toEqual([
      { path: "src/alpha.ts", start: 4, end: 17 },
    ]);
  });

  it("closes the token so the picker does not reopen on what was just accepted", () => {
    const apply = vi.fn();
    workspaceFiles.current = [{ path: "src/alpha.ts" }];
    const { result } = renderHook(
      ({ value, caret }) => useFileMentions({ value, caret, cwd: "/repo", apply }),
      { initialProps: { value: "@al", caret: 3 } },
    );
    act(() => result.current.accept("src/alpha.ts"));
    const [text, caret] = apply.mock.calls[0]!;
    expect(activeMention(text as string, caret as number)).toBeNull();
  });
});
