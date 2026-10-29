import {
  act,
  cleanup,
  fireEvent,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import assert from "node:assert/strict";
import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import type { ComponentContext } from "@flame/component-sdk";
import {
  createFlameClient,
  createHttpTransport,
  type MutationJournalStorage,
} from "@flame/runtime-client";
import type { CallPluginRequest, ServerCapabilities } from "@flame/runtime-client/wire";
import { stringifyJSON } from "@qimu/sdk/json";
import { createTestClient } from "@/test/renderClient";
import { mockSelectLayout } from "@/test/selectLayout";
import { Deliverables } from "../../../../plugins/deliverables/component/component";
import { guidedContent } from "../../../../plugins/deliverables/component/authoring";
import { useDeliverables } from "../../../../plugins/deliverables/component/useDeliverables";
import { CAPABILITY_JSON } from "../../../../plugins/deliverables/component/methods.generated";
import { METHODS } from "../../../../plugins/deliverables/contracts/methods";
import {
  emptyProvenance,
  type Assessment,
  type DocumentRead,
  type Upload,
} from "../../../../plugins/deliverables/component/types";
import { encodeChunk, sha256 } from "../../../../plugins/deliverables/component/transfer";

const installationId = "12345678-1234-4234-9234-123456789abc";
const digest = "f".repeat(64);
const timestamp = "2026-10-29T00:00:00.000Z";
const doc1 = "1".repeat(64);
const doc2 = "2".repeat(64);
const rev1 = "a".repeat(64);
const rev2 = "b".repeat(64);
const newDocument = "3".repeat(64);
const newRevision = "d".repeat(64);

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

async function rig() {
  mockSelectLayout();
  const bytes = new TextEncoder().encode("fixture native bytes");
  const hash = await sha256(bytes);
  const asset = {
    id: "c".repeat(64),
    sha256: hash,
    size: bytes.length,
    mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    filename: "report.docx",
  };
  const make = (id: string, revision: string, title: string): DocumentRead => ({
    document: { id, head: revision, format: "docx", title, createdAt: timestamp },
    revision: {
      id: revision,
      documentId: id,
      operation: "create",
      format: "docx",
      sha256: hash,
      size: bytes.length,
      createdAt: timestamp,
      native: asset,
      provenance: emptyProvenance(),
    },
    inspectionJSON: JSON.stringify({
      format: "docx",
      title,
      items: [{ id: "word/document.xml#text:0", kind: "text", text: title }],
      facts: { paragraphs: 1 },
      checks: [],
      truncated: false,
    }),
    assessments: [],
  });
  const documents = new Map([
    [doc1, make(doc1, rev1, "Quarterly report")],
    [doc2, make(doc2, rev2, "Meeting notes")],
  ]);
  const historicalRevision = (document: DocumentRead) => ({
    ...document.revision,
    id: (document.document.id === doc1 ? "e" : "f").repeat(64),
    operation: "edit" as const,
  });
  const assessments = new Map<string, Assessment>();
  const uploads = new Map<string, Upload>();
  const assets = new Map([[asset.id, { asset, bytes }]]);
  const originals = new Map<string, CallPluginRequest>();
  const results = new Map<string, unknown>();
  const calls: Array<{ method: string; input: Record<string, unknown>; key: string | null }> = [];
  let unknownCreate = false;
  let failedReads = false;
  let paged: "list" | "history" | undefined;
  let failedPage = false;
  let creations = 0;
  const holds = new Map<
    string,
    { gate: PromiseWithResolvers<void>; arrived: PromiseWithResolvers<void> }
  >();
  async function waitForRead(key: string) {
    const hold = holds.get(key);
    if (!hold) return;
    holds.delete(key);
    hold.arrived.resolve();
    await hold.gate.promise;
  }
  const fetch = vi.fn<typeof globalThis.fetch>(async (_url, init) => {
    const request = JSON.parse(init!.body as string) as {
      id: string;
      method: string;
      params: CallPluginRequest;
    };
    const params = request.params;
    const key = new Headers(init!.headers).get("Idempotency-Key");
    const success = (result: unknown) => Response.json({ jsonrpc: "2.0", id: request.id, result });
    const output = (result: unknown) => success({ output: stringifyJSON(result) });
    const uncertain = () =>
      Response.json({
        jsonrpc: "2.0",
        id: request.id,
        error: {
          code: -32603,
          message: "internal_error",
          data: { type: "internal_error", detail: "fixture acknowledgement unavailable" },
        },
      });
    if (request.method === "plugins.getCommand")
      return success(originals.get((params as unknown as { key: string }).key));
    if (request.method !== "plugins.call") throw new Error("unexpected fixture Runtime operation");
    const input = JSON.parse(params.input) as Record<string, unknown>;
    calls.push({ method: params.method, input, key });
    if (params.method === METHODS.catalog)
      return output({
        formatsJSON: JSON.stringify({
          formats: [
            {
              format: "docx",
              createSchemaJSON: "{}",
              editSchemaJSON: "{}",
              exampleJSON:
                '{"title":"Example","blocks":[{"type":"paragraph","runs":[{"text":"Example"}]}]}',
              limitations: [],
            },
            {
              format: "xlsx",
              createSchemaJSON: "{}",
              editSchemaJSON: "{}",
              exampleJSON:
                '{"title":"Example","sheets":[{"name":"Data","cells":[{"address":"A1","value":{"type":"number","value":1}}]}]}',
              limitations: [],
            },
          ],
        }),
        enginesJSON: JSON.stringify({
          platform: "linux",
          configured: false,
          configuration: {},
          limitations: [],
        }),
        limitsJSON: "{}",
      });
    if (params.method === METHODS.list) {
      await waitForRead(`list:${input.before ?? ""}`);
      if (paged === "list") {
        if (input.before === "next")
          return failedPage ? uncertain() : output({ entries: [documents.get(doc2)!.document] });
        assert.equal(input.before, undefined);
        return output({ entries: [documents.get(doc1)!.document], next: "next" });
      }
      return output({ entries: [...documents.values()].map((value) => value.document) });
    }
    if (params.method === METHODS.uploads) return output([...uploads.values()]);
    if (params.method === METHODS.assessment)
      return output(assessments.get(input.assessmentId as string));
    if (params.method === METHODS.history) {
      await waitForRead(`history:${input.before ?? ""}`);
      const document = documents.get(input.id as string)!;
      const revision = document.revision;
      if (paged === "history") {
        if (input.before === "next")
          return failedPage ? uncertain() : output({ entries: [historicalRevision(document)] });
        assert.equal(input.before, undefined);
        return output({ entries: [revision], next: "next" });
      }
      return output({ entries: [revision] });
    }
    if (params.method === METHODS.read) {
      const captured = structuredClone(documents.get(input.id as string));
      if (captured && input.revision === historicalRevision(captured).id)
        captured.revision = historicalRevision(captured);
      await waitForRead(input.id as string);
      if (failedReads) return uncertain();
      return output(captured);
    }
    if (params.method === METHODS.download) {
      const captured = assets.get(input.assetId as string)!;
      const offset = Number(input.offset);
      const next = Math.min(captured.bytes.length, offset + Number(input.length));
      return output({
        asset: captured.asset,
        offset,
        next,
        done: next === captured.bytes.length,
        base64: encodeChunk(captured.bytes.slice(offset, next)),
      });
    }
    if (!key) throw new Error("fixture command has no identity");
    const prior = originals.get(key);
    if (prior) {
      assert.deepEqual(params, prior);
      if (unknownCreate && params.method === METHODS.create) return uncertain();
      return output(results.get(key));
    }
    originals.set(key, structuredClone(params));
    if (params.method === METHODS.create) {
      creations++;
      const document = make(newDocument, newRevision, input.title as string);
      documents.set(newDocument, document);
      results.set(key, document.revision);
      if (unknownCreate) return uncertain();
      return output(document.revision);
    }
    if (params.method === METHODS.export) {
      results.set(key, asset);
      return output(asset);
    }
    if (params.method === METHODS.review) {
      const value: Assessment = {
        id: "9".repeat(64),
        revision: input.revision as string,
        sourceSha256: hash,
        createdAt: timestamp,
        kind: "review",
        assets: [],
        reportJSON: JSON.stringify({
          sourceRevision: input.revision,
          sourceSha256: hash,
          renderId: input.renderId,
          pages: input.pages,
          verdict: input.verdict,
          notes: input.notes,
          reviewer: { origin: "human", invocationId: key },
        }),
      };
      assessments.set(value.id, value);
      const { reportJSON: _, ...header } = value;
      documents.get(input.id as string)!.assessments.push(header);
      results.set(key, value);
      return output(value);
    }
    if (params.method === METHODS.append) {
      const upload = uploads.get(input.uploadId as string)!;
      assert.equal(input.offset, upload.received);
      const next = {
        ...upload,
        received: Number(upload.received) + atob(input.base64 as string).length,
      };
      uploads.set(upload.id, next);
      results.set(key, next);
      return output(next);
    }
    if (params.method === METHODS.import) {
      const upload = uploads.get(input.uploadId as string)!;
      assert.equal(upload.received, upload.size);
      uploads.delete(upload.id);
      const document = make(newDocument, newRevision, input.title as string);
      documents.set(newDocument, document);
      results.set(key, document.revision);
      return output(document.revision);
    }
    throw new Error("unexpected fixture command " + params.method);
  });
  const journal = new Map<string, unknown>();
  const storage: MutationJournalStorage = {
    get: (key) => structuredClone(journal.get(key)),
    set: (key, value) => {
      journal.set(key, structuredClone(value));
    },
    remove: (key) => {
      journal.delete(key);
    },
    keys: () => [...journal.keys()],
  };
  const client = createFlameClient(
    createHttpTransport({ baseUrl: "https://runtime.test", fetch }),
    {
      mutationStorage: storage,
      capabilities: () =>
        ({
          features: { plugins: { enabled: true } },
          limits: { idempotency: { namespace: "idp_" + "1".repeat(32), retentionSeconds: 3600 } },
        }) as unknown as ServerCapabilities,
    },
  );
  const ui = createTestClient({ start: false, runtimeClient: () => client });
  const controllers: AbortController[] = [];
  const gates: Array<PromiseWithResolvers<void>> = [];
  const saveFile = vi.fn<ComponentContext["saveFile"]>(async () => "requested");
  function context(): ComponentContext {
    const controller = new AbortController();
    controllers.push(controller);
    return {
      component: { installationId, digest, componentId: "deliverables" },
      binding: { installationId, digest, contribution: "deliverables", contract: CAPABILITY_JSON },
      locale: "en",
      runtime: client,
      signal: controller.signal,
      track: (factory) => factory(controller.signal),
      telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear: () => undefined },
      saveFile,
      compose: () => {
        throw new Error("fixture does not compose");
      },
    };
  }
  onTestFinished(async () => {
    gates.forEach((gate) => gate.resolve());
    controllers.forEach((controller) => controller.abort());
    await client.close();
    await ui.dispose();
  });
  return {
    context,
    ui,
    client,
    calls,
    documents,
    originals,
    saveFile,
    bytes,
    creations: () => creations,
    unknown: (value: boolean) => {
      unknownCreate = value;
    },
    failReads: (value: boolean) => {
      failedReads = value;
    },
    paginate(kind: "list" | "history", fail: boolean) {
      paged = kind;
      failedPage = fail;
    },
    stage() {
      const upload = { id: "e".repeat(64), sha256: hash, size: bytes.length, received: 8 };
      uploads.set(upload.id, upload);
      return upload;
    },
    async render() {
      const png = Uint8Array.from(
        atob(
          "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==",
        ),
        (value) => value.charCodeAt(0),
      );
      const image = {
        id: "7".repeat(64),
        sha256: await sha256(png),
        size: png.length,
        mime: "image/png",
        filename: "page-1.png",
      };
      assets.set(image.id, { asset: image, bytes: png });
      const value: Assessment = {
        id: "6".repeat(64),
        revision: rev1,
        sourceSha256: hash,
        createdAt: timestamp,
        kind: "render",
        assets: [image],
        reportJSON: JSON.stringify({
          sourceRevision: rev1,
          sourceSha256: hash,
          pdfSha256: hash,
          platform: "linux",
          engines: [{ name: "fixture", version: "1" }],
          pages: [{ number: 1, assetId: image.id, sha256: image.sha256 }],
          checks: [],
          extractedText: "Rendered page",
          fontStatus: "unverified",
        }),
      };
      assessments.set(value.id, value);
      const { reportJSON: _, ...header } = value;
      documents.get(doc1)!.assessments.push(header);
      return value;
    },
    hold(id: string) {
      const gate = Promise.withResolvers<void>();
      const arrived = Promise.withResolvers<void>();
      gates.push(gate);
      holds.set(id, { gate, arrived });
      return { arrived: arrived.promise, release: gate.resolve };
    },
  };
}

it("retires old reads when another exact revision is selected", async () => {
  const fixture = await rig();
  const context = fixture.context();
  const hook = renderHook(() => useDeliverables(context));
  await waitFor(() => expect(hook.result.current.list?.value?.entries).toHaveLength(2));
  const held = fixture.hold(doc1);
  act(() => hook.result.current.open({ id: doc1, revision: rev1 }));
  await held.arrived;
  act(() => hook.result.current.open({ id: doc2, revision: rev2 }));
  await waitFor(() => expect(hook.result.current.read?.value?.revision.id).toBe(rev2));
  await act(async () => {
    held.release();
    await Promise.resolve();
  });
  expect(hook.result.current.read?.value?.document.id).toBe(doc2);
  expect(hook.result.current.selection).toEqual({ id: doc2, revision: rev2 });
});

it("recovers the same uncertain command after remount without creating another native revision", async () => {
  const fixture = await rig();
  fixture.unknown(true);
  const firstContext = fixture.context();
  const first = renderHook(() => useDeliverables(firstContext));
  const input = {
    title: "Recovered report",
    format: "docx" as const,
    contentJSON: '{"title":"Recovered"}',
    provenance: emptyProvenance(),
  };
  await act(async () => {
    await first.result.current.run(METHODS.create, input);
  });
  expect(first.result.current.commands[0]?.status).toBe("unknown");
  const original = first.result.current.commands[0]!.id;
  await act(async () => {
    await first.result.current.run(METHODS.create, input);
  });
  expect(fixture.creations()).toBe(1);
  first.unmount();
  const nextContext = fixture.context();
  const next = renderHook(() => useDeliverables(nextContext));
  await waitFor(() => expect(next.result.current.commands[0]?.id).toBe(original));
  fixture.unknown(false);
  act(() => next.result.current.recover(next.result.current.commands[0]!));
  await waitFor(() => expect(next.result.current.commands).toHaveLength(0));
  expect(fixture.creations()).toBe(1);
  const creationCalls = fixture.calls.filter((call) => call.method === METHODS.create);
  expect(creationCalls).toHaveLength(2);
  expect(new Set(creationCalls.map((call) => call.key))).toEqual(new Set([original]));
  expect(next.result.current.notice).toMatchObject({
    kind: "command",
    revision: { id: newRevision },
  });
});

it("preserves the selected revision and draft when a refresh fails", async () => {
  const fixture = await rig();
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(await screen.findByRole("button", { name: /Quarterly report/ }));
  await screen.findByText("Current native revision");
  fireEvent.click(
    within(screen.getByRole("group", { name: "Deliverable actions" })).getByRole("button", {
      name: "Create a deliverable",
    }),
  );
  const body = await screen.findByRole("textbox", { name: "Document text" });
  const form = body.closest("form")!;
  fireEvent.change(within(form).getByRole("textbox", { name: "Title" }), {
    target: { value: "Unsaved title" },
  });
  fireEvent.change(body, { target: { value: "Keep this original draft." } });
  fixture.failReads(true);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() =>
    expect(fixture.calls.filter((call) => call.method === METHODS.read)).toHaveLength(2),
  );
  await waitFor(() =>
    expect(screen.getAllByRole("note").map((note) => note.textContent)).toEqual(
      expect.arrayContaining([expect.stringContaining("fixture acknowledgement unavailable")]),
    ),
  );
  expect((body as HTMLTextAreaElement).value).toBe("Keep this original draft.");
  expect((within(form).getByRole("textbox", { name: "Title" }) as HTMLInputElement).value).toBe(
    "Unsaved title",
  );
  expect(screen.getByText("Current native revision")).toBeTruthy();
});

it("saves verified bytes through the host and reports browser request rather than disk completion", async () => {
  const fixture = await rig();
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(await screen.findByRole("button", { name: /Quarterly report/ }));
  fireEvent.click(await screen.findByRole("button", { name: "Save native file" }));
  await waitFor(() => expect(fixture.saveFile).toHaveBeenCalledTimes(1));
  expect(fixture.saveFile.mock.calls[0]![0]).toEqual({
    filename: "report.docx",
    mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    bytes: fixture.bytes,
  });
  await screen.findByText(
    "Download requested for report.docx. The browser cannot confirm a disk write.",
  );
  const exported = fixture.calls.find((call) => call.method === METHODS.export)!;
  expect(exported.input).toEqual({ id: doc1, revision: rev1, kind: "native" });
});

it("binds visual review to the loaded page and revokes its preview on revision navigation", async () => {
  const fixture = await rig();
  const render = await fixture.render();
  const createURL = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:fixture-page");
  const revokeURL = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(await screen.findByRole("button", { name: /Quarterly report/ }));
  const image = await screen.findByRole("img", { name: "Page 1" });
  fireEvent.click(screen.getByRole("button", { name: "Record visual review" }));
  const add = screen.getByRole("button", { name: "Add page 1 to review" });
  expect((add as HTMLButtonElement).disabled).toBe(true);
  fireEvent.load(image);
  fireEvent.click(add);
  fireEvent.change(screen.getByRole("textbox", { name: "Observations and remaining issues" }), {
    target: { value: "The heading clips at the right edge." },
  });
  fireEvent.click(screen.getByRole("button", { name: "Record review" }));
  await screen.findByText("Your review is recorded against the exact source and render.");
  expect(fixture.calls.find((call) => call.method === METHODS.review)?.input).toEqual({
    id: doc1,
    revision: rev1,
    renderId: render.id,
    pages: [1],
    verdict: "changes_requested",
    notes: "The heading clips at the right edge.",
  });
  expect(createURL).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: /Meeting notes/ }));
  await waitFor(() => expect(revokeURL).toHaveBeenCalledWith("blob:fixture-page"));
  expect(screen.queryByRole("img", { name: "Page 1" })).toBeNull();
});

it("resumes an imported file from the canonical staged offset", async () => {
  const fixture = await rig();
  fixture.stage();
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(
    within(screen.getByRole("group", { name: "Deliverable actions" })).getByRole("button", {
      name: "Import native file",
    }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "Resume this upload" }));
  const file = new File([fixture.bytes], "original.docx", {
    type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  });
  fireEvent.change(screen.getByLabelText("Choose native file"), { target: { files: [file] } });
  await screen.findByText("original.docx");
  fireEvent.click(screen.getByRole("button", { name: "Resume and import" }));
  await screen.findByText("The original file was imported as a new revision.");
  const appended = fixture.calls.filter((call) => call.method === METHODS.append);
  expect(appended).toHaveLength(1);
  expect(appended[0]!.input.offset).toBe(8);
  expect(atob(appended[0]!.input.base64 as string)).toBe(
    new TextDecoder().decode(fixture.bytes.slice(8)),
  );
  expect(fixture.calls.filter((call) => call.method === METHODS.upload)).toHaveLength(0);
});

async function chooseOption(label: string, option: string | RegExp) {
  fireEvent.click(screen.getByRole("combobox", { name: label }));
  const item = await screen.findByRole("option", { name: option });
  fireEvent.pointerDown(item, { pointerType: "mouse" });
  fireEvent.click(item);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
}

it("refuses lossy guided workbook numbers before they become native cells", () => {
  expect(guidedContent("xlsx", "Numbers", "12.5\t00123\t=SUM(A1:A2)", "Data")).toMatchObject({
    sheets: [
      {
        cells: [
          { value: { type: "number", value: 12.5 } },
          { value: { type: "string", value: "00123" } },
          { value: { type: "formula", value: "=SUM(A1:A2)" } },
        ],
      },
    ],
  });
  for (const value of ["9007199254740993", "1.0000000000000001", "1e999"])
    expect(() => guidedContent("xlsx", "Numbers", value, "Data")).toThrow("exactly representable");
});

it("refuses duplicate keys and lossy numbers in advanced creation before dispatch", async () => {
  const fixture = await rig();
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(
    within(screen.getByRole("group", { name: "Deliverable actions" })).getByRole("button", {
      name: "Create a deliverable",
    }),
  );
  await chooseOption("Format", "Workbook · XLSX");
  await chooseOption("Authoring mode", "Advanced JSON editor");
  const source = screen.getByRole("textbox", { name: "Native content specification (JSON)" });
  const form = source.closest("form")!;
  fireEvent.change(within(form).getByRole("textbox", { name: "Title" }), {
    target: { value: "Exact numbers" },
  });
  const template =
    '{"title":"Numbers","sheets":[{"name":"Data","cells":[{"address":"A1","value":{"type":"number","value":NUMBER}}]}]}';
  for (const value of ["9007199254740993", "1.0000000000000001"]) {
    fireEvent.change(source, { target: { value: template.replace("NUMBER", value) } });
    fireEvent.submit(form);
    await screen.findByText(/number is not exactly representable/);
  }
  fireEvent.change(source, {
    target: {
      value: template
        .replace('"title":"Numbers"', '"title":"First","title":"Second"')
        .replace("NUMBER", "1"),
    },
  });
  fireEvent.submit(form);
  await screen.findByText(/duplicate JSON member/);
  expect(fixture.calls.filter((call) => call.method === METHODS.create)).toHaveLength(0);
});

it("refuses lossy numeric replacements in both item and advanced edit forms", async () => {
  const fixture = await rig();
  const document = fixture.documents.get(doc1)!;
  document.document.format = "xlsx";
  document.revision.format = "xlsx";
  const item = {
    id: "xl/worksheets/sheet1.xml!A1",
    kind: "cell",
    text: '{"type":"number","value":1}',
  };
  document.inspectionJSON = JSON.stringify({
    format: "xlsx",
    title: "Numbers",
    items: [item],
    facts: {},
    checks: [],
    truncated: false,
  });
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(await screen.findByRole("button", { name: /Quarterly report/ }));
  fireEvent.click(await screen.findByRole("button", { name: "Edit this revision" }));
  await chooseOption("Item to edit", /^xl\/worksheets\/sheet1.xml!A1/);
  const replacement = screen.getByRole("textbox", { name: "Replacement value" });
  fireEvent.change(replacement, { target: { value: "9007199254740993" } });
  fireEvent.submit(replacement.closest("form")!);
  await screen.findByText(/number is not exactly representable/);
  await chooseOption("Editing mode", "Advanced JSON editor");
  const operations = screen.getByRole("textbox", { name: "Local edit operations (JSON)" });
  fireEvent.change(operations, {
    target: {
      value: JSON.stringify({
        operations: [
          {
            type: "setCell",
            id: item.id,
            expect: item.text,
            value: { type: "number", value: "NUMBER" },
          },
        ],
      }).replace('"NUMBER"', "1.0000000000000001"),
    },
  });
  fireEvent.submit(operations.closest("form")!);
  await screen.findByText(/number is not exactly representable/);
  expect(fixture.calls.filter((call) => call.method === METHODS.edit)).toHaveLength(0);
});

it.each([
  { kind: "list", fail: false, later: false },
  { kind: "list", fail: false, later: true },
  { kind: "list", fail: true, later: false },
  { kind: "list", fail: true, later: true },
  { kind: "history", fail: false, later: false },
  { kind: "history", fail: false, later: true },
  { kind: "history", fail: true, later: false },
  { kind: "history", fail: true, later: true },
] as const)(
  "keeps $kind page focus through settlement (failure=$fail, later focus=$later)",
  async ({ kind, fail, later }) => {
    const fixture = await rig();
    fixture.paginate(kind, fail);
    fixture.ui.render(<Deliverables context={fixture.context()} />);
    const first = await screen.findByRole("button", { name: /Quarterly report/ });
    if (kind === "history") fireEvent.click(first);
    const pager = within(
      await screen.findByRole("group", {
        name: kind === "list" ? "Deliverable pages" : "Revision history pages",
      }),
    );
    const next = pager.getByRole<HTMLButtonElement>("button", { name: "Next" });
    const previous = pager.getByRole<HTMLButtonElement>("button", { name: "Previous" });
    const held = fixture.hold(kind + ":next");
    act(() => next.focus());
    fireEvent.click(next);
    await held.arrived;
    expect(next.matches(":disabled")).toBe(false);
    expect(next.getAttribute("aria-disabled")).toBe("true");
    expect(next.getAttribute("aria-busy")).toBe("true");
    expect(document.activeElement).toBe(next);
    const method = kind === "list" ? METHODS.list : METHODS.history;
    const readCount = () => fixture.calls.filter((call) => call.method === method).length;
    const calls = readCount();
    fireEvent.click(next);
    fireEvent.click(previous);
    fireEvent.keyDown(next, { key: "Enter" });
    fireEvent.keyDown(next, { key: " " });
    fireEvent.keyUp(next, { key: " " });
    expect(readCount()).toBe(calls);
    const search = screen.getByRole("searchbox", { name: "Search this page of titles" });
    if (later) act(() => search.focus());
    await act(async () => {
      held.release();
    });
    await waitFor(() => expect(next.getAttribute("aria-busy")).not.toBe("true"));
    expect(next.matches(":disabled")).toBe(false);
    expect(next.getAttribute("aria-disabled")).toBe(fail ? "false" : "true");
    expect(document.activeElement).toBe(later ? search : next);
    expect(readCount()).toBe(calls);
    expect(Boolean(screen.queryByRole("button", { name: /Meeting notes/ }))).toBe(
      kind === "history" || !fail,
    );
    if (!fail) {
      fireEvent.click(next);
      fireEvent.keyDown(next, { key: "Enter" });
      fireEvent.keyDown(next, { key: " " });
      fireEvent.keyUp(next, { key: " " });
    }
    expect(readCount()).toBe(calls);
    fireEvent.click(previous);
    await waitFor(() => expect(previous.getAttribute("aria-disabled")).toBe("true"));
    expect(previous.matches(":disabled")).toBe(false);
    expect(next.getAttribute("aria-disabled")).toBe("false");
    expect(readCount()).toBe(calls + Number(!fail));
  },
);

it("keeps refresh focus while the overview rereads and preserves later focus through a refused revision read", async () => {
  const fixture = await rig();
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(await screen.findByRole("button", { name: /Quarterly report/ }));
  await screen.findByText("Current native revision");
  const refresh = screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });
  const list = fixture.hold("list:");
  const read = fixture.hold(doc1);
  act(() => refresh.focus());
  fireEvent.click(refresh);
  await list.arrived;
  await read.arrived;
  expect(refresh.matches(":disabled")).toBe(false);
  expect(refresh.getAttribute("aria-busy")).toBe("true");
  expect(document.activeElement).toBe(refresh);
  const calls = () => fixture.calls.filter((call) => call.method === METHODS.list).length;
  const count = calls();
  fireEvent.click(refresh);
  fireEvent.keyDown(refresh, { key: "Enter" });
  fireEvent.keyDown(refresh, { key: " " });
  fireEvent.keyUp(refresh, { key: " " });
  expect(calls()).toBe(count);
  await act(async () => list.release());
  await waitFor(() => expect(refresh.getAttribute("aria-busy")).not.toBe("true"));
  expect(document.activeElement).toBe(refresh);
  expect(refresh.matches(":disabled")).toBe(false);
  expect(refresh.getAttribute("aria-disabled")).not.toBe("true");
  const search = screen.getByRole("searchbox", { name: "Search this page of titles" });
  act(() => search.focus());
  fixture.failReads(true);
  await act(async () => read.release());
  await screen.findByText(/fixture acknowledgement unavailable/);
  expect(document.activeElement).toBe(search);
  expect(fixture.calls.filter((call) => call.key !== null)).toHaveLength(0);
});

it("retains a history page only for its document and rejects its late response after switching", async () => {
  const fixture = await rig();
  fixture.paginate("history", false);
  const context = fixture.context();
  const hook = renderHook(() => useDeliverables(context));
  act(() => hook.result.current.open({ id: doc1, revision: rev1 }));
  await waitFor(() => expect(hook.result.current.history?.value?.next).toBe("next"));
  const held = fixture.hold("history:next");
  act(() => {
    void hook.result.current.page("history", "next");
  });
  await held.arrived;
  expect(hook.result.current.history?.loading).toBe(true);
  expect(hook.result.current.history?.value?.entries[0]?.id).toBe(rev1);
  act(() => hook.result.current.open({ id: doc2, revision: rev2 }));
  expect(hook.result.current.history?.value).toBeUndefined();
  await waitFor(() => expect(hook.result.current.history?.value?.entries[0]?.id).toBe(rev2));
  await act(async () => held.release());
  expect(hook.result.current.history?.value?.entries[0]?.id).toBe(rev2);
  expect(hook.result.current.historyPrevious).toBe(false);
  expect(hook.result.current.selection).toEqual({ id: doc2, revision: rev2 });
});

it("keeps the selected revision recognizable and selectable across history pages", async () => {
  const fixture = await rig();
  fixture.paginate("history", false);
  fixture.ui.render(<Deliverables context={fixture.context()} />);
  fireEvent.click(await screen.findByRole("button", { name: /Quarterly report/ }));
  const select = await screen.findByRole("combobox", { name: "Choose exact revision" });
  const currentLabel = `${timestamp} · Created · ${rev1.slice(0, 12)}`;
  const historicalLabel = `${timestamp} · Locally edited · ${"e".repeat(12)}`;
  await waitFor(() => expect(select.textContent).toBe(currentLabel));
  const pager = within(await screen.findByRole("group", { name: "Revision history pages" }));
  fireEvent.click(pager.getByRole("button", { name: "Next" }));
  await waitFor(() =>
    expect(pager.getByRole("button", { name: "Previous" }).getAttribute("aria-disabled")).toBe(
      "false",
    ),
  );
  expect(select.textContent).toBe(currentLabel);
  fireEvent.click(select);
  expect(
    (await screen.findByRole("option", { name: currentLabel })).getAttribute("aria-selected"),
  ).toBe("true");
  expect(screen.getAllByRole("option")).toHaveLength(2);
  fireEvent.pointerDown(screen.getByRole("option", { name: historicalLabel }), {
    pointerType: "mouse",
  });
  fireEvent.click(screen.getByRole("option", { name: historicalLabel }));
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  await waitFor(() => expect(select.textContent).toBe(historicalLabel));
  await screen.findByText("Historical native revision");
  expect(fixture.calls.filter((call) => call.method === METHODS.read).at(-1)?.input).toEqual({
    id: doc1,
    revision: "e".repeat(64),
  });
  fireEvent.click(pager.getByRole("button", { name: "Previous" }));
  await waitFor(() =>
    expect(pager.getByRole("button", { name: "Previous" }).getAttribute("aria-disabled")).toBe(
      "true",
    ),
  );
  expect(select.textContent).toBe(historicalLabel);
  fireEvent.click(select);
  expect(
    (await screen.findByRole("option", { name: historicalLabel })).getAttribute("aria-selected"),
  ).toBe("true");
  expect(screen.getAllByRole("option")).toHaveLength(2);
  fireEvent.pointerDown(screen.getByRole("option", { name: currentLabel }), {
    pointerType: "mouse",
  });
  fireEvent.click(screen.getByRole("option", { name: currentLabel }));
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  await waitFor(() => expect(select.textContent).toBe(currentLabel));
  fireEvent.click(screen.getByRole("button", { name: /Meeting notes/ }));
  await waitFor(() =>
    expect(select.textContent).toBe(`${timestamp} · Created · ${rev2.slice(0, 12)}`),
  );
  fireEvent.click(select);
  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.queryByRole("option", { name: currentLabel })).toBeNull();
  expect(screen.queryByRole("option", { name: historicalLabel })).toBeNull();
  expect(fixture.calls.filter((call) => call.key !== null)).toHaveLength(0);
});

it.each([
  { fail: false, focus: "origin" },
  { fail: false, focus: "later" },
  { fail: false, focus: "outside" },
  { fail: true, focus: "origin" },
  { fail: true, focus: "later" },
  { fail: true, focus: "outside" },
] as const)(
  "keeps a local reading entry when opening the current revision (failure=$fail, focus=$focus)",
  async ({ fail, focus }) => {
    const fixture = await rig();
    fixture.paginate("history", false);
    fixture.ui.render(<Deliverables context={fixture.context()} />);
    fireEvent.click(await screen.findByRole("button", { name: /Quarterly report/ }));
    const pager = within(await screen.findByRole("group", { name: "Revision history pages" }));
    fireEvent.click(pager.getByRole("button", { name: "Next" }));
    await waitFor(() =>
      expect(pager.getByRole("button", { name: "Previous" }).getAttribute("aria-disabled")).toBe(
        "false",
      ),
    );
    await chooseOption("Choose exact revision", /eeeeeeeeeeee/);
    await screen.findByText("Historical native revision");
    const jump = screen.getByRole<HTMLButtonElement>("button", { name: "Open current revision" });
    const select = screen.getByRole("combobox", { name: "Choose exact revision" });
    const entry = select.closest("section")!;
    const search = screen.getByRole("searchbox", { name: "Search this page of titles" });
    fixture.failReads(fail);
    const held = fixture.hold(doc1);
    act(() => (focus === "outside" ? search : jump).focus());
    fireEvent.click(jump);
    await held.arrived;
    expect(document.activeElement).toBe(focus === "outside" ? search : entry);
    expect(screen.getByRole("region", { name: "Revision history" })).toBe(entry);
    expect(entry.tabIndex).toBe(-1);
    if (focus === "later") act(() => search.focus());
    await act(async () => held.release());
    await waitFor(() => expect(screen.queryByText("Reading…")).toBeNull());
    expect(document.activeElement).toBe(focus === "origin" ? entry : search);
    expect(Boolean(screen.queryByText("Current native revision"))).toBe(!fail);
    expect(Boolean(screen.queryByText(/fixture acknowledgement unavailable/))).toBe(fail);
    expect(fixture.calls.filter((call) => call.method === METHODS.read).at(-1)?.input).toEqual({
      id: doc1,
      revision: rev1,
    });
    expect(fixture.calls.filter((call) => call.key !== null)).toHaveLength(0);
  },
);
