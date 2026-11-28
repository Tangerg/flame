import { mkdir, stat, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { JSONRPC_VERSION } from "@flame/runtime-client";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import { describe, expect, it } from "vitest";
import { useRuntimeFixture } from "./fixture";
import { crashAfterGit, gitIn } from "./gitCrash";

describe("Git worktree receipts after Runtime process exit", () => {
  const fixture = useRuntimeFixture("git-crash");

  it.each(["apply", "release"] as const)("retains the original %s receipt", async (operation) => {
    const source = join(fixture.root, operation);
    await mkdir(source);
    const git = gitIn(source);
    const file = "tracked.txt";
    const baseline = "original\n";
    const proposed = "handed off\n";
    await git("init", "--quiet", "--template=");
    await writeFile(join(source, file), baseline);
    await git("add", "--", file);
    await git("commit", "--quiet", "-m", "baseline");
    const task = await fixture.client.workspaces.tasks.create({
      source: { path: source },
      base: "HEAD",
    });
    if (operation === "apply") await writeFile(join(task.workspace.path, file), proposed);
    const preview =
      operation === "apply" ? await fixture.client.workspaces.tasks.preview(task.id) : null;
    const method = preview ? "workspaces.tasks.handoff.apply" : "workspaces.tasks.release";
    const params = preview ? { id: task.id, digest: preview.digest } : { id: task.id };
    const result = { jsonrpc: JSONRPC_VERSION, id: method, result: preview ?? {} };
    const { capabilities } = await fixture.client.runtime.discover();
    const headers = {
      [HTTP_HEADERS.idempotencyKey]: `git-crash-${operation}`,
      [HTTP_HEADERS.idempotencyNamespace]: capabilities.limits.idempotency.namespace,
    };
    const call = () => fixture.postRPC(method, params, headers).then((response) => response.json());

    await crashAfterGit(fixture, preview ? "apply" : "remove", call, async () => {
      if (preview) {
        expect((await git("show", `:${file}`)).stdout).toBe(proposed);
      } else {
        await expect(stat(task.workspace.path)).rejects.toMatchObject({ code: "ENOENT" });
        expect((await git("worktree", "list", "--porcelain")).stdout).not.toContain(
          task.workspace.path,
        );
      }
    });
    await fixture.startRuntimeProcess({ executableSearchPath: "" });
    fixture.client = fixture.createRuntimeClient();
    try {
      const { capabilities } = await fixture.client.runtime.discover();
      expect(capabilities.features.git?.enabled).toBe(false);
      await expect(call()).resolves.toEqual(result);
      await expect(call()).resolves.toEqual(result);
    } finally {
      if (operation === "apply") {
        await fixture.client.close();
        await fixture.stopRuntimeProcess();
        await fixture.startRuntimeProcess();
        fixture.client = fixture.createRuntimeClient();
      }
    }
  });
});
