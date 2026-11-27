import { lstat } from "node:fs/promises";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { RpcError, asRunId, asSegmentId, asSessionId } from "@flame/runtime-client";
import { createProviderGate } from "./fakeProvider";
import { collectRunEvents, originalOpeningEvents, within } from "./observation";
import { useRuntimeFixture } from "./fixture";

const unconfirmedModelOutcome = {
  type: "lost",
  loss: expect.objectContaining({ reason: "unresolvedEffects" }),
  unresolvedEffects: [
    expect.objectContaining({
      cause: "termination",
      detail: expect.stringContaining("model provider: exchange is unconfirmed"),
    }),
  ],
};

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: CLI", () => {
  const fixture = useRuntimeFixture("cli");

  it("completes an older Session by ID even when it is outside the first page", async () => {
    const title = "A Session older than the first catalog page";
    const workspace = { path: fixture.root };
    const older = await fixture.client.sessions.create({ title, workspace });
    const pageSize = 100;
    for (let index = 0; index < pageSize; index++) {
      await fixture.client.sessions.create({ title: `More recent Session ${index}`, workspace });
    }
    const first = await fixture.client.sessions.list({ limit: pageSize });
    expect(first.data).toHaveLength(pageSize);
    expect(first.data.some(({ id }) => id === older.id)).toBe(false);

    const prefix = older.id.slice(0, -4);
    const matching = await fixture.client.sessions.list({ idPrefix: prefix });
    expect(matching.data.map(({ id }) => id)).toEqual([older.id]);
    const differentlyCased = await fixture.client.sessions.list({ idPrefix: prefix.toUpperCase() });
    expect(differentlyCased.data).toEqual([]);
    const broad = await fixture.client.sessions.list({ idPrefix: older.id.slice(0, 1), limit: 1 });
    if (!broad.nextCursor) throw new Error("expected a continuation for the broader prefix");
    await expect(
      fixture.client.sessions.list({ idPrefix: prefix, cursor: broad.nextCursor }),
    ).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "invalid_params",
    );

    const command = fixture.startCLI(["__complete", "sessions", "show", prefix]);
    try {
      const result = await within(command.done, "session ID completion");
      expect(result.code).toBe(0);
      expect(result.stdout.split("\n")).toContain(`${older.id}\t${title}`);
    } finally {
      if (command.child.exitCode === null) command.child.kill("SIGKILL");
      await command.done;
    }
  }, 30_000);

  it("joins a CLI-owned run from independent views without canceling it when a view closes", async () => {
    const observer = fixture.createSharedClient();
    const retiringView = fixture.createSharedClient();
    const gate = createProviderGate("E2E_SHARED_CLI");
    fixture.providerGate = gate;
    const session = await observer.sessions.create({
      workspace: { path: fixture.root },
      title: "Shared CLI run",
    });
    const command = fixture.startCLI([
      "run",
      "--session",
      session.id,
      "--output-format",
      "streaming-json",
      "E2E_SHARED_CLI complete without tools.",
    ]);
    try {
      await within(
        Promise.race([
          gate.arrived.promise,
          command.done.then((result) => {
            throw new Error(`CLI exited before dispatch: ${result.code}: ${result.stderr}`);
          }),
        ]),
        "CLI provider dispatch",
      );
      const runs = await observer.runs.list({ sessionId: asSessionId(session.id) });
      expect(runs.data).toHaveLength(1);
      const run = runs.data[0];
      if (!run?.activeSegmentId) throw new Error("CLI did not publish an active segment");
      const runId = asRunId(run.id);
      const segmentId = asSegmentId(run.activeSegmentId);
      const attached = await retiringView.runs.subscribe({ runId, segmentId, snapshot: true });
      expect(attached.result.snapshot?.runs).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            id: run.id,
            activeSegmentId: run.activeSegmentId,
            status: "running",
          }),
        ]),
      );
      await retiringView.close();
      await expect(observer.runs.get(runId)).resolves.toMatchObject({
        status: "running",
        activeSegmentId: run.activeSegmentId,
      });
      await expect(
        observer.runs.start({
          sessionId: asSessionId(session.id),
          input: [{ type: "text", text: "must not duplicate the shared run" }],
        }),
      ).rejects.toSatisfy(
        (error: unknown) =>
          error instanceof RpcError && error.data.type === "session_has_active_run",
      );
      const successor = await observer.runs.subscribe({ runId, segmentId, snapshot: true });
      await observer.runs.steer(runId, segmentId, [
        { type: "text", text: "Shared view steering instruction." },
      ]);
      gate.release.resolve();
      const [events, result] = await within(
        Promise.all([collectRunEvents(successor.events), command.done]),
        "shared Run completion",
      );
      expect(result).toMatchObject({ code: 0 });
      expect(result.stdout).toContain(run.id);
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      await expect(
        observer.runs.list({ sessionId: asSessionId(session.id) }),
      ).resolves.toMatchObject({ data: [{ id: run.id, status: "finished" }] });
      await expect(
        lstat(join(fixture.environmentRoot, "cli-data", "runtime")),
      ).rejects.toMatchObject({
        code: "ENOENT",
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
      if (command.child.exitCode === null) command.child.kill("SIGKILL");
      await command.done;
      await retiringView.close();
      await observer.close();
    }
  }, 30_000);

  it("interrupts the one-shot CLI while preserving its unconfirmed Run and shared Runtime", async () => {
    const observer = fixture.createSharedClient();
    const gate = createProviderGate("E2E_SHARED_CANCEL");
    fixture.providerGate = gate;
    const session = await observer.sessions.create({ workspace: { path: fixture.root } });
    const command = fixture.startCLI([
      "run",
      "--session",
      session.id,
      "E2E_SHARED_CANCEL wait for interruption.",
    ]);
    try {
      await within(
        Promise.race([
          gate.arrived.promise,
          command.done.then((result) => {
            throw new Error(`CLI exited before interruption: ${result.code}: ${result.stderr}`);
          }),
        ]),
        "CLI dispatch before interruption",
      );
      expect(command.child.kill("SIGINT")).toBe(true);
      const result = await within(command.done, "interrupted CLI shutdown");
      expect(result).toMatchObject({ code: 130 });
      await within(gate.closed.promise, "canceled model request");
      await expect(
        observer.runs.list({ sessionId: asSessionId(session.id) }),
      ).resolves.toMatchObject({
        data: [{ status: "finished", outcome: unconfirmedModelOutcome }],
      });
      const next = await observer.sessions.create({
        workspace: { path: fixture.root },
        title: "Runtime remains available",
      });
      await expect(observer.sessions.get(asSessionId(next.id))).resolves.toMatchObject({
        id: next.id,
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
      if (command.child.exitCode === null) command.child.kill("SIGKILL");
      await command.done;
      await observer.close();
    }
  }, 30_000);

  it("keeps a CLI question after process exit and consumes one answer from competing views", async () => {
    const first = fixture.createSharedClient();
    const second = fixture.createSharedClient();
    const session = await first.sessions.create({
      workspace: { path: fixture.root },
      title: "Shared CLI waiting",
    });
    const command = fixture.startCLI([
      "run",
      "--session",
      session.id,
      "E2E_HITL ask before continuing.",
    ]);
    try {
      const result = await within(command.done, "CLI waiting exit");
      expect(result).toMatchObject({ code: 1 });
      const runs = await first.runs.list({ sessionId: asSessionId(session.id) });
      expect(runs.data).toHaveLength(1);
      const run = runs.data[0];
      if (!run) throw new Error("CLI did not publish its run");
      expect(run.status).toBe("waiting");
      const runId = asRunId(run.id);
      const pending = await first.interrupts.list({ rootRunId: runId });
      const question = pending.data[0]?.interrupts[0];
      if (!question || question.type !== "question")
        throw new Error("CLI question was not retained");
      const resume = {
        runId,
        responses: [
          { itemId: question.itemId, response: { type: "answer" as const, answers: [["Yes"]] } },
        ],
      };
      const attempts = await Promise.allSettled([
        first.runs.resume(resume),
        second.runs.resume(resume),
      ]);
      const accepted = attempts.filter((attempt) => attempt.status === "fulfilled");
      const rejected = attempts.filter((attempt) => attempt.status === "rejected");
      expect(accepted).toHaveLength(1);
      expect(rejected).toHaveLength(1);
      expect(rejected[0]?.reason).toSatisfy(
        (error: unknown) =>
          error instanceof RpcError &&
          ["interrupt_not_open", "session_busy"].includes(error.data.type),
      );
      const winner = accepted[0];
      if (!winner) throw new Error("no view acquired the waiting continuation");
      const events = await collectRunEvents(originalOpeningEvents(winner.value));
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      await expect(first.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({
        data: [],
      });
      await expect(first.runs.list({ sessionId: asSessionId(session.id) })).resolves.toMatchObject({
        data: [{ id: run.id, status: "finished" }],
      });
    } finally {
      if (command.child.exitCode === null) command.child.kill("SIGKILL");
      await command.done;
      await first.close();
      await second.close();
    }
  }, 30_000);
});
