import mcpProtocol from "@flame/backend-sdk/mcp/protocol" with { type: "json" };
import protocol from "@flame/backend-sdk/protocol" with { type: "json" };
import { rolldown } from "rolldown";
import { buildBackend } from "@flame/backend-sdk/build";
import { randomUUID } from "node:crypto";
import { mkdtemp, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterAll, afterEach, beforeAll, beforeEach, expect, it } from "vitest";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import {
  Capability,
  declareMethod,
  PROTOCOL,
  decodeReply,
  digestJSON,
  parseJSON,
} from "@flame/backend-sdk/mcp";
import { z } from "zod";
import descriptor, { METHODS } from "../contracts/capability";
import { digest } from "./document";
import { inspectFile } from "./formats";
import { OfficePackage } from "./formats/archive";
import { LIMITS } from "../contracts/limits";
const bundlePreparationTimeoutMilliseconds = 60_000;
const requestTimeoutMilliseconds = 90_000;
const scenarioTimeoutMilliseconds = 90_000;
const maximumBackendDiagnosticCodeUnits = 16_000;
const toolReceiptMethodName = declareMethod(parseJSON(protocol.toolReceipt))
  .descriptor.name;
let compiled;
const cleanup = [];
beforeAll(async () => {
  compiled = await mkdtemp(
    join(await realpath(tmpdir()), "flame-deliverables-bundle-"),
  );
  await buildBackend({
    target: protocol.constants.nodeTarget,
    maximumFrameBytes: protocol.constants.maximumFrameBytes,
    input: fileURLToPath(new URL("./start.ts", import.meta.url)),
    directory: compiled,
    compiler: rolldown,
  });
}, bundlePreparationTimeoutMilliseconds);
afterEach(async () => {
  const failures = [];
  for (const close of cleanup.splice(0).reverse()) {
    try {
      await close();
    } catch (failure) {
      failures.push(failure);
    }
  }
  if (failures.length === 1) throw failures[0];
  if (failures.length > 1)
    throw new AggregateError(failures, "protocol fixture cleanup failed");
});
afterAll(async () => {
  if (compiled)
    await rm(compiled, {
      recursive: true,
      force: true,
    });
});
const response = z.record(z.string(), z.unknown());
const provenance = {
  sources: [
    {
      title: "Fixture evidence",
      uri: "https://example.invalid/source",
      note: "offline fixture; not fetched",
    },
  ],
  assumptions: ["Numbers are illustrative"],
  units: "USD",
  reproduction: "Authored from the retained create command.",
};
async function connect(dataDirectory) {
  if (dataDirectory === undefined) {
    dataDirectory = await mkdtemp(
      join(await realpath(tmpdir()), "flame-deliverables-protocol-"),
    );
    const directory = dataDirectory;
    cleanup.push(() =>
      rm(directory, {
        recursive: true,
        force: true,
      }),
    );
  }
  const client = new Client({
    name: "deliverables-test",
    version: "1",
  });
  const transport = new StdioClientTransport({
    command: process.execPath,
    args: [join(compiled, "start.mjs")],
    env: {
      PLUGIN_DATA: dataDirectory,
      PLUGIN_ROOT: compiled,
    },
    stderr: "pipe",
  });
  let stderr = "";
  transport.stderr?.on("data", (chunk) => {
    stderr = (stderr + chunk.toString()).slice(
      -maximumBackendDiagnosticCodeUnits,
    );
  });
  const ended = Promise.withResolvers();
  client.onclose = ended.resolve;
  const close = async () => {
    try {
      await client.close();
    } finally {
      // Stdio close can return after signalling; its event confirms exit before removing data.
      await ended.promise;
    }
  };
  cleanup.push(close);
  try {
    await client.connect(transport);
  } catch (cause) {
    throw new Error(`packaged backend failed: ${stderr}`, { cause });
  }
  const described = decodeReply(
    await client.request(
      {
        method: mcpProtocol.constants.describeMethod,
        params: { protocol: PROTOCOL },
      },
      response,
    ),
  );
  const methods = new Map(
    described.methods.map((method) => [method.name, declareMethod(method)]),
  );
  const capability = new Capability(
    described.capabilities.find((entry) => entry.name === "deliverables")
      .contract,
  );
  expect(capability.identity).toBe(new Capability(descriptor).identity);
  await client.request(
    {
      method: mcpProtocol.constants.setupMethod,
      params: {
        protocol: PROTOCOL,
        contributions: [
          {
            id: "deliverables",
            export: "deliverables",
            requires: [],
          },
        ],
      },
    },
    response,
  );
  await client.request(
    {
      method: mcpProtocol.constants.activateMethod,
      params: { protocol: PROTOCOL },
    },
    response,
  );
  const meta = (id, origin) => ({
    [mcpProtocol.constants.invocationMetadataKey]: {
      id,
      origin,
      target: {
        scope: "installation",
        reference: "deliverables.fixture",
      },
    },
  });
  async function invoke(name, input, id = randomUUID()) {
    const method = methods.get(name);
    const member =
      descriptor.methods.some((entry) => entry.name === name) ||
      name === mcpProtocol.constants.receiptMethod;
    return decodeReply(
      await client.request(
        {
          method: mcpProtocol.constants.invokeMethod,
          params: {
            protocol: PROTOCOL,
            method: name,
            contract: method.identity,
            input,
            ...(member
              ? {
                  export: "deliverables",
                  capability: capability.identity,
                }
              : {}),
            ...(method.descriptor.kind === "command" ? { commandId: id } : {}),
            _meta: meta(`caller.${id}`, "human"),
          },
        },
        response,
        { timeout: requestTimeoutMilliseconds },
      ),
    );
  }
  return {
    dataDirectory,
    close,
    invoke,
    tools: () => client.listTools(),
    tool: (name, input, id = randomUUID(), project) =>
      client.callTool(
        {
          name,
          arguments: input,
          _meta: {
            ...meta(id, "model"),
            [mcpProtocol.constants.toolContextMetadataKey]: {
              cwd: dataDirectory,
              ...(project ? { project } : {}),
            },
          },
        },
        undefined,
        { timeout: requestTimeoutMilliseconds },
      ),
  };
}
beforeEach(async (context) => {
  context.backend = await connect();
});
it(
  "packaged creation, native edit, transfer and original receipt survive restart without content duplication",
  async ({ backend: first }) => {
    const input = {
      title: "Quarterly report",
      format: "docx",
      contentJSON: JSON.stringify({
        title: "Quarterly report",
        blocks: [
          {
            type: "paragraph",
            heading: 1,
            runs: [{ text: "Quarterly report" }],
          },
          {
            type: "paragraph",
            runs: [{ text: "Revenue is 100 USD." }],
          },
        ],
      }),
      provenance,
    };
    const original = "create-report";
    const created = await first.invoke(METHODS.create, input, original);
    expect(created.operation).toBe("create");
    const read = await first.invoke(METHODS.read, { id: created.documentId });
    const item = JSON.parse(read.inspectionJSON).items.find(
      (item) => item.text === "Revenue is 100 USD.",
    );
    expect(item).toBeDefined();
    const edit = {
      id: created.documentId,
      expectedRevision: created.id,
      operationsJSON: JSON.stringify({
        operations: [
          {
            type: "replaceText",
            id: item.id,
            expect: item.text,
            text: "Revenue is 112 USD.",
          },
        ],
      }),
    };
    const changed = await first.invoke(METHODS.edit, edit, "edit-report");
    expect(changed.parent).toBe(created.id);
    expect(changed.sha256).not.toBe(created.sha256);
    await expect(
      first.invoke(METHODS.edit, edit, "stale-edit"),
    ).rejects.toMatchObject({ code: "revision_conflict" });
    const exported = await first.invoke(
      METHODS.export,
      {
        id: created.documentId,
        revision: changed.id,
        kind: "native",
      },
      "export-native",
    );
    const downloaded = await first.invoke(METHODS.download, {
      assetId: exported.id,
      offset: 0,
      length: LIMITS.chunkBytes,
    });
    expect(downloaded.done).toBe(true);
    const bytes = Buffer.from(downloaded.base64, "base64");
    expect(bytes.length).toBe(exported.size);
    expect(digest(bytes)).toBe(exported.sha256);
    expect(
      (await inspectFile("docx", bytes)).items.some(
        (item) => item.text === "Revenue is 112 USD.",
      ),
    ).toBe(true);
    const historical = await first.invoke(METHODS.read, {
      id: created.documentId,
      revision: created.id,
    });
    expect(historical.inspectionJSON).toContain("Revenue is 100 USD.");
    expect(historical.assessments).toEqual([]);
    await expect(
      first.invoke(
        METHODS.render,
        {
          id: created.documentId,
          revision: changed.id,
        },
        "unconfigured-render",
      ),
    ).rejects.toBeInstanceOf(Error);
    await first.close();
    const next = await connect(first.dataDirectory);
    expect(await next.invoke(METHODS.create, input, original)).toEqual(created);
    expect(
      await next.invoke(mcpProtocol.constants.receiptMethod, {
        id: "edit-report",
        method: METHODS.edit,
        inputDigest: digestJSON(edit),
      }),
    ).toEqual({
      found: true,
      reply: { value: changed },
    });
    expect(
      (await next.invoke(METHODS.read, { id: created.documentId })).revision.id,
    ).toBe(changed.id);
    const restore = {
      id: created.documentId,
      expectedRevision: changed.id,
      revision: created.id,
    };
    const restored = await next.invoke(
      METHODS.restore,
      restore,
      "restore-report",
    );
    expect(restored).toMatchObject({
      operation: "restore",
      parent: changed.id,
      restored: created.id,
      sha256: created.sha256,
    });
    await expect(
      next.invoke(METHODS.restore, restore, "stale-restore"),
    ).rejects.toMatchObject({ code: "revision_conflict" });
    await expect(
      next.invoke(
        METHODS.restore,
        { ...restore, expectedRevision: restored.id },
        "current-restore",
      ),
    ).rejects.toMatchObject({ code: "invalid_restore" });
    expect(
      (
        await next.invoke(METHODS.history, { id: created.documentId, limit: 8 })
      ).entries.map((entry) => entry.id),
    ).toEqual([restored.id, changed.id, created.id]);
    await next.invoke(
      METHODS.remove,
      {
        id: created.documentId,
        expectedRevision: restored.id,
      },
      "remove-report",
    );
    expect(await next.invoke(METHODS.create, input, original)).toEqual(created);
    expect(await next.invoke(METHODS.list, { limit: 10 })).toEqual({
      entries: [],
    });
    await expect(
      next.invoke(METHODS.download, {
        assetId: exported.id,
        offset: 0,
        length: 1,
      }),
    ).rejects.toMatchObject({ code: "asset_not_found" });
  },
  scenarioTimeoutMilliseconds,
);
it(
  "model tools create editable workbook and chart slides in the calling Run's Project through the same owner and recover the exact tool outcome",
  async ({ backend }) => {
    const result = await backend.invoke(METHODS.catalog, {});
    const formats = JSON.parse(result.formatsJSON).formats;
    const project = { xlsx: "prj_payments", pptx: undefined };
    for (const format of ["xlsx", "pptx"]) {
      const input = {
        title: `Native ${format}`,
        format,
        contentJSON: formats.find((entry) => entry.format === format)
          .exampleJSON,
        provenance,
      };
      const id = `tool-${format}`;
      const output = await backend.tool("create", input, id, project[format]);
      expect(output.isError, JSON.stringify(output)).not.toBe(true);
      const native = output.structuredContent.value;
      expect(native.format).toBe(format);
      const read = await backend.invoke(METHODS.read, {
        id: native.documentId,
      });
      expect(read.revision.provenance).toEqual(provenance);
      expect(read.inspectionJSON).toContain(
        format === "xlsx" ? "formula" : "chart",
      );
      const receipt = await backend.invoke(toolReceiptMethodName, {
        id: `${protocol.constants.toolCommandPrefix}${id}`,
        method: "create",
        inputDigest: digestJSON(input),
      });
      expect(receipt.result).toBeDefined();
      expect(receipt.result.output).toEqual({
        content: output.content.map((part) => ({
          kind: "text",
          text: part.text,
        })),
        details: output.structuredContent,
      });
      expect(await backend.tool("create", input, id, project[format])).toEqual(
        output,
      );
    }
    const formatsIn = (entries) => entries.map((entry) => entry.format);
    const listed = await backend.tool(
      "list",
      { limit: LIMITS.pageSize },
      undefined,
      "prj_payments",
    );
    expect(formatsIn(listed.structuredContent.value.entries)).toEqual(["xlsx"]);
    const unscoped = await backend.invoke(METHODS.list, {
      limit: LIMITS.pageSize,
    });
    expect(formatsIn(unscoped.entries)).toEqual(["pptx", "xlsx"]);
  },
  scenarioTimeoutMilliseconds,
);
it(
  "verified chunked import preserves native bytes and rejects wrong hashes without consuming the upload",
  async ({ backend }) => {
    const content = JSON.stringify({
      title: "Import source",
      blocks: [
        {
          type: "paragraph",
          runs: [{ text: "Original bytes" }],
        },
      ],
    });
    const native = await backend.invoke(METHODS.create, {
      title: "Import source",
      format: "docx",
      contentJSON: content,
      provenance,
    });
    const chunk = await backend.invoke(METHODS.download, {
      assetId: native.id,
      offset: 0,
      length: LIMITS.chunkBytes,
    });
    const bytes = Buffer.from(chunk.base64, "base64");
    for (const valid of [false, true]) {
      const upload = await backend.invoke(METHODS.upload, {
        size: bytes.length,
        sha256: valid ? digest(bytes) : digest("other"),
      });
      const middle = Math.floor(bytes.length / 2);
      await backend.invoke(METHODS.append, {
        uploadId: upload.id,
        offset: 0,
        base64: bytes.subarray(0, middle).toString("base64"),
      });
      await backend.invoke(METHODS.append, {
        uploadId: upload.id,
        offset: middle,
        base64: bytes.subarray(middle).toString("base64"),
      });
      const request = {
        title: "Imported",
        format: "docx",
        uploadId: upload.id,
        provenance,
      };
      if (!valid) {
        await expect(
          backend.invoke(METHODS.import, request),
        ).rejects.toMatchObject({ code: "incomplete_upload" });
        expect(await backend.invoke(METHODS.uploads, {})).toContainEqual(
          expect.objectContaining({ id: upload.id }),
        );
        await backend.invoke(METHODS.discard, { uploadId: upload.id });
      } else {
        const imported = await backend.invoke(METHODS.import, request);
        expect(imported.sha256).toBe(native.sha256);
        expect(imported.operation).toBe("import");
        expect(await backend.invoke(METHODS.uploads, {})).toEqual([]);
      }
    }
  },
  scenarioTimeoutMilliseconds,
);
it(
  "live sheet edits keep independent concurrent cells, refuse stale and locked cells and project the native workbook",
  async ({ backend }) => {
    const created = await backend.invoke(METHODS.create, {
      title: "Regional revenue",
      format: "xlsx",
      contentJSON: JSON.stringify({
        title: "Regional revenue",
        sheets: [
          {
            name: "Revenue",
            cells: [
              { address: "A1", value: { type: "string", value: "Region" } },
              { address: "B1", value: { type: "string", value: "Revenue" } },
              { address: "A2", value: { type: "string", value: "North" } },
              { address: "B2", value: { type: "number", value: 100 } },
              { address: "A3", value: { type: "string", value: "South" } },
              { address: "B3", value: { type: "number", value: 200 } },
              {
                address: "B4",
                value: { type: "formula", value: "=SUM(B2:B3)" },
              },
              { address: "D1", value: { type: "string", value: "Note" } },
            ],
            merges: ["D1:E1"],
            freezeRows: 1,
          },
        ],
      }),
      provenance,
    });
    const id = created.documentId;
    const sheets = async (revision) =>
      JSON.parse(
        (await backend.invoke(METHODS.sheets, { id, revision })).layoutJSON,
      ).sheets;
    const edit = (expectedRevision, operations, command) =>
      backend.invoke(
        METHODS.edit,
        {
          id,
          expectedRevision,
          operationsJSON: JSON.stringify({ operations }),
        },
        command,
      );
    const [sheet] = await sheets(created.id);
    const cell = (view, ref) => view.cells.find((entry) => entry.ref === ref);
    expect(sheet).toMatchObject({
      name: "Revenue",
      rows: 4,
      columns: 5,
      frozen: { rows: 1, columns: 0 },
      locked: [{ range: "D1:E1", reason: "merge", anchor: "D1" }],
    });
    expect(cell(sheet, "B2")).toMatchObject({
      shown: { type: "number", value: 100 },
      input: { type: "number", value: 100 },
    });
    expect(cell(sheet, "B4").input).toEqual({
      type: "formula",
      value: "=SUM(B2:B3)",
    });
    const setCell = (view, ref, value) => ({
      type: "setCell",
      id: `${view.id}!${ref}`,
      expect: cell(view, ref)?.expect ?? "",
      value,
    });

    const byHuman = await edit(
      created.id,
      [setCell(sheet, "B2", { type: "number", value: 150 })],
      "human-cell",
    );
    const byAgent = await edit(
      byHuman.id,
      [
        setCell(sheet, "A3", { type: "string", value: "South (revised)" }),
        setCell(sheet, "C2", { type: "string", value: "checked" }),
      ],
      "agent-cells",
    );
    const [current] = await sheets(byAgent.id);
    expect(cell(current, "B2").shown).toEqual({ type: "number", value: 150 });
    expect(cell(current, "A3").shown).toEqual({
      type: "string",
      value: "South (revised)",
    });
    expect(cell(current, "C2").input).toEqual({
      type: "string",
      value: "checked",
    });
    expect(cell(current, "B4").shown).toEqual({ type: "blank" });
    await expect(
      edit(
        byAgent.id,
        [setCell(sheet, "B2", { type: "number", value: 999 })],
        "stale-cell",
      ),
    ).rejects.toMatchObject({ code: "invalid_native" });
    await expect(
      edit(
        byAgent.id,
        [setCell(current, "E1", { type: "string", value: "inside merge" })],
        "locked-cell",
      ),
    ).rejects.toMatchObject({ code: "invalid_native" });
    expect(await sheets()).toEqual([current]);

    const outline = await backend.tool("sheets", { id });
    expect(outline.isError, JSON.stringify(outline)).not.toBe(true);
    expect(JSON.parse(outline.content[0].text).value).toMatchObject({
      revision: byAgent.id,
      sheets: [{ id: sheet.id, name: "Revenue", cells: current.cells.length }],
    });
  },
  scenarioTimeoutMilliseconds,
);
it(
  "live document edits keep independent concurrent blocks, refuse stale and overlapping ones and project the native body",
  async ({ backend }) => {
    const created = await backend.invoke(METHODS.create, {
      title: "Launch brief",
      format: "docx",
      contentJSON: JSON.stringify({
        title: "Launch brief",
        blocks: [
          { type: "paragraph", heading: 1, runs: [{ text: "Launch brief" }] },
          {
            type: "paragraph",
            runs: [
              { text: "Ship the " },
              { text: "beta", bold: true },
              { text: " in March." },
            ],
          },
          {
            type: "table",
            headers: ["Owner", "Task"],
            rows: [["Ana", "Docs"]],
          },
          { type: "paragraph", runs: [{ text: "Risks remain." }] },
        ],
      }),
      provenance,
    });
    const id = created.documentId;
    const blocks = async (revision) =>
      JSON.parse(
        (await backend.invoke(METHODS.blocks, { id, revision })).layoutJSON,
      ).blocks;
    const edit = (expectedRevision, operations, command) =>
      backend.invoke(
        METHODS.edit,
        {
          id,
          expectedRevision,
          operationsJSON: JSON.stringify({ operations }),
        },
        command,
      );
    const text = (block) => block.runs.map((run) => run.text).join("");
    const paragraph = (view, value) =>
      view.find((block) => block.kind === "paragraph" && text(block) === value);
    const original = await blocks(created.id);
    const heading = paragraph(original, "Launch brief");
    const plan = paragraph(original, "Ship the beta in March.");
    const risks = paragraph(original, "Risks remain.");
    const table = original.find((block) => block.kind === "table");
    expect(heading.style).toBe("Heading1");
    expect(plan.runs.map((run) => [run.text, run.bold])).toEqual([
      ["Ship the ", false],
      ["beta", true],
      [" in March.", false],
    ]);
    expect(table.rows).toEqual([
      ["Owner", "Task"],
      ["Ana", "Docs"],
    ]);

    const month = plan.runs[2];
    const byHuman = await edit(
      created.id,
      [
        {
          type: "replaceText",
          id: month.id,
          expect: month.text,
          text: " in April.",
        },
        {
          type: "setParagraphStyle",
          id: risks.id,
          expect: risks.expect,
          style: "Heading2",
        },
      ],
      "human-blocks",
    );
    const byAgent = await edit(
      byHuman.id,
      [
        {
          type: "insertParagraph",
          after: table.id,
          expect: table.expect,
          text: "Owners confirm by Friday.",
        },
        { type: "insertParagraph", text: "Draft", style: "Heading3" },
      ],
      "agent-blocks",
    );
    const current = await blocks(byAgent.id);
    expect(
      current.map((block) =>
        block.kind === "paragraph" ? [block.style, text(block)] : block.kind,
      ),
    ).toEqual([
      ["Heading3", "Draft"],
      ["Heading1", "Launch brief"],
      [expect.any(String), "Ship the beta in April."],
      "table",
      [expect.any(String), "Owners confirm by Friday."],
      ["Heading2", "Risks remain."],
    ]);
    await expect(
      edit(
        byAgent.id,
        [{ type: "deleteBlock", id: plan.id, expect: plan.expect }],
        "stale-block",
      ),
    ).rejects.toMatchObject({ code: "invalid_native" });
    const revised = paragraph(current, "Ship the beta in April.");
    await expect(
      edit(
        byAgent.id,
        [
          { type: "deleteBlock", id: revised.id, expect: revised.expect },
          {
            type: "replaceText",
            id: revised.runs[0].id,
            expect: revised.runs[0].text,
            text: "Launch the ",
          },
        ],
        "overlapping-block",
      ),
    ).rejects.toMatchObject({ code: "invalid_native" });
    expect(await blocks()).toEqual(current);

    const outline = await backend.tool("blocks", { id });
    expect(outline.isError, JSON.stringify(outline)).not.toBe(true);
    expect(JSON.parse(outline.content[0].text).value).toMatchObject({
      revision: byAgent.id,
      blocks: current.length,
      tables: 1,
      headings: [
        { index: 0, style: "Heading3", text: "Draft" },
        { index: 1, style: "Heading1", text: "Launch brief" },
        { index: 5, style: "Heading2", text: "Risks remain." },
      ],
    });
  },
  scenarioTimeoutMilliseconds,
);
it(
  "live slide edits keep independent concurrent changes, refuse stale ones and leave valid native packages",
  async ({ backend }) => {
    const created = await backend.invoke(METHODS.create, {
      title: "Review deck",
      format: "pptx",
      contentJSON: JSON.stringify({
        title: "Review deck",
        slides: [
          {
            title: "Revenue",
            elements: [
              {
                type: "text",
                x: 0.5,
                y: 0.5,
                width: 6,
                height: 1,
                text: "Revenue grew",
              },
              {
                type: "shape",
                shape: "rect",
                x: 0.5,
                y: 2,
                width: 2,
                height: 1,
                fill: "245B78",
              },
              {
                type: "chart",
                chart: "bar",
                x: 4,
                y: 2,
                width: 6,
                height: 4,
                series: [
                  {
                    name: "Revenue (USD million)",
                    labels: ["Q1", "Q2"],
                    values: [100, 112],
                  },
                ],
              },
            ],
            notes: "Explain the growth",
          },
        ],
      }),
      provenance,
    });
    const id = created.documentId;
    const layout = async (revision) =>
      JSON.parse(
        (await backend.invoke(METHODS.slides, { id, revision })).layoutJSON,
      );
    const edit = (expectedRevision, operations, command) =>
      backend.invoke(
        METHODS.edit,
        {
          id,
          expectedRevision,
          operationsJSON: JSON.stringify({ operations }),
        },
        command,
      );
    const native = async (revision) => {
      const chunk = await backend.invoke(METHODS.download, {
        assetId: revision.native.id,
        offset: 0,
        length: LIMITS.chunkBytes,
      });
      expect(chunk.done).toBe(true);
      return OfficePackage.open("pptx", Buffer.from(chunk.base64, "base64"));
    };
    const [slide] = (await layout(created.id)).slides;
    const title = slide.elements.find((element) => element.kind === "text");
    const shape = slide.elements.find((element) => element.kind === "shape");
    const chart = slide.elements.find((element) => element.kind === "chart");
    expect(chart.chart).toEqual({
      type: "bar",
      workbook: "chart",
      data: {
        categories: ["Q1", "Q2"],
        series: [{ name: "Revenue (USD million)", values: [100, 112] }],
      },
    });
    expect(slide.notes.map((run) => run.text)).toEqual(["Explain the growth"]);

    const moved = { ...shape.frame, x: shape.frame.x + 914400 };
    const human = [
      { type: "setFrame", id: shape.id, expect: shape.frame, frame: moved },
    ];
    const byHuman = await edit(created.id, human, "human-move");
    const converted = {
      categories: ["Q1", "Q2"],
      series: [{ name: "Revenue (CNY 10k)", values: [720, 806.4] }],
    };
    const byAgent = await edit(
      byHuman.id,
      [
        {
          type: "replaceText",
          id: title.text[0].id,
          expect: "Revenue grew",
          text: "Revenue grew 12%",
        },
        {
          type: "setChartData",
          id: chart.id,
          expect: chart.chart.data,
          data: converted,
        },
      ],
      "agent-edit",
    );
    const [current] = (await layout(byAgent.id)).slides;
    const find = (target) =>
      current.elements.find((element) => element.id === target.id);
    expect(find(shape).frame).toEqual(moved);
    expect(find(title).text.map((run) => run.text)).toEqual([
      "Revenue grew 12%",
    ]);
    expect(find(chart).chart.data).toEqual(converted);
    const deck = await native(byAgent);
    const workbooks = [...deck.parts.keys()].filter((part) =>
      part.startsWith("ppt/embeddings/"),
    );
    expect(workbooks).toHaveLength(1);
    const cells = (
      await inspectFile("xlsx", deck.parts.get(workbooks[0]))
    ).items.filter((item) => item.kind === "cell");
    for (const value of ['"Revenue (CNY 10k)"', "720", "806.4"])
      expect(cells.some((cell) => cell.text.includes(value))).toBe(true);
    expect(cells.some((cell) => cell.text.includes("112"))).toBe(false);
    await expect(edit(byAgent.id, human, "stale-move")).rejects.toMatchObject({
      code: "invalid_native",
    });
    expect((await layout()).slides[0]).toEqual(current);

    const duplicated = await edit(
      byAgent.id,
      [{ type: "duplicateSlide", id: slide.id }],
      "duplicate",
    );
    const [original, copy] = (await layout(duplicated.id)).slides;
    expect(original.id).toBe(slide.id);
    expect(copy.id).not.toBe(slide.id);
    expect(copy.notes.map((run) => run.text)).toEqual(["Explain the growth"]);
    const copiedChart = copy.elements.find(
      (element) => element.kind === "chart",
    );
    const independent = await edit(
      duplicated.id,
      [
        {
          type: "setChartData",
          id: copiedChart.id,
          expect: converted,
          data: {
            categories: ["H1", "H2"],
            series: [{ name: "Copy", values: [1, null] }],
          },
        },
      ],
      "copy-chart",
    );
    const separated = await layout(independent.id);
    expect(
      separated.slides[0].elements.find((element) => element.kind === "chart")
        .chart.data,
    ).toEqual(converted);

    const before = await native(independent);
    const removed = await edit(
      independent.id,
      [{ type: "deleteSlide", id: slide.id }],
      "delete-original",
    );
    const remaining = await layout(removed.id);
    expect(remaining.slides.map((entry) => entry.id)).toEqual([copy.id]);
    const after = await native(removed);
    const gone = [...before.parts.keys()].filter(
      (part) => !after.parts.has(part),
    );
    expect(gone).toContain(slide.id);
    expect(
      gone.filter((part) => part.startsWith("ppt/charts/chart")),
    ).toHaveLength(1);
    expect(
      gone.filter((part) => part.startsWith("ppt/embeddings/")),
    ).toHaveLength(1);
    expect(
      gone.filter((part) => part.startsWith("ppt/notesSlides/notesSlide")),
    ).toHaveLength(1);
    await expect(
      edit(removed.id, [{ type: "deleteSlide", id: copy.id }], "delete-last"),
    ).rejects.toMatchObject({ code: "invalid_native" });

    const outline = await backend.tool("slides", { id });
    expect(outline.isError, JSON.stringify(outline)).not.toBe(true);
    expect(JSON.parse(outline.content[0].text).value).toMatchObject({
      revision: removed.id,
      slides: [{ id: copy.id, elements: copy.elements.length }],
    });

    const [blank] = remaining.layouts;
    const grown = await edit(
      removed.id,
      [{ type: "insertSlide", after: copy.id, layout: blank.id }],
      "insert-slide",
    );
    const inserted = (await layout(grown.id)).slides[1];
    expect(inserted.id).not.toBe(copy.id);
    const frame = { x: 914400, y: 914400, width: 3657600, height: 914400 };
    const labelled = await edit(
      grown.id,
      [
        { type: "addText", slide: inserted.id, frame, text: "Next steps" },
        {
          type: "addShape",
          slide: inserted.id,
          frame: { ...frame, y: 2743200 },
          shape: "roundRect",
          text: "Owner",
        },
      ],
      "add-elements",
    );
    expect(
      (await layout(labelled.id)).slides[1].elements.map((element) => [
        element.frame,
        element.text.map((run) => run.text),
      ]),
    ).toEqual([
      [frame, ["Next steps"]],
      [{ ...frame, y: 2743200 }, ["Owner"]],
    ]);
  },
  scenarioTimeoutMilliseconds,
);
it(
  "open editors reading a revision while the model edits it all succeed",
  async ({ backend }) => {
    const created = await backend.invoke(METHODS.create, {
      title: "Status note",
      format: "docx",
      contentJSON: JSON.stringify({
        title: "Status note",
        blocks: [{ type: "paragraph", runs: [{ text: "On track." }] }],
      }),
      provenance,
    });
    const id = created.documentId;
    const read = (revision) =>
      backend
        .invoke(METHODS.blocks, { id, revision })
        .then((result) => JSON.parse(result.layoutJSON).blocks);
    const [first, second, edited] = await Promise.all([
      read(created.id),
      read(created.id),
      backend.invoke(METHODS.edit, {
        id,
        expectedRevision: created.id,
        operationsJSON: JSON.stringify({
          operations: [{ type: "appendParagraph", text: "Next: launch." }],
        }),
      }),
    ]);
    expect(first).toEqual(second);
    expect(first).toHaveLength(1);
    const after = await read(edited.id);
    expect(
      after.map((block) => block.runs.map((run) => run.text).join("")),
    ).toEqual(["On track.", "Next: launch."]);
  },
  scenarioTimeoutMilliseconds,
);
