import {
  JSONRPC_VERSION,
  OperationFailure,
  RpcError,
  asSessionId,
  createFlameClient,
  createHttpTransport,
} from "@flame/runtime-client";
import { HTTP_HEADERS, RPC_HTTP, type TaskWorktree } from "@flame/runtime-client/wire";
import type { WireMethodName } from "@flame/runtime-client/methods";
import { TaskWorktreeCommands } from "@/features/workspace/application/taskWorktreeCommands";
import { parseVisualDiff } from "../../../../../plugins/diff/component/visualDiff";
import {
  addInstalledMemory,
  captureMemoryBinding,
  deleteInstalledMemory,
  readInstalledMemory,
  reviewInstalledMemory,
  updateInstalledMemory,
} from "./memoryOperations";
import { execFile } from "node:child_process";
import { mkdir, readFile, realpath, stat, symlink, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { promisify } from "node:util";
import { describe, expect, it } from "vitest";
import {
  collectRunEvents,
  nextRuntimeEvent,
  originalOpeningEvents,
  watchRuntimeTopic,
  within,
} from "./observation";
import { useRuntimeFixture } from "./fixture";
import { MemoryMutationStorage } from "./mutationStorage";

const execFileAsync = promisify(execFile);

function workspaceGit(cwd: string) {
  return (...args: string[]) =>
    execFileAsync(
      "git",
      [
        "-c",
        "user.name=Runtime HTTP E2E",
        "-c",
        "user.email=runtime-http-e2e@example.invalid",
        "-c",
        "core.hooksPath=/dev/null",
        "-c",
        "commit.gpgSign=false",
        ...args,
      ],
      { cwd },
    );
}

describe("Go Runtime ↔ HTTP: Git command receipts", () => {
  const fixture = useRuntimeFixture("git-receipts");

  it("reads committed Git receipts while Git is unavailable", async () => {
    const source = join(fixture.root, "source");
    const file = "tracked.txt";
    const baseline = "original checkout\n";
    const material = "approved handoff\n";
    await mkdir(source);
    const git = workspaceGit(source);
    await git("init", "--quiet", "--template=");
    await writeFile(join(source, file), baseline);
    await git("add", "--", file);
    await git("commit", "--quiet", "-m", "source baseline");

    const { capabilities } = await fixture.client.runtime.discover();
    const namespace = capabilities.limits.idempotency.namespace;
    const send = async (method: WireMethodName, params: unknown, key: string) => {
      const response = await fixture.postRPC(method, params, {
        [HTTP_HEADERS.idempotencyKey]: key,
        [HTTP_HEADERS.idempotencyNamespace]: namespace,
      });
      expect(response.status).toBe(RPC_HTTP.callStatus);
      return (await response.json()) as { result?: unknown; error?: unknown };
    };
    const create = [
      "workspaces.tasks.create",
      { source: { path: source }, base: "HEAD" },
      "create",
    ] as const;
    const created = await send(...create);
    expect(created).toHaveProperty("result.workspace.path");
    const task = created.result as TaskWorktree;
    const taskFile = join(task.workspace.path, file);
    await writeFile(taskFile, material);
    const { digest } = await fixture.client.workspaces.tasks.preview(task.id);
    const apply = ["workspaces.tasks.handoff.apply", { id: task.id, digest }, "apply"] as const;
    const applied = await send(...apply);
    expect(applied).toMatchObject({ result: { digest } });
    expect((await git("show", `:${file}`)).stdout).toBe(material);
    await writeFile(taskFile, baseline);
    const release = ["workspaces.tasks.release", { id: task.id }, "release"] as const;
    const released = await send(...release);
    expect(released).toEqual({ jsonrpc: JSONRPC_VERSION, id: release[0], result: {} });
    await expect(stat(task.workspace.path)).rejects.toMatchObject({ code: "ENOENT" });

    await git("commit", "--quiet", "-m", "approved handoff");
    const { id } = await fixture.client.sessions.create({ workspace: { path: source } });
    const sessionId = asSessionId(id);
    const run = await fixture.client.runs.start({
      sessionId,
      input: [{ type: "text", text: "Capture the handed-off checkout." }],
    });
    await collectRunEvents(originalOpeningEvents(run));
    await writeFile(join(source, file), "edit after the checkpoint\n");
    const rollback = [
      "sessions.rollback",
      {
        sessionId,
        restoreType: "files",
        toRunId: run.result.runId,
        expectedLastRootRunId: run.result.runId,
        expectedWorkspacePath: await realpath(source),
      },
      "rollback",
    ] as const;
    const rolledBack = await send(...rollback);
    expect(rolledBack).toMatchObject({ result: { session: { id }, droppedRuns: [] } });
    await expect(readFile(join(source, file), "utf8")).resolves.toBe(material);

    const restart = async (executableSearchPath?: string) => {
      await fixture.client.close();
      await fixture.stopRuntimeProcess();
      await fixture.startRuntimeProcess({ executableSearchPath });
      fixture.client = fixture.createRuntimeClient();
      return (await fixture.client.runtime.discover()).capabilities;
    };
    const restricted = await restart("");
    expect(restricted.features.git?.enabled).toBe(false);
    expect(restricted.limits.idempotency.namespace).toBe(namespace);

    const denied = {
      error: { data: { problem: { type: "capability_not_negotiated" } } },
    };
    const newKey = "create-after-restart";
    await expect(send(create[0], create[1], newKey)).resolves.toMatchObject(denied);
    await expect(
      Promise.all([send(...create), send(...apply), send(...release), send(...rollback)]),
    ).resolves.toEqual([created, applied, released, rolledBack]);

    const rebound = await send(apply[0], { id: "different-task", digest }, apply[2]);
    expect(rebound).toMatchObject({
      error: { data: { problem: { type: "idempotency_conflict" } } },
    });
    await restart();
    await expect(send(create[0], create[1], newKey)).resolves.toHaveProperty(
      "result.workspace.path",
    );
  }, 30_000);
});

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: workspace", () => {
  const fixture = useRuntimeFixture("workspace");

  it("recovers the original worktree command through the Workspace owner", async () => {
    const source = join(fixture.root, "task-worktree-command");
    await mkdir(source);
    const git = workspaceGit(source);
    const file = "tracked.txt";
    const baseline = "original source\n";
    const change = "worktree handoff\n";
    await git("init", "--quiet", "--template=");
    await writeFile(join(source, file), baseline);
    await git("add", file);
    await git("commit", "--quiet", "-m", "source baseline");
    await git("switch", "--quiet", "--detach");

    const originalKeys: string[] = [];
    let deliverReceipts = false;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const headers = new Headers(init?.headers);
      const response = await fetch(url, init);
      if (headers.get(HTTP_HEADERS.method) === "workspaces.tasks.create") {
        const key = headers.get(HTTP_HEADERS.idempotencyKey);
        if (!key) throw new Error("the worktree command omitted its key");
        originalKeys.push(key);
        if (!deliverReceipts && response.ok) {
          await response.arrayBuffer();
          throw new Error("the worktree commit receipt was lost");
        }
      }
      return response;
    });
    const catalog = () => fixture.client.workspaces.tasks.list();
    const { events } = await watchRuntimeTopic(fixture.client, "taskWorktrees.changed");
    const commands = new TaskWorktreeCommands(() => client.workspaces.tasks);
    commands.replaceTarget();
    let recoveredPath: string | undefined;
    try {
      const request = { source: { path: source }, base: "HEAD" };
      await expect(commands.create(request)).resolves.toBe(false);
      expect(commands.snapshot()).toMatchObject({ phase: "unconfirmed", operation: "create" });
      await nextRuntimeEvent(events, "taskWorktrees.changed");
      const original = (await catalog()).data;
      expect(original).toHaveLength(1);
      const [worktree] = original;
      if (!worktree) throw new Error("the Runtime did not commit the original worktree");
      recoveredPath = join(worktree.workspace.path, file);

      const sent = originalKeys.length;
      await expect(commands.create(request)).resolves.toBe(false);
      expect(originalKeys).toHaveLength(sent);
      deliverReceipts = true;
      await expect(commands.recover()).resolves.toBe(true);
      expect(commands.snapshot()).toEqual({ phase: "committed", operation: "create" });
      expect(originalKeys).toHaveLength(sent + 1);
      expect(new Set(originalKeys).size).toBe(1);
      expect((await catalog()).data.map(({ id }) => id)).toEqual([worktree.id]);

      await writeFile(recoveredPath, change);
      const handoff = await client.workspaces.tasks.preview(worktree.id);
      expect(handoff.branch).toBeUndefined();
      expect(handoff.head).toBe(worktree.base);
      await expect(commands.apply({ id: worktree.id, digest: handoff.digest })).resolves.toBe(true);
      expect(commands.snapshot()).toEqual({ phase: "committed", operation: "apply" });
      expect((await git("show", `:${file}`)).stdout).toBe(change);
      await writeFile(recoveredPath, baseline);
      await expect(commands.release(worktree.id)).resolves.toBe(true);
      expect(commands.snapshot()).toEqual({ phase: "committed", operation: "release" });
      await nextRuntimeEvent(events, "taskWorktrees.changed");
      expect((await catalog()).data).toEqual([]);
    } finally {
      commands.retireTarget();
      await client.close();
      for (const worktree of (await catalog()).data) {
        if (recoveredPath && worktree.workspace.path === dirname(recoveredPath))
          await writeFile(recoveredPath, baseline);
        await fixture.client.workspaces.tasks.release(worktree.id);
      }
    }
  }, 30_000);

  it.each(["apply", "release"] as const)(
    "recovers the original %s receipt after replacing the Workspace owner",
    async (operation) => {
      const source = join(fixture.root, `task-recovery-${operation}`);
      await mkdir(source);
      const git = workspaceGit(source);
      const file = "tracked.txt";
      const baseline = "original source\n";
      const change = "approved handoff\n";
      await git("init", "--quiet", "--template=");
      await writeFile(join(source, file), baseline);
      await git("add", "--", file);
      await git("commit", "--quiet", "-m", "source baseline");
      const { tasks } = fixture.client.workspaces;
      const task = await tasks.create({ source: { path: source }, base: "HEAD" });
      const taskFile = join(task.workspace.path, file);
      const method =
        operation === "apply" ? "workspaces.tasks.handoff.apply" : "workspaces.tasks.release";
      const storage = new MemoryMutationStorage();
      const receipts: { key: string; result: unknown }[] = [];
      const openClient = (loseReceipt: boolean) =>
        createFlameClient(
          createHttpTransport({
            baseUrl: fixture.baseUrl,
            fetch: async (url, init) => {
              const response = await fetch(url, init);
              const headers = new Headers(init?.headers);
              if (headers.get(HTTP_HEADERS.method) !== method) return response;
              const key = headers.get(HTTP_HEADERS.idempotencyKey);
              if (!key) throw new Error("the worktree command omitted its key");
              const envelope = (await response.clone().json()) as { result?: unknown };
              if (Object.hasOwn(envelope, "result"))
                receipts.push({ key, result: envelope.result });
              if (loseReceipt) {
                await response.arrayBuffer();
                throw new Error("connection lost after Git committed");
              }
              return response;
            },
          }),
          { capabilities: () => fixture.negotiated, mutationStorage: storage },
        );
      const disconnected = openClient(true);
      const commands = new TaskWorktreeCommands(() => disconnected.workspaces.tasks);
      commands.replaceTarget();
      try {
        if (operation === "apply") {
          await writeFile(taskFile, change);
          const preview = await tasks.preview(task.id);
          await expect(commands.apply({ id: task.id, digest: preview.digest })).resolves.toBe(
            false,
          );
          expect((await git("show", `:${file}`)).stdout).toBe(change);
        } else {
          await expect(commands.release(task.id)).resolves.toBe(false);
          await expect(stat(task.workspace.path)).rejects.toMatchObject({ code: "ENOENT" });
        }
        expect(commands.snapshot()).toMatchObject({ phase: "unconfirmed", operation });
        const original = receipts[0];
        if (!original) throw new Error("the original Git receipt was not captured");
        const originalAttempts = receipts.length;
        expect(storage.size).toBe(1);
        commands.retireTarget();
        await disconnected.close();

        const connected = openClient(false);
        const restored = new TaskWorktreeCommands(() => connected.workspaces.tasks);
        restored.replaceTarget();
        try {
          restored.restorePending();
          expect(restored.snapshot()).toMatchObject({ phase: "unconfirmed", operation });
          await expect(restored.recover()).resolves.toBe(true);
          expect(restored.snapshot()).toEqual({ phase: "committed", operation });
          expect(receipts).toHaveLength(originalAttempts + 1);
          expect(receipts.at(-1)).toEqual(original);
          expect(storage.size).toBe(0);
        } finally {
          restored.retireTarget();
          await connected.close();
        }
      } finally {
        commands.retireTarget();
        await disconnected.close();
        if ((await tasks.list()).data.some(({ id }) => id === task.id)) {
          await writeFile(taskFile, baseline);
          await tasks.release(task.id);
        }
      }
    },
    30_000,
  );

  it("reads current diagnostic tool output through the query binding", async () => {
    const workspace = { path: join(fixture.root, "diagnostic-query") };
    await mkdir(workspace.path);
    const filename = "diagnostic-notes.txt";
    const path = join(workspace.path, filename);
    const read = () =>
      fixture.client.queryJSON("tools.invoke", {
        workspace,
        name: "read",
        arguments: { path: filename },
      });
    await writeFile(path, "original diagnostic text\n");
    await expect(read()).resolves.toContain("original diagnostic text");
    await writeFile(path, "current diagnostic text\n");
    await expect(read()).resolves.toContain("current diagnostic text");
  });

  it("reconciles an agent file write through files.changed", async () => {
    const workspaceRoot = join(fixture.root, "workspace-agent-write");
    await mkdir(workspaceRoot);
    await writeFile(join(workspaceRoot, "agent-write.txt"), "before\n");
    const session = await fixture.client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "HTTP agent file write",
    });
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["files.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const started = await fixture.client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_FILE_WRITE update the fixture." }],
    });
    const events = await collectRunEvents(originalOpeningEvents(started));
    expect(events.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });

    const changed = await nextRuntimeEvent(runtimeEvents, "files.changed");
    expect(changed).toMatchObject({
      type: "files.changed",
      paths: ["agent-write.txt"],
      workspace: session.workspace.ref,
    });
    const file = await fixture.client
      .workspace(session.workspace.ref)
      .files.read({ path: "agent-write.txt" });
    if (file.content !== "after\n") {
      const toolEvents = events.filter(
        (event) => event.event.type === "item.completed" && event.event.item.type === "toolCall",
      );
      throw new Error(`agent write did not persist: ${JSON.stringify(toolEvents)}`);
    }

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("keeps file-change signals scoped and reconciles them through cold reads", async () => {
    const repositoryRoot = join(fixture.root, "workspace-file-events");
    const workspaceRoot = join(repositoryRoot, "packages", "desktop");
    await mkdir(workspaceRoot, { recursive: true });
    await execFileAsync("git", ["init", "--quiet"], { cwd: repositoryRoot });
    await execFileAsync("git", ["config", "user.name", "Runtime HTTP E2E"], {
      cwd: repositoryRoot,
    });
    await execFileAsync("git", ["config", "user.email", "runtime-http-e2e@example.invalid"], {
      cwd: repositoryRoot,
    });
    await writeFile(join(workspaceRoot, "tracked.txt"), "before\n");
    await execFileAsync("git", ["add", "packages/desktop/tracked.txt"], { cwd: repositoryRoot });
    await execFileAsync("git", ["commit", "--quiet", "-m", "baseline"], {
      cwd: repositoryRoot,
    });

    const streamController = new AbortController();
    const watchId = "workspace-file-events";
    const subscription = await fixture.client.runtimeEvents.subscribe(
      {
        topics: ["files.changed"],
        watches: [{ watchId, workspace: { path: workspaceRoot } }],
      },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    await writeFile(join(workspaceRoot, "tracked.txt"), "after\n");
    await execFileAsync("git", ["add", "packages/desktop/tracked.txt"], { cwd: repositoryRoot });

    const changed = await nextRuntimeEvent(events, "resync");
    expect(changed.type === "resync" && changed.topics).toContain("files.changed");
    expect(changed.type === "resync" && changed.watchIds).toContain(watchId);

    const workspace = fixture.client.workspace({ path: workspaceRoot });
    await expect(workspace.files.read({ path: "tracked.txt" })).resolves.toMatchObject({
      content: "after\n",
    });
    await expect(workspace.changes.list()).resolves.toMatchObject({
      data: [{ path: "tracked.txt", status: "modified" }],
    });
    await expect(workspace.diff.get()).resolves.toMatchObject({
      type: "rows",
      rows: {
        files: [{ path: "tracked.txt", status: "modified" }],
        truncated: false,
      },
    });

    await writeFile(join(workspaceRoot, "new-中文.txt"), "新🙂");
    const raw = await workspace.diff.get({ format: "raw" });
    expect(raw.type).toBe("raw");
    if (raw.type !== "raw") throw new Error("expected the raw diff projection");
    expect(raw.patch).toContain("+新🙂\n\\ No newline at end of file\n");

    const visual = await workspace.diff.get({ format: "visual" });
    expect(visual.type).toBe("visual");
    if (visual.type !== "visual") throw new Error("expected the visual diff projection");
    expect(visual.baseline).toEqual(raw.baseline);
    expect(visual.visual).toEqual({
      patch: raw.patch,
      manifest: [
        { path: "new-中文.txt", status: "untracked", added: 1, removed: 0 },
        { path: "tracked.txt", status: "modified", added: 1, removed: 1 },
      ],
      truncated: false,
      budget: { files: 5_000, rows: 5_000, bytes: 64 * 1024 * 1024 },
    });
    expect([...parseVisualDiff(visual).rendered.keys()]).toEqual(["new-中文.txt", "tracked.txt"]);

    await expect(workspace.diff.get({ format: "visual", limit: 1 })).resolves.toEqual({
      type: "visual",
      baseline: raw.baseline,
      visual: {
        patch: "",
        manifest: [],
        truncated: true,
        budget: { files: 5_000, rows: 1, bytes: 64 * 1024 * 1024 },
      },
    });

    streamController.abort();
    await events.return?.();
  }, 30_000);

  it("round-trips project and user agent memory through durable cold reads", async () => {
    const workspaceRoot = join(fixture.root, "workspace-agent-memory");
    await mkdir(workspaceRoot);
    const workspace = fixture.client.workspace({ path: workspaceRoot });
    const binding = await captureMemoryBinding(fixture.client);
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["plugins.dataChanged", "sessions.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();
    const project = await addInstalledMemory(fixture.client, binding, {
      scope: "project",
      cwd: workspaceRoot,
      content: "project memory marker",
    });
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    expect(project).toMatchObject({
      scope: "project",
      content: "project memory marker",
      origin: "user",
      status: "active",
      pinned: false,
    });
    const duplicate = await addInstalledMemory(fixture.client, binding, {
      scope: "project",
      cwd: workspaceRoot,
      content: "project memory marker",
    });
    expect(duplicate.id).toBe(project.id);
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    await expect(
      readInstalledMemory(fixture.client, binding, "project", workspaceRoot),
    ).resolves.toEqual([project]);

    const orderingMarker = await fixture.client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "Agent memory duplicate ordering marker",
    });
    const next = await within(runtimeEvents.next(), "event after duplicate agent-memory add");
    expect(next).toMatchObject({
      done: false,
      value: { type: "sessions.changed", sessionIds: expect.arrayContaining([orderingMarker.id]) },
    });

    const user = await addInstalledMemory(fixture.client, binding, {
      scope: "user",
      content: "user memory marker",
    });
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    expect(user).toMatchObject({ scope: "user", origin: "user", status: "active" });
    await expect(
      readInstalledMemory(fixture.client, binding, "project", workspace.ref.path).then((data) => ({
        data,
      })),
    ).resolves.toMatchObject({
      data: [expect.objectContaining({ id: project.id, content: "project memory marker" })],
    });
    await expect(
      readInstalledMemory(fixture.client, binding, "user", "").then((data) => ({ data })),
    ).resolves.toMatchObject({
      data: [expect.objectContaining({ id: user.id, content: "user memory marker" })],
    });

    const updated = await updateInstalledMemory(fixture.client, binding, {
      id: project.id,
      expectedRevision: project.revision,
      content: "project memory marker updated",
      pinned: true,
    });
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    expect(updated).toMatchObject({
      id: project.id,
      content: "project memory marker updated",
      pinned: true,
    });
    await expect(
      readInstalledMemory(fixture.client, binding, "project", workspace.ref.path).then((data) => ({
        data,
      })),
    ).resolves.toMatchObject({
      data: [
        expect.objectContaining({
          id: project.id,
          content: "project memory marker updated",
          pinned: true,
        }),
      ],
    });
    await expect(
      reviewInstalledMemory(fixture.client, binding, project.id, updated.revision, "approve"),
    ).rejects.toSatisfy(
      (error: unknown) => error instanceof OperationFailure && error.code === "not_pending",
    );

    await deleteInstalledMemory(fixture.client, binding, project.id, updated.revision);
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    await deleteInstalledMemory(fixture.client, binding, user.id, user.revision);
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    expect(
      (
        await readInstalledMemory(fixture.client, binding, "project", workspace.ref.path).then(
          (data) => ({
            data,
          }),
        )
      ).data.some((item) => item.id === project.id),
    ).toBe(false);
    expect(
      (
        await readInstalledMemory(fixture.client, binding, "user", "").then((data) => ({ data }))
      ).data.some((item) => item.id === user.id),
    ).toBe(false);
    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("consumes workspace files and hook trust through bound APIs", async () => {
    const repositoryRoot = join(fixture.root, "workspace-side-api-project");
    const workspaceRoot = join(repositoryRoot, "packages", "desktop");
    await mkdir(workspaceRoot, { recursive: true });
    await execFileAsync("git", ["init", "--quiet"], { cwd: repositoryRoot });

    await Promise.all([
      mkdir(join(repositoryRoot, ".flame"), { recursive: true }),
      mkdir(join(workspaceRoot, ".flame"), { recursive: true }),
      mkdir(join(fixture.runtimeHome, ".flame"), { recursive: true }),
      mkdir(join(workspaceRoot, "nested"), { recursive: true }),
    ]);
    await Promise.all([
      writeFile(join(workspaceRoot, "alpha.txt"), "first\nworkspace-side-api-marker\nthird\n"),
      writeFile(join(workspaceRoot, "nested", "bravo.txt"), "workspace-side-api-marker\n"),
      writeFile(join(workspaceRoot, ".gitignore"), "ignored.log\n"),
      writeFile(join(workspaceRoot, "ignored.log"), "ignored marker\n"),
      writeFile(join(repositoryRoot, "AGENTS.md"), "project-root instructions\n"),
      writeFile(join(workspaceRoot, ".flame", "AGENTS.md"), "workspace instructions\n"),
      writeFile(join(fixture.runtimeHome, ".flame", "AGENTS.md"), "home instructions\n"),
      writeFile(
        join(fixture.runtimeHome, ".flame", "hooks.json"),
        JSON.stringify({ hooks: [{ event: "SessionStart", inject: "global hook context" }] }),
      ),
      writeFile(
        join(repositoryRoot, ".flame", "hooks.json"),
        JSON.stringify({
          hooks: [{ event: "PreToolUse", matcher: "shell", command: "true" }],
        }),
      ),
    ]);

    const resolved = await fixture.client.workspaces.resolve({ path: workspaceRoot });
    const canonicalWorkspaceRoot = await realpath(workspaceRoot);
    expect(resolved).toMatchObject({
      ref: { path: canonicalWorkspaceRoot },
      repositoryRoot: await realpath(repositoryRoot),
      availability: "available",
    });
    if (!resolved.repositoryRoot) throw new Error("nested Git workspace omitted its project root");

    const workspace = fixture.client.workspace({ path: workspaceRoot });
    await expect(
      workspace.files.read({ path: "alpha.txt", startLine: 1, endLine: 2 }),
    ).resolves.toMatchObject({ content: "first\nworkspace-side-api-marker", startLine: 1 });

    const listed = await workspace.files.list({ recursive: true, limit: 1 }).autoPagingToArray();
    expect(listed).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: "alpha.txt", name: "alpha.txt", type: "file" }),
        expect.objectContaining({ path: "nested/bravo.txt", name: "bravo.txt", type: "file" }),
      ]),
    );
    expect(listed.some((entry) => entry.path === "ignored.log")).toBe(false);
    const includingIgnored = await workspace.files
      .list({ recursive: true, includeIgnored: true, limit: 1 })
      .autoPagingToArray();
    expect(includingIgnored).toEqual(
      expect.arrayContaining([expect.objectContaining({ path: "ignored.log", type: "file" })]),
    );

    // A plain workspace cannot depend on Git's diagnosis of broken repository metadata.
    const plainRoot = join(fixture.environmentRoot, "workspace-side-api-plain");
    await mkdir(join(plainRoot, "empty"), { recursive: true });
    await writeFile(join(plainRoot, "visible.txt"), "visible\n");
    const plainListing = await fixture.client.workspace({ path: plainRoot }).files.list();
    expect(plainListing.data.toSorted((a, b) => a.path.localeCompare(b.path))).toMatchObject([
      { path: "empty", name: "empty", type: "dir" },
      { path: "visible.txt", name: "visible.txt", type: "file" },
    ]);
    await writeFile(join(plainRoot, ".git"), "gitdir: ../not-a-repository\n");
    const plainIncludingIgnored = await fixture.client
      .workspace({ path: plainRoot })
      .files.list({ includeIgnored: true });
    expect(plainIncludingIgnored.data.toSorted((a, b) => a.path.localeCompare(b.path))).toEqual(
      plainListing.data.toSorted((a, b) => a.path.localeCompare(b.path)),
    );
    expect(plainIncludingIgnored.data.some((entry) => entry.path === ".git")).toBe(false);
    const outsidePlainRoot = join(fixture.environmentRoot, "workspace-side-api-outside");
    await mkdir(outsidePlainRoot);
    await writeFile(join(outsidePlainRoot, "secret.txt"), "secret\n");
    await symlink(outsidePlainRoot, join(plainRoot, "escape"), "dir");
    await expect(
      fixture.client.workspace({ path: plainRoot }).files.list({ path: "escape" }),
    ).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "path_outside_root",
    );
    await expect(workspace.files.read({ path: "../AGENTS.md" })).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "path_outside_root",
    );

    const untrusted = await workspace.hooks.list();
    expect(untrusted).toMatchObject({
      repositoryRoot: resolved.repositoryRoot,
      repositoryTrusted: false,
    });
    expect(untrusted.hooks).toEqual([
      expect.objectContaining({
        event: "SessionStart",
        scope: "global",
        inject: "global hook context",
        active: true,
      }),
      expect.objectContaining({
        event: "PreToolUse",
        scope: "repository",
        matcher: "shell",
        command: "true",
        active: false,
      }),
    ]);
    const hookController = new AbortController();
    const hookSubscription = await fixture.client.runtimeEvents.subscribe(
      {
        topics: ["files.changed", "hooks.changed"],
        watches: [{ watchId: "hook-files", workspace: { path: workspaceRoot } }],
      },
      hookController.signal,
    );
    const hookEvents = hookSubscription.events[Symbol.asyncIterator]();
    await fixture.client.hooks.setTrust(resolved.repositoryRoot, true);
    await expect(nextRuntimeEvent(hookEvents, "hooks.changed")).resolves.toMatchObject({
      type: "hooks.changed",
    });
    await expect(workspace.hooks.list()).resolves.toMatchObject({
      repositoryRoot: resolved.repositoryRoot,
      repositoryTrusted: true,
      hooks: [
        expect.objectContaining({ scope: "global", active: true }),
        expect.objectContaining({ scope: "repository", active: true }),
      ],
    });
    await fixture.client.hooks.setTrust(resolved.repositoryRoot, false);
    await expect(nextRuntimeEvent(hookEvents, "hooks.changed")).resolves.toMatchObject({
      type: "hooks.changed",
    });
    await expect(workspace.hooks.list()).resolves.toMatchObject({ repositoryTrusted: false });

    const externalHookChanges = [
      {
        path: join(fixture.runtimeHome, ".flame", "hooks.json"),
        marker: "external global hook",
        scope: "global",
        event: "SessionStart",
      },
      {
        path: join(repositoryRoot, ".flame", "hooks.json"),
        marker: "external project hook",
        scope: "repository",
        event: "UserPromptSubmit",
      },
      {
        path: join(workspaceRoot, ".flame", "hooks.json"),
        marker: "external cwd hook",
        scope: "repository",
        event: "Notification",
      },
    ] as const;
    for (const change of externalHookChanges) {
      await mkdir(dirname(change.path), { recursive: true });
      await writeFile(
        change.path,
        JSON.stringify({ hooks: [{ event: change.event, inject: change.marker }] }),
      );
      await expect(nextRuntimeEvent(hookEvents, "hooks.changed")).resolves.toMatchObject({
        type: "hooks.changed",
      });
      await expect(workspace.hooks.list()).resolves.toMatchObject({
        hooks: expect.arrayContaining([
          expect.objectContaining({ scope: change.scope, inject: change.marker }),
        ]),
      });
    }
    hookController.abort();
    await hookEvents.return?.();
  }, 30_000);
});
