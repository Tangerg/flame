import { OperationFailure, RpcError, asSessionId } from "@flame/runtime-client";
import { parseVisualDiff } from "../../../../../plugins/diff/component/visualDiff";
import {
  addInstalledMemory,
  deleteInstalledMemory,
  readInstalledMemory,
  reviewInstalledMemory,
  updateInstalledMemory,
} from "./memoryOperations";
import { execFile } from "node:child_process";
import { mkdir, realpath, symlink, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { promisify } from "node:util";
import { describe, expect, it } from "vitest";
import { collectRunEvents, nextRuntimeEvent, originalOpeningEvents, within } from "./observation";
import { useRuntimeFixture } from "./fixture";

const execFileAsync = promisify(execFile);

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: workspace", () => {
  const fixture = useRuntimeFixture("workspace");

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
        { path: "tracked.txt", status: "modified", added: 1, removed: 1 },
        { path: "new-中文.txt", status: "untracked", added: 1, removed: 0 },
      ],
      truncated: false,
      budget: { files: 5_000, rows: 5_000, bytes: 64 * 1024 * 1024 },
    });
    expect([...parseVisualDiff(visual).rendered.keys()]).toEqual(["tracked.txt", "new-中文.txt"]);

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
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["plugins.dataChanged", "sessions.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();
    const project = await addInstalledMemory(fixture.client, {
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
    const duplicate = await addInstalledMemory(fixture.client, {
      scope: "project",
      cwd: workspaceRoot,
      content: "project memory marker",
    });
    expect(duplicate.id).toBe(project.id);
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    await expect(readInstalledMemory(fixture.client, "project", workspaceRoot)).resolves.toEqual([
      project,
    ]);

    const orderingMarker = await fixture.client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "Agent memory duplicate ordering marker",
    });
    const next = await within(runtimeEvents.next(), "event after duplicate agent-memory add");
    expect(next).toMatchObject({
      done: false,
      value: { type: "sessions.changed", sessionIds: expect.arrayContaining([orderingMarker.id]) },
    });

    const user = await addInstalledMemory(fixture.client, {
      scope: "user",
      content: "user memory marker",
    });
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    expect(user).toMatchObject({ scope: "user", origin: "user", status: "active" });
    await expect(
      readInstalledMemory(fixture.client, "project", workspace.ref.path).then((data) => ({ data })),
    ).resolves.toMatchObject({
      data: [expect.objectContaining({ id: project.id, content: "project memory marker" })],
    });
    await expect(
      readInstalledMemory(fixture.client, "user", "").then((data) => ({ data })),
    ).resolves.toMatchObject({
      data: [expect.objectContaining({ id: user.id, content: "user memory marker" })],
    });

    const updated = await updateInstalledMemory(fixture.client, {
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
      readInstalledMemory(fixture.client, "project", workspace.ref.path).then((data) => ({ data })),
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
      reviewInstalledMemory(fixture.client, project.id, updated.revision, "approve"),
    ).rejects.toSatisfy(
      (error: unknown) => error instanceof OperationFailure && error.code === "not_pending",
    );

    await deleteInstalledMemory(fixture.client, project.id, updated.revision);
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    await deleteInstalledMemory(fixture.client, user.id, user.revision);
    await expect(nextRuntimeEvent(runtimeEvents, "plugins.dataChanged")).resolves.toMatchObject({
      type: "plugins.dataChanged",
    });
    expect(
      (
        await readInstalledMemory(fixture.client, "project", workspace.ref.path).then((data) => ({
          data,
        }))
      ).data.some((item) => item.id === project.id),
    ).toBe(false);
    expect(
      (await readInstalledMemory(fixture.client, "user", "").then((data) => ({ data }))).data.some(
        (item) => item.id === user.id,
      ),
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
