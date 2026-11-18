import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { QueryClientProvider } from "@tanstack/react-query";
import { act, waitFor } from "@testing-library/react";
import { type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { useWorkspaceDirectory } from "@/features/workspace/application/workspaceQueries";
import { useWorkspaceListFiles } from "@/features/workspace/public/queries";
import { type Disposable } from "@/extensions/types/lifetime";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { respondSuccess } from "@flame/runtime-client/testing";
import { registerWorkspaceDataProviders } from "@/features/workspace/adapters/runtimeDataProviders";
import { contributeForTest } from "@/test/kernel";
let disposables: Disposable[] = [];

let transport: ReturnType<typeof createMemoryTransport>;

let client: ReturnType<typeof createFlameClient>;

let unmountHook: (() => void) | undefined;

function file(path: string) {
  return {
    path,
    name: path,
    type: "file",
    modifiedAt: "2026-08-14T00:00:00Z",
  };
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(async () => {
  queryClient.clear();
  disposables = [];
  transport = createMemoryTransport();
  client = createFlameClient(transport);
  await contributeForTest((ctx) => registerWorkspaceDataProviders(ctx.contributions, () => client));
});

afterEach(async () => {
  unmountHook?.();
  unmountHook = undefined;
  queryClient.clear();
  for (const disposable of disposables.reverse()) disposable[Symbol.dispose]();
  await client.close();
  vi.restoreAllMocks();
});

function page(path: string, nextCursor?: string) {
  return {
    data: [file(path)],
    complete: nextCursor === undefined,
    ...(nextCursor ? { nextCursor } : {}),
    coverage: { scannedEntries: 1, excludedEntries: 0, budgetExhausted: false },
  };
}
describe("mounted Runtime directory pages", () => {
  it("returns a single honest page without eagerly traversing the directory", async () => {
    const hook = testClient.renderHook(() => useWorkspaceListFiles({ cwd: "/captured" }), {
      wrapper,
    });
    unmountHook = hook.unmount;
    const first = await transport.waitForRequest("workspace.files.list");
    respondSuccess(transport, first.id, page("first.ts", "continuation"));
    await waitFor(() => expect(hook.result.current.data?.data[0]?.path).toBe("first.ts"));
    expect(hook.result.current.data?.complete).toBe(false);
    expect(hook.result.current.data?.nextCursor).toBe("continuation");
    expect(
      transport.outbox().filter(({ method }) => method === "workspace.files.list"),
    ).toHaveLength(1);
  });
  it("keeps a continuation bound to its original workspace and fences its late reply", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = testClient.renderHook(({ cwd }) => useWorkspaceDirectory({ cwd }), {
      wrapper,
      initialProps: { cwd: "/original" },
    });
    unmountHook = hook.unmount;
    const first = await transport.waitForRequest("workspace.files.list");
    respondSuccess(transport, first.id, page("old.ts", "next-old"));
    await waitFor(() => expect(hook.result.current.hasNextPage).toBe(true));
    let pending!: ReturnType<typeof hook.result.current.fetchNextPage>;
    act(() => {
      pending = hook.result.current.fetchNextPage();
    });
    const next = await transport.waitForRequest(
      "workspace.files.list",
      ({ id }) => id !== first.id,
    );
    expect(next.params).toMatchObject({ workspace: { path: "/original" }, cursor: "next-old" });
    hook.rerender({ cwd: "/successor" });
    const successor = await transport.waitForRequest(
      "workspace.files.list",
      ({ id }) => id !== first.id && id !== next.id,
    );
    respondSuccess(transport, successor.id, page("successor.ts"));
    await waitFor(() =>
      expect(hook.result.current.data?.pages[0]?.data[0]?.path).toBe("successor.ts"),
    );
    expect(send.mock.calls[1]?.[1]?.aborted).toBe(true);
    respondSuccess(transport, next.id, page("late-original.ts"));
    await pending;
    expect(
      hook.result.current.data?.pages.flatMap(({ data }) => data.map(({ path }) => path)),
    ).toEqual(["successor.ts"]);
  });
});
