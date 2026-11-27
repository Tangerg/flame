import { mkdtemp, readdir } from "node:fs/promises";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import { describe, expect, it, onTestFinished } from "vitest";
import { asSessionId, RpcError } from "@flame/runtime-client";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

const deletionJournal = "session-deletions";

describe("Go CLI ↔ HTTP Runtime: session deletion settlement", () => {
  const fixture = useRuntimeFixture("cli-deletion");

  async function deleteSession(home: string, sessionId: string) {
    const command = fixture.startCLI(["sessions", "delete", sessionId, "--yes"], home);
    try {
      return await within(command.done, "CLI session deletion");
    } finally {
      if (command.child.exitCode === null) command.child.kill("SIGKILL");
      await command.done;
    }
  }

  async function openAuthoring() {
    const home = await mkdtemp(join(fixture.environmentRoot, "deletion-authoring-"));
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    expect(await deleteSession(home, session.id)).toMatchObject({ code: 0 });
    const targets = join(home, "cli", "targets");
    const partitions = await readdir(targets);
    expect(partitions).toHaveLength(1);
    const database = new DatabaseSync(join(targets, partitions[0]!, "authoring.db"));
    onTestFinished(() => database.close());
    return { database, home };
  }

  it("reports an expired deletion when the session still exists", async () => {
    const { database, home } = await openAuthoring();
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const limits = fixture.negotiated?.limits.idempotency;
    if (!limits) throw new Error("Runtime omitted its idempotency limits");
    const earlier = {
      commandId: "cli_0123456789abcdef0123456789abcdef",
      sessionId: session.id,
      replay: {
        type: "protected",
        namespace: limits.namespace,
        until: "2000-01-01T00:00:00Z",
      },
    };
    database
      .prepare("UPDATE authoring_records SET body = ? WHERE key = ?")
      .run(Buffer.from(JSON.stringify([earlier])), deletionJournal);

    const refusal = await deleteSession(home, session.id);
    await expect(fixture.client.sessions.get(asSessionId(session.id))).resolves.toMatchObject({
      id: session.id,
    });
    expect(refusal.code).toBe(1);
    expect(refusal.stdout).toBe("");
    expect(refusal.stderr).toContain("the session still exists");
    const retry = await deleteSession(home, session.id);
    expect(retry).toMatchObject({ code: 0, stdout: `${session.id}\n` });
  }, 30_000);

  it("prints a confirmed deletion when the authoring journal cannot be cleared", async () => {
    const { database, home } = await openAuthoring();
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    database.exec(`CREATE TRIGGER refuse_deletion_cleanup
      BEFORE UPDATE ON authoring_records
      WHEN new.key = '${deletionJournal}' AND CAST(new.body AS TEXT) = '[]'
      BEGIN SELECT RAISE(ABORT, 'local deletion cleanup refused'); END`);

    const deletion = await deleteSession(home, session.id);
    await expect(fixture.client.sessions.get(asSessionId(session.id))).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "session_not_found",
    );
    expect(deletion).toMatchObject({ code: 0, stdout: `${session.id}\n` });
    expect(deletion.stderr).toContain("local deletion cleanup refused");

    database.exec("DROP TRIGGER refuse_deletion_cleanup");
    const retry = await deleteSession(home, session.id);
    expect(retry).toMatchObject({ code: 0, stdout: `${session.id}\n` });
    expect(retry.stderr).not.toContain("local deletion cleanup refused");
  }, 30_000);
});
