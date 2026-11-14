import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { activeMention, useFileMentions } from "@/features/composer/application/fileMentions";
import { draftMentions } from "@/features/composer/application/draftContext";
const workspaceFiles = vi.hoisted(() => ({
  current: [] as Array<{ path: string }> | undefined,
  isError: false,
  isFetching: false,
  isFetched: true,
  error: undefined as Error | undefined,
  refetch: vi.fn(() => Promise.resolve()),
}));

vi.mock("@/features/workspace/public/queries", () => ({
  useWorkspaceListFiles: () => ({
    data: workspaceFiles.current,
    isError: workspaceFiles.isError,
    isFetching: workspaceFiles.isFetching,
    isFetched: workspaceFiles.isFetched,
    error: workspaceFiles.error,
    refetch: workspaceFiles.refetch,
  }),
}));

beforeEach(() => {
  workspaceFiles.isError = false;
  workspaceFiles.isFetching = false;
  workspaceFiles.isFetched = true;
  workspaceFiles.error = undefined;
  workspaceFiles.refetch.mockClear();
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
    const { result, rerender } = testClient.renderHook(
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
    const { result } = testClient.renderHook(
      ({ value, caret }) => useFileMentions({ value, caret, cwd: "/repo", apply }),
      { initialProps: { value: "see @al", caret: 7 } },
    );

    act(() => result.current.accept("src/alpha.ts"));
    expect(apply).toHaveBeenCalledWith("see @src/alpha.ts ", 18);
    expect(draftMentions("see @src/alpha.ts ", new Set(["src/alpha.ts"]))).toEqual([
      { path: "src/alpha.ts", start: 4, end: 17 },
    ]);
  });

  it("closes the token so the picker does not reopen on what was just accepted", () => {
    const apply = vi.fn();
    workspaceFiles.current = [{ path: "src/alpha.ts" }];
    const { result } = testClient.renderHook(
      ({ value, caret }) => useFileMentions({ value, caret, cwd: "/repo", apply }),
      { initialProps: { value: "@al", caret: 3 } },
    );
    act(() => result.current.accept("src/alpha.ts"));
    const [text, caret] = apply.mock.calls[0]!;
    expect(activeMention(text as string, caret as number)).toBeNull();
  });
});

describe("useFileMentions status", () => {
  const apply = () => {};

  it("says why there is nothing to pick without a project", () => {
    const { result } = testClient.renderHook(() =>
      useFileMentions({ value: "@a", caret: 2, cwd: undefined, apply }),
    );
    expect(result.current.open).toBe(true);
    expect(result.current.status).toBe("no-workspace");
  });

  it("keeps the panel open with an empty result instead of vanishing", () => {
    workspaceFiles.current = [];
    const { result } = testClient.renderHook(() =>
      useFileMentions({ value: "@zzz", caret: 4, cwd: "/repo", apply }),
    );
    expect(result.current.open).toBe(true);
    expect(result.current.status).toBe("empty");
  });

  it("opens a new mention at the previously dismissed position", () => {
    const { result, rerender } = testClient.renderHook(
      ({ value }) => useFileMentions({ value, caret: value.length, cwd: "/repo", apply }),
      { initialProps: { value: "@a" } },
    );
    act(() => result.current.dismiss());
    expect(result.current.open).toBe(false);
    rerender({ value: "" });
    rerender({ value: "@b" });
    expect(result.current.open).toBe(true);
  });

  it("reports a listing that has not arrived as loading", () => {
    workspaceFiles.current = undefined;
    const { result } = testClient.renderHook(() =>
      useFileMentions({ value: "@a", caret: 2, cwd: "/repo", apply }),
    );
    expect(result.current.status).toBe("loading");
  });

  it("retains the retry action across its pending read and retires it on success", () => {
    workspaceFiles.current = undefined;
    workspaceFiles.isError = true;
    workspaceFiles.error = new Error("listing failed");
    const { result, rerender } = testClient.renderHook(() =>
      useFileMentions({ value: "@a", caret: 2, cwd: "/repo", apply }),
    );
    expect(result.current.retry?.status).toBe("ready");
    act(() => result.current.retry?.run());
    expect(workspaceFiles.refetch).toHaveBeenCalledWith({ cancelRefetch: false });

    workspaceFiles.isError = false;
    workspaceFiles.error = undefined;
    workspaceFiles.isFetching = true;
    rerender();
    expect(result.current.status).toBe("loading");
    expect(result.current.retry?.status).toBe("pending");

    workspaceFiles.current = [{ path: "a.ts" }];
    workspaceFiles.isFetching = false;
    rerender();
    expect(result.current.status).toBe("ready");
    expect(result.current.retry).toBeUndefined();
  });

  it("offers no retry before the first read has settled", () => {
    workspaceFiles.current = undefined;
    workspaceFiles.isFetching = true;
    workspaceFiles.isFetched = false;
    const { result } = testClient.renderHook(() =>
      useFileMentions({ value: "@a", caret: 2, cwd: "/repo", apply }),
    );
    expect(result.current.status).toBe("loading");
    expect(result.current.retry).toBeUndefined();
  });
});
