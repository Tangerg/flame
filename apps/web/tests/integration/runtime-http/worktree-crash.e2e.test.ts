import { execFile } from "node:child_process";
import { mkdir, stat, watch, writeFile } from "node:fs/promises";
import { delimiter, join } from "node:path";
import { promisify } from "node:util";
import { JSONRPC_VERSION } from "@flame/runtime-client";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import { describe, expect, it } from "vitest";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

const exec = promisify(execFile);

function shellWord(value: string) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

describe("Git worktree receipts after Runtime process exit", () => {
  const fixture = useRuntimeFixture("git-crash");

  it.each(["apply", "release"] as const)("retains the original %s receipt", async (operation) => {
    const source = join(fixture.root, operation);
    const control = join(fixture.root, `${operation}-git`);
    await mkdir(source);
    await mkdir(control);
    const git = (...args: string[]) =>
      exec(
        "git",
        [
          "-c",
          "user.name=Receipt Fixture",
          "-c",
          "user.email=fixture@example.test",
          "-c",
          "core.hooksPath=/dev/null",
          "-c",
          "commit.gpgSign=false",
          ...args,
        ],
        { cwd: source },
      );
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

    const gitExecutable = (await exec("/bin/sh", ["-c", "command -v git"])).stdout.trim();
    const continued = join(control, "continue");
    const marker = "git-written";
    const mutation = operation === "apply" ? "apply" : "remove";
    await writeFile(
      join(control, "git"),
      `#!/bin/sh
previous=
selected=
for argument do
  case "$argument" in
    apply) selected=apply ;;
    remove) if [ "$previous" = worktree ]; then selected=remove; fi ;;
  esac
  previous=$argument
done
if [ "$selected" != ${shellWord(mutation)} ]; then
  exec ${shellWord(gitExecutable)} "$@"
fi
${shellWord(gitExecutable)} "$@" || exit
: > ${shellWord(join(control, marker))}
while [ ! -f ${shellWord(continued)} ]; do sleep 0.02; done
`,
      { mode: 0o700 },
    );
    await fixture.client.close();
    await fixture.stopRuntimeProcess();
    await fixture.startRuntimeProcess({
      executableSearchPath: `${control}${delimiter}${process.env.PATH ?? ""}`,
    });
    const cut = new AbortController();
    const events = watch(control, { signal: cut.signal });
    const written = (async () => {
      for await (const event of events) if (event.filename === marker) return;
      throw new Error("native Git did not reach the write boundary");
    })();
    const original = call().then(
      (response) => {
        throw new Error(
          `original request answered before termination: ${JSON.stringify(response)}`,
        );
      },
      (cause) => cause,
    );
    try {
      await within(
        Promise.race([
          written,
          original.then((cause) => {
            throw cause;
          }),
        ]),
        "the completed native Git write",
      );
      if (preview) {
        expect((await git("show", `:${file}`)).stdout).toBe(proposed);
      } else {
        await expect(stat(task.workspace.path)).rejects.toMatchObject({ code: "ENOENT" });
        expect((await git("worktree", "list", "--porcelain")).stdout).not.toContain(
          task.workspace.path,
        );
      }
      await fixture.killRuntimeProcess();
      await within(original, "the disconnected original request");
    } finally {
      cut.abort();
      await writeFile(continued, "");
    }
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
