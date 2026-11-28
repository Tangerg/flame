import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { asSessionId } from "@flame/runtime-client";
import { HTTP_HEADERS, type Session } from "@flame/runtime-client/wire";
import { describe, expect, it } from "vitest";
import { useRuntimeFixture } from "./fixture";
import { crashAfterGit, gitIn } from "./gitCrash";
import { collectRunEvents, originalOpeningEvents } from "./observation";

describe("independent Session fork commit", () => {
  const fixture = useRuntimeFixture("fork-crash");

  it("publishes the admitted history and checkout after Runtime exits during the fork", async () => {
    const source = join(fixture.root, "source");
    await mkdir(source);
    const git = gitIn(source);
    const file = "tracked.txt";
    const approved = "approved for fork\n";
    const prompt = "E2E_FORK preserve this visible turn.";
    await git("init", "--quiet", "--template=");
    await writeFile(join(source, file), "baseline\n");
    await git("add", "--", file);
    await git("commit", "--quiet", "-m", "baseline");

    const parent = await fixture.client.sessions.create({
      workspace: { path: source },
      title: "Parent",
    });
    const originalTurn = await fixture.client.runs.start({
      sessionId: asSessionId(parent.id),
      input: [{ type: "text", text: prompt }],
    });
    expect(
      (await collectRunEvents(originalOpeningEvents(originalTurn))).at(-1)?.event,
    ).toMatchObject({
      type: "segment.finished",
      run: { outcome: { type: "completed" } },
    });

    await writeFile(join(source, file), approved);
    const preview = await fixture.client.sessions.previewFork({ sessionId: parent.id });
    expect(preview.runs).toBe(1);
    expect(preview.patch).toContain(approved.trim());
    const { capabilities } = await fixture.client.runtime.discover();
    const method = "sessions.fork";
    const call = async () => {
      const response = await fixture.postRPC(
        method,
        { sessionId: parent.id, worktree: preview.worktree },
        {
          [HTTP_HEADERS.idempotencyKey]: "original-worktree-fork",
          [HTTP_HEADERS.idempotencyNamespace]: capabilities.limits.idempotency.namespace,
        },
      );
      return response.json() as Promise<{ result: Session }>;
    };
    const checkout = gitIn(preview.destination.path);
    await crashAfterGit(
      fixture,
      "read-tree",
      call,
      async () => {
        expect((await git("worktree", "list", "--porcelain")).stdout).toContain(
          preview.destination.path,
        );
        expect((await checkout("rev-parse", "HEAD")).stdout.trim()).toBe(preview.head);
        expect((await checkout("show", `:${file}`)).stdout).toBe(approved);
        await expect(readFile(join(preview.destination.path, file), "utf8")).resolves.toBe(
          approved,
        );
        expect((await fixture.client.sessions.list()).data.map(({ id }) => id)).toEqual([
          parent.id,
        ]);
      },
      async () => {
        await writeFile(join(source, file), "new source state\n");
        await git("add", "--", file);
        await git("commit", "--quiet", "-m", "new source");
      },
    );

    await fixture.startRuntimeProcess({ executableSearchPath: "" });
    fixture.client = fixture.createRuntimeClient();
    const discovery = await fixture.client.runtime.discover();
    expect(discovery.capabilities.features.git?.enabled).toBe(false);
    await fixture.client.sessions.delete(asSessionId(parent.id));
    const receipt = await call();
    expect(receipt).toMatchObject({
      result: { id: expect.any(String), title: "Parent", workspace: { ref: preview.destination } },
    });
    const child = asSessionId(receipt.result.id);
    const copied = await fixture.client.sessions.snapshot(child);
    expect(copied.runs).toHaveLength(1);
    expect(copied.runs[0]).not.toMatchObject({ id: originalTurn.result.runId });
    expect(copied.items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ type: "userMessage", content: [{ type: "text", text: prompt }] }),
      ]),
    );
    await fixture.client.sessions.update({
      sessionId: child,
      expectedRevision: receipt.result.revision,
      title: "Renamed after publication",
    });
    await expect(call()).resolves.toEqual(receipt);
    const registrations = (await git("worktree", "list", "--porcelain")).stdout;
    expect(
      registrations.split("\n").filter((line) => line === `worktree ${preview.destination.path}`),
    ).toHaveLength(1);
    await expect(readFile(join(preview.destination.path, file), "utf8")).resolves.toBe(approved);
  });
});
