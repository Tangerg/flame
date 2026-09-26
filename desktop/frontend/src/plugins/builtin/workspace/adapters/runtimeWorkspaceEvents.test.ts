import { beforeEach, describe, expect, it, vi } from "vitest";
import { subscribeRuntimeWorkspaceEvents } from "./runtimeWorkspaceEvents";

const { fileWatch, resolveWorkspace, subscribe, supportedTopics } = vi.hoisted(() => ({
  fileWatch: vi.fn(() => true),
  resolveWorkspace: vi.fn(),
  subscribe: vi.fn(),
  supportedTopics: new Set<string>(),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  runtimeCapability: fileWatch,
  runtimeSupportsStreamingMethod: vi.fn(() => true),
  runtimeSupportsTopic: (topic: string) => supportedTopics.has(topic),
}));

vi.mock("@/main/container", () => ({
  getContainer: () => ({
    client: () => ({
      workspaces: { resolve: resolveWorkspace },
      runtimeEvents: { subscribe },
    }),
  }),
}));

const events = {
  async *[Symbol.asyncIterator]() {},
};

beforeEach(() => {
  fileWatch.mockReturnValue(true);
  supportedTopics.clear();
  for (const topic of [
    "files.changed",
    "skills.changed",
    "mcp.changed",
    "schedules.changed",
    "sessions.changed",
    "runs.changed",
    "interrupts.changed",
    "goals.changed",
    "plan.changed",
    "hooks.changed",
    "models.changed",
    "approvals.changed",
    "agentMemory.changed",
  ]) {
    supportedTopics.add(topic);
  }
  resolveWorkspace.mockReset();
  subscribe.mockReset();
  subscribe.mockResolvedValue({ result: {}, events });
});

describe("runtime workspace event subscription", () => {
  it("releases a scoped stream even before its first event is requested", async () => {
    resolveWorkspace.mockResolvedValue({ ref: { path: "/repo" }, availability: "available" });
    const release = vi.fn().mockResolvedValue({ done: true });
    subscribe.mockResolvedValue({
      events: { [Symbol.asyncIterator]: () => ({ next: vi.fn(), return: release }) },
    });
    const observed = await subscribeRuntimeWorkspaceEvents(
      { type: "workspace", cwd: "/repo" },
      new AbortController().signal,
    );
    await observed[Symbol.asyncIterator]().return?.();
    expect(release).toHaveBeenCalledOnce();
  });
  it("observes mounted paths across workspaces and retains watch identity for Git resync", async () => {
    resolveWorkspace.mockImplementation(async (ref: { path: string }) => ({
      ref: { path: ref.path.replace("/alias", "/canonical") },
      availability: "available",
    }));
    subscribe.mockResolvedValue({
      events: (async function* () {
        yield {
          type: "resync",
          sequence: 1,
          topics: ["files.changed"],
          watchIds: ["open-reads-1"],
        };
      })(),
    });
    const result = await subscribeRuntimeWorkspaceEvents(
      {
        type: "workspace",
        cwd: "/alias/first",
        reads: [
          { cwd: "/alias/first", paths: ["src", "src/a.ts"] },
          { cwd: "/second", paths: ["same.ts"] },
        ],
      },
      new AbortController().signal,
    );
    expect(subscribe.mock.calls[0]?.[0].watches).toEqual([
      {
        watchId: "active-session",
        workspace: { path: "/canonical/first" },
        paths: ["src", "src/a.ts"],
      },
      { watchId: "open-reads-1", workspace: { path: "/second" }, paths: ["same.ts"] },
    ]);
    const received = [];
    for await (const event of result) received.push(event);
    expect(received[0]?.watchScopes).toEqual([
      { watchId: "active-session", workspace: { path: "/canonical/first" }, cwd: "/alias/first" },
      { watchId: "open-reads-1", workspace: { path: "/second" }, cwd: "/second" },
    ]);
  });
  it("uses the canonical available workspace as the file-watch scope", async () => {
    resolveWorkspace.mockResolvedValue({
      ref: { path: "/canonical/repo" },
      projectRoot: "/canonical/repo",
      availability: "available",
    });
    const signal = new AbortController().signal;

    const observed = await subscribeRuntimeWorkspaceEvents(
      { type: "workspace", cwd: "/linked/repo" },
      signal,
    );
    expect(observed[Symbol.asyncIterator]).toBeTypeOf("function");

    expect(resolveWorkspace).toHaveBeenCalledWith({ path: "/linked/repo" }, signal);
    expect(subscribe).toHaveBeenCalledWith(
      expect.objectContaining({
        topics: expect.arrayContaining([
          "hooks.changed",
          "models.changed",
          "approvals.changed",
          "agentMemory.changed",
        ]),
        watches: [{ watchId: "active-session", workspace: { path: "/canonical/repo" } }],
      }),
      signal,
    );
  });

  it("intersects foldable topics with discovery for an older Runtime", async () => {
    supportedTopics.delete("hooks.changed");
    const signal = new AbortController().signal;

    await expect(subscribeRuntimeWorkspaceEvents({ type: "none" }, signal)).resolves.toBe(events);

    expect(subscribe).toHaveBeenCalledWith(
      expect.objectContaining({
        topics: expect.arrayContaining(["files.changed", "sessions.changed", "goals.changed"]),
      }),
      signal,
    );
    const request = subscribe.mock.calls[0]?.[0];
    expect(request.topics).not.toContain("hooks.changed");
  });

  it("cancels watch-root resolution with the subscription lifecycle", async () => {
    const controller = new AbortController();
    const reason = new Error("retargeted");
    resolveWorkspace.mockImplementation(
      (_ref: unknown, signal: AbortSignal) =>
        new Promise((_, reject) => {
          signal.addEventListener("abort", () => reject(signal.reason), { once: true });
        }),
    );

    const opening = subscribeRuntimeWorkspaceEvents(
      { type: "workspace", cwd: "/repo" },
      controller.signal,
    );
    controller.abort(reason);

    await expect(opening).rejects.toBe(reason);
    expect(subscribe).not.toHaveBeenCalled();
  });

  it("keeps global invalidations online when the active workspace disappeared", async () => {
    resolveWorkspace.mockResolvedValue({
      ref: { path: "/missing/repo" },
      projectRoot: "/missing/repo",
      availability: "missing",
    });
    const signal = new AbortController().signal;

    await expect(
      subscribeRuntimeWorkspaceEvents({ type: "workspace", cwd: "/missing/repo" }, signal),
    ).resolves.toBe(events);

    expect(subscribe).toHaveBeenCalledWith(
      expect.not.objectContaining({ watches: expect.anything() }),
      signal,
    );
  });

  it("does not resolve a watch scope when file watching is unavailable", async () => {
    fileWatch.mockReturnValue(false);
    const signal = new AbortController().signal;

    await expect(subscribeRuntimeWorkspaceEvents({ type: "workspace" }, signal)).resolves.toBe(
      events,
    );

    expect(resolveWorkspace).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith(
      expect.not.objectContaining({ watches: expect.anything() }),
      signal,
    );
  });

  it("subscribes global topics without resolving a default watch while identity is unknown", async () => {
    const signal = new AbortController().signal;

    await expect(subscribeRuntimeWorkspaceEvents({ type: "none" }, signal)).resolves.toBe(events);

    expect(resolveWorkspace).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith(
      expect.not.objectContaining({ watches: expect.anything() }),
      signal,
    );
  });
});
