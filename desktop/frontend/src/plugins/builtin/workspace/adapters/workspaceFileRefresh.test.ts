import { QueryObserver } from "@tanstack/react-query";
import { waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { invalidateWorkspaceEvent } from "./queryInvalidation";
import {
  installWorkspaceFocusRefresh,
  subscribeWorkspaceReadTargets,
  workspaceReadTargets,
} from "./workspaceReadObservation";

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  AGENT_SESSIONS_KEY: "agent-sessions",
  AGENT_SESSION_USAGE_KEY: "agent-session-usage",
  synchronizeMountedAgentSessions: vi.fn(),
}));

const dispose: (() => void)[] = [];
beforeEach(() => queryClient.clear());
afterEach(() => {
  for (const stop of dispose.splice(0)) stop();
  queryClient.clear();
});

function observe(key: string, params: object) {
  const fetcher = vi.fn().mockResolvedValue("loaded");
  const observer = new QueryObserver(queryClient, {
    queryKey: [key, params],
    queryFn: fetcher,
    staleTime: Infinity,
    retry: false,
  });
  const stop = observer.subscribe(() => {});
  dispose.push(stop);
  return { fetcher, stop };
}

describe("mounted workspace file refresh", () => {
  it("refetches all ranges and ancestor lists while preserving other workspace caches", async () => {
    const first = observe("read-file", {
      cwd: "/first",
      path: "src/same.ts",
      startLine: 1,
      endLine: 10,
    });
    const range = observe("read-file", {
      cwd: "/first",
      path: "src/same.ts",
      startLine: 20,
      endLine: 30,
    });
    const otherWorkspace = observe("read-file", { cwd: "/second", path: "src/same.ts" });
    const otherFile = observe("read-file", { cwd: "/first", path: "other.ts" });
    const root = observe("list-files", { cwd: "/first" });
    const directory = observe("list-files", { cwd: "/first", path: "src" });
    const unrelated = observe("list-files", { cwd: "/first", path: "other" });
    await waitFor(() => expect(queryClient.isFetching()).toBe(0));

    invalidateWorkspaceEvent({
      type: "files.changed",
      sequence: 1,
      workspace: { path: "/first" },
      paths: ["src/same.ts"],
    });
    await waitFor(() => expect(first.fetcher).toHaveBeenCalledTimes(2));
    expect(range.fetcher).toHaveBeenCalledTimes(2);
    expect(root.fetcher).toHaveBeenCalledTimes(2);
    expect(directory.fetcher).toHaveBeenCalledTimes(2);
    expect(otherWorkspace.fetcher).toHaveBeenCalledOnce();
    expect(otherFile.fetcher).toHaveBeenCalledOnce();
    expect(unrelated.fetcher).toHaveBeenCalledOnce();
  });

  it("derives bounded targets from mounted reads and releases collapsed directories", async () => {
    const changed = vi.fn();
    dispose.push(subscribeWorkspaceReadTargets(changed));
    observe("read-file", { cwd: "/first", path: "src/same.ts", startLine: 1 });
    observe("read-file", { cwd: "/first", path: "src/same.ts", startLine: 20 });
    const directory = observe("list-files", { cwd: "/first", path: "src" });
    observe("list-files", { cwd: "/second" });
    queryClient.setQueryData(["read-file", { cwd: "/inactive", path: "old.ts" }], "cached");
    await waitFor(() => expect(queryClient.isFetching()).toBe(0));
    expect(workspaceReadTargets()).toEqual([
      { cwd: "/first", paths: ["src", "src/same.ts"] },
      { cwd: "/second", paths: ["."] },
    ]);
    changed.mockClear();
    directory.stop();
    expect(workspaceReadTargets()[0]?.paths).toEqual(["src/same.ts"]);
    expect(changed).toHaveBeenCalledOnce();
  });

  it("revalidates mounted file panels on focus and removes its listener on disposal", async () => {
    const file = observe("read-file", { cwd: "/first", path: "same.ts" });
    const stop = installWorkspaceFocusRefresh();
    dispose.push(stop);
    await waitFor(() => expect(queryClient.isFetching()).toBe(0));
    window.dispatchEvent(new Event("focus"));
    await waitFor(() => expect(file.fetcher).toHaveBeenCalledTimes(2));
    stop();
    window.dispatchEvent(new Event("focus"));
    expect(file.fetcher).toHaveBeenCalledTimes(2);
  });
});
