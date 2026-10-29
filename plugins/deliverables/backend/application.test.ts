import { randomUUID } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { inflateRawSync } from "node:zlib";
import { afterEach, expect, it, vi } from "vitest";
import { canonicalJSON, digestJSON, type CallContext } from "@qimu/node";
import { Lifetime } from "@qimu/sdk";
import { PDFDocument } from "pdf-lib";
import { METHODS, schemas } from "../contracts/capability";
import { renderReportSchema, reviewReportSchema } from "../contracts/evidence";
import { Application, type Operation } from "./application";
import { Archive } from "./archive";
import { digest } from "./document";
import { Engine } from "./engine";
import { Formats } from "./format-process";
import { createFile, editFile, inspectFile } from "./formats";

const cleanup: Array<() => void | Promise<void>> = [];
afterEach(async () => {
  for (const close of cleanup.splice(0).reverse()) await close();
  vi.restoreAllMocks();
});

function context(
  input: unknown,
  id = randomUUID(),
  origin = "human",
): CallContext {
  const lifetime = new Lifetime("deliverables-application-test");
  cleanup.push(async () => {
    await lifetime.close();
  });
  return {
    signal: lifetime.signal,
    commandId: id,
    requestInput: canonicalJSON(input),
    inputDigest: digestJSON(input),
    invocation: {
      id: `invocation.${id}`,
      origin,
      target: { scope: "installation", reference: "deliverables.fixture" },
    },
    lifetime,
    defer: (close) => lifetime.cleanup(close),
  };
}

async function rendering(): Promise<Awaited<ReturnType<Engine["render"]>>> {
  const pdf = await PDFDocument.create();
  pdf.addPage([612, 792]);
  pdf.addPage([612, 792]);
  const png = Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6pV8AAAAASUVORK5CYII=",
    "base64",
  );
  return {
    report: {
      platform: "controlled-test-engine",
      engines: [{ name: "fixture", version: "1" }],
      pages: [
        { number: 1, filename: "page-1.png" },
        { number: 2, filename: "page-2.png" },
      ],
      checks: [
        {
          code: "rendered",
          severity: "info",
          message: "Controlled renderer fixture.",
        },
      ],
      extractedText: "Original claim",
      fontStatus: "unverified",
    },
    files: [
      {
        filename: "render.pdf",
        mime: "application/pdf",
        bytes: await pdf.save({ useObjectStreams: false }),
      },
      { filename: "page-1.png", mime: "image/png", bytes: png },
      { filename: "page-2.png", mime: "image/png", bytes: png },
    ],
  };
}

async function fixture() {
  const directory = await mkdtemp(
    join(tmpdir(), "flame-deliverables-application-"),
  );
  cleanup.push(() => rm(directory, { recursive: true, force: true }));
  const archive = new Archive(join(directory, "deliverables.db"));
  cleanup.push(() => archive.close());
  const engine = new Engine({}, directory);
  cleanup.push(() => engine.close());
  const formats = new Formats(join(directory, "controlled-format-entry.mjs"));
  cleanup.push(() => formats.close());
  vi.spyOn(formats, "create").mockImplementation((format, json) =>
    createFile(format, json),
  );
  vi.spyOn(formats, "edit").mockImplementation((format, bytes, json) =>
    editFile(format, bytes, json),
  );
  vi.spyOn(formats, "inspect").mockImplementation((format, bytes) =>
    inspectFile(format, bytes),
  );
  const rendered = await rendering();
  vi.spyOn(engine, "render").mockResolvedValue(rendered);
  const app = new Application(archive, engine, formats);
  function call(operation: Operation, input: Record<string, unknown>) {
    return app.command(context(input), operation, input);
  }
  const source = schemas.revision.parse(
    await call("create", {
      title: "Reviewed document",
      format: "docx",
      contentJSON: JSON.stringify({
        title: "Reviewed document",
        blocks: [{ type: "paragraph", runs: [{ text: "Original claim" }] }],
      }),
      provenance: {
        sources: [
          {
            title: "Fixture source",
            uri: "https://example.com/source",
            note: "Authored provenance.",
          },
        ],
        assumptions: ["Fixture assumption"],
        units: "USD",
        reproduction: "Controlled fixture",
      },
    }),
  );
  const selected = { id: source.documentId, revision: source.id };
  async function edit() {
    const item = (
      await inspectFile(
        "docx",
        archive.revision(source.documentId, source.id).bytes,
      )
    ).items.find((entry) => entry.text === "Original claim")!;
    return schemas.revision.parse(
      await call("edit", {
        id: source.documentId,
        expectedRevision: source.id,
        operationsJSON: JSON.stringify({
          operations: [
            {
              type: "replaceText",
              id: item.id,
              expect: item.text,
              text: "Revised claim",
            },
          ],
        }),
      }),
    );
  }
  return { app, archive, engine, rendered, call, source, selected, edit };
}

// This reader only inspects the bounded ZIPs produced by the test's export call.
// Native package admission and hostile ZIP handling have their own format tests.
function bundleEntries(bytes: Uint8Array): Map<string, Buffer> {
  const buffer = Buffer.from(bytes);
  const entries = new Map<string, Buffer>();
  let cursor = 0;
  while (buffer.readUInt32LE(cursor) === 0x04034b50) {
    expect(buffer.readUInt16LE(cursor + 6) & 8).toBe(0);
    expect(buffer.readUInt16LE(cursor + 8)).toBe(8);
    const compressed = buffer.readUInt32LE(cursor + 18);
    const length = buffer.readUInt16LE(cursor + 26);
    const start = cursor + 30 + length + buffer.readUInt16LE(cursor + 28);
    const name = buffer
      .subarray(cursor + 30, cursor + 30 + length)
      .toString("utf8");
    entries.set(
      name,
      inflateRawSync(buffer.subarray(start, start + compressed)),
    );
    cursor = start + compressed;
  }
  expect(buffer.readUInt32LE(cursor)).toBe(0x02014b50);
  return entries;
}

it("binds rendered assets and attributed visual review to the exact native revision", async () => {
  const { app, archive, engine, rendered, call, source, selected } =
    await fixture();
  const observation = schemas.assessment.parse(await call("render", selected));
  const report = renderReportSchema.parse(JSON.parse(observation.reportJSON));
  expect(engine.render).toHaveBeenCalledWith(
    "docx",
    archive.revision(source.documentId, source.id).bytes,
    expect.any(AbortSignal),
  );
  expect(observation).toMatchObject({
    revision: source.id,
    sourceSha256: source.sha256,
    kind: "render",
  });
  expect(report).toMatchObject({
    sourceRevision: source.id,
    sourceSha256: source.sha256,
    pdfSha256: digest(rendered.files[0]!.bytes),
    fontStatus: "unverified",
  });
  expect(report.pages.map((page) => page.number)).toEqual([1, 2]);
  for (const page of report.pages) {
    const preview = app.page(page.assetId);
    expect(digest(Buffer.from(preview.base64, "base64"))).toBe(page.sha256);
    expect(preview.metadata).toEqual(
      observation.assets.find((asset) => asset.id === page.assetId),
    );
  }
  const input = {
    ...selected,
    renderId: observation.id,
    pages: [2],
    verdict: "changes_requested",
    notes: "Second-page claim needs a source.",
  };
  const reviewContext = context(input, randomUUID(), "model");
  const review = schemas.assessment.parse(
    await app.command(reviewContext, "review", input),
  );
  expect(reviewReportSchema.parse(JSON.parse(review.reportJSON))).toEqual({
    sourceRevision: source.id,
    sourceSha256: source.sha256,
    renderId: observation.id,
    pages: [2],
    verdict: "changes_requested",
    notes: input.notes,
    reviewer: { origin: "model", invocationId: reviewContext.invocation!.id },
  });
  expect(
    app.query("assessment", { ...selected, assessmentId: review.id }),
  ).toEqual(review);
  expect(app.read(selected).assessments).toHaveLength(2);
  for (const pages of [[1, 1], [3]]) {
    await expect(call("review", { ...input, pages })).rejects.toMatchObject({
      code: "invalid_review",
    });
  }
  expect(app.read(selected).assessments).toHaveLength(2);
});

it("does not inherit renders or review observations when an edit advances the document", async () => {
  const { app, call, source, selected, edit } = await fixture();
  const render = schemas.assessment.parse(await call("render", selected));
  await call("review", {
    ...selected,
    renderId: render.id,
    pages: [1, 2],
    verdict: "accepted",
    notes: "Reviewed both fixture pages.",
  });
  const next = await edit();
  const current = app.read({ id: source.documentId });
  expect(current.revision.id).toBe(next.id);
  expect(current.assessments).toEqual([]);
  expect(
    app.read(selected).assessments.map((assessment) => assessment.kind),
  ).toEqual(["render", "review"]);
  await expect(
    call("export", { id: source.documentId, revision: next.id, kind: "pdf" }),
  ).rejects.toMatchObject({ code: "render_required" });
  await expect(
    call("review", {
      id: source.documentId,
      revision: next.id,
      renderId: render.id,
      pages: [1],
      verdict: "accepted",
      notes: "Old preview cannot review a changed claim.",
    }),
  ).rejects.toMatchObject({ code: "render_not_found" });
  expect(() =>
    app.query("assessment", {
      id: source.documentId,
      revision: next.id,
      assessmentId: render.id,
    }),
  ).toThrow("exact native revision");
  expect(app.read({ id: source.documentId }).assessments).toEqual([]);
});

it("exports authoritative native bytes, exact rendered PDF and a self-contained source-bound evidence bundle", async () => {
  const { app, archive, rendered, call, source, selected, edit } =
    await fixture();
  const render = schemas.assessment.parse(await call("render", selected));
  const review = schemas.assessment.parse(
    await call("review", {
      ...selected,
      renderId: render.id,
      pages: [1],
      verdict: "accepted",
      notes: "Reviewed the first page only.",
    }),
  );
  const next = await edit();
  const native = schemas.asset.parse(
    await call("export", { ...selected, kind: "native" }),
  );
  expect(native).toEqual(source.native);
  expect(archive.asset(native.id).bytes).toEqual(
    archive.revision(source.documentId, source.id).bytes,
  );
  const pdf = schemas.asset.parse(
    await call("export", { ...selected, kind: "pdf" }),
  );
  expect(pdf).toEqual(
    render.assets.find((asset) => asset.mime === "application/pdf"),
  );
  expect(archive.asset(pdf.id).bytes).toEqual(rendered.files[0]!.bytes);
  const bundled = schemas.asset.parse(
    await call("export", { ...selected, kind: "bundle" }),
  );
  const bytes = archive.asset(bundled.id).bytes;
  expect(digest(bytes)).toBe(bundled.sha256);
  const entries = bundleEntries(bytes);
  expect([...entries.keys()].sort()).toEqual(
    [
      "evidence.json",
      `native/${source.native.filename}`,
      ...rendered.files.map((file) => `render/${file.filename}`),
    ].sort(),
  );
  expect(entries.get(`native/${source.native.filename}`)).toEqual(
    Buffer.from(archive.revision(source.documentId, source.id).bytes),
  );
  for (const file of rendered.files)
    expect(entries.get(`render/${file.filename}`)).toEqual(
      Buffer.from(file.bytes),
    );
  const evidence = JSON.parse(entries.get("evidence.json")!.toString("utf8"));
  expect(evidence.revision).toEqual(source);
  expect(evidence.document.head).toBe(next.id);
  expect(evidence.assessments).toEqual([render, review]);
  expect(evidence.revision.provenance).toEqual(source.provenance);
  expect(evidence.processing).toBeNull();
  expect(app.read(selected).assessments).toHaveLength(2);
  expect(app.read({ id: source.documentId }).assessments).toEqual([]);
});

it("does not publish late renderer results after their source document was removed", async () => {
  const { app, archive, engine, rendered, call, source, selected } =
    await fixture();
  const started = Promise.withResolvers<void>();
  const pending =
    Promise.withResolvers<Awaited<ReturnType<Engine["render"]>>>();
  vi.mocked(engine.render).mockImplementationOnce(() => {
    started.resolve();
    return pending.promise;
  });
  const renderContext = context(selected);
  const result = app.command(renderContext, "render", selected);
  cleanup.push(async () => {
    pending.resolve(rendered);
    await result.catch(() => undefined);
  });
  await started.promise;
  await call("remove", { id: source.documentId, expectedRevision: source.id });
  pending.resolve(rendered);
  await expect(result).rejects.toMatchObject({ code: "revision_not_found" });
  expect(archive.assessments(source.documentId, source.id)).toEqual([]);
  expect(archive.list(undefined, 10).entries).toEqual([]);
  expect(
    archive.receipt(
      {
        id: renderContext.commandId!,
        method: METHODS.render,
        inputDigest: renderContext.inputDigest,
      },
      "command",
    ),
  ).toMatchObject({
    found: true,
    reply: { failure: { code: "revision_not_found" } },
  });
  await expect(
    app.command(renderContext, "render", selected),
  ).rejects.toMatchObject({ code: "revision_not_found" });
  expect(engine.render).toHaveBeenCalledTimes(1);
});

it("retains malformed renderer outcomes as unconfirmed without publishing partial evidence or replaying", async () => {
  const { app, archive, engine, rendered, selected } = await fixture();
  vi.mocked(engine.render).mockResolvedValueOnce({
    ...rendered,
    report: {
      ...rendered.report,
      pages: [{ number: 0, filename: "page-1.png" }],
    },
  });
  const renderContext = context(selected);
  await expect(
    app.command(renderContext, "render", selected),
  ).rejects.toMatchObject({ name: "ZodError" });
  expect(app.read(selected).assessments).toEqual([]);
  expect(
    archive.receipt(
      {
        id: renderContext.commandId!,
        method: METHODS.render,
        inputDigest: renderContext.inputDigest,
      },
      "command",
    ),
  ).toEqual({ found: false });
  await expect(app.command(renderContext, "render", selected)).rejects.toThrow(
    "unconfirmed",
  );
  expect(engine.render).toHaveBeenCalledTimes(1);
});
