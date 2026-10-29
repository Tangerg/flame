import protocol from "@flame/backend-sdk/protocol" with { type: "json" };
import { rolldown } from "rolldown";
import { buildBackend } from "@qimu/node/build";
import { randomUUID } from "node:crypto";
import { mkdtemp, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterAll, afterEach, beforeAll, expect, it } from "vitest";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import {
  Capability,
  Method,
  PROTOCOL,
  decodeReply,
  digestJSON,
} from "@qimu/node";
import { z } from "zod";
import descriptor, { METHODS } from "../contracts/capability";
import { digest } from "./document";
import { inspectFile } from "./formats";
import { LIMITS } from "../contracts/limits";
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
}, 60_000);
afterEach(async () => {
  for (const close of cleanup.splice(0).reverse()) await close();
});
afterAll(async () => {
  if (compiled)
    await rm(compiled, {
      recursive: true,
      force: true,
    });
});
const rpc = {
  describe: "io.github.tangerg.qimu/operations.describe",
  setup: "io.github.tangerg.qimu/operations.setup",
  activate: "io.github.tangerg.qimu/operations.activate",
  invoke: "io.github.tangerg.qimu/operations.invoke",
  receipt: "io.github.tangerg.qimu/commands.get",
  invocation: "io.github.tangerg.qimu/invocation",
  toolReceipt: "io.github.tangerg.flame/tools.getResult",
};
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
async function connect(data) {
  if (!data) {
    data = await mkdtemp(
      join(await realpath(tmpdir()), "flame-deliverables-protocol-"),
    );
    const directory = data;
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
      PLUGIN_DATA: data,
      PLUGIN_ROOT: compiled,
    },
    stderr: "pipe",
  });
  let stderr = "";
  transport.stderr?.on("data", (chunk) => {
    stderr = (stderr + chunk.toString()).slice(-16e3);
  });
  try {
    await client.connect(transport);
  } catch (cause) {
    throw new Error(`packaged backend failed: ${stderr}`, { cause });
  }
  let closed = false;
  const close = async () => {
    if (closed) return;
    closed = true;
    await client.close();
  };
  cleanup.push(close);
  const described = decodeReply(
    await client.request(
      {
        method: rpc.describe,
        params: { protocol: PROTOCOL },
      },
      response,
    ),
  );
  const methods = new Map(
    described.methods.map((method) => [method.name, new Method(method)]),
  );
  const capability = new Capability(
    described.capabilities.find((entry) => entry.name === "deliverables")
      .contract,
  );
  expect(capability.identity).toBe(new Capability(descriptor).identity);
  await client.request(
    {
      method: rpc.setup,
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
      method: rpc.activate,
      params: { protocol: PROTOCOL },
    },
    response,
  );
  const meta = (id, origin) => ({
    [rpc.invocation]: {
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
      name === rpc.receipt;
    return decodeReply(
      await client.request(
        {
          method: rpc.invoke,
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
        { timeout: 9e4 },
      ),
    );
  }
  return {
    data,
    close,
    invoke,
    tool: (name, input, id = randomUUID()) =>
      client.callTool(
        {
          name,
          arguments: input,
          _meta: meta(id, "model"),
        },
        undefined,
        { timeout: 9e4 },
      ),
  };
}
it("packaged creation, native edit, transfer and original receipt survive restart without content duplication", async () => {
  const first = await connect();
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
  const next = await connect(first.data);
  expect(await next.invoke(METHODS.create, input, original)).toEqual(created);
  expect(
    await next.invoke(rpc.receipt, {
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
  await next.invoke(
    METHODS.remove,
    {
      id: created.documentId,
      expectedRevision: changed.id,
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
}, 9e4);
it("model tools create editable workbook and chart slides through the same owner and recover the exact tool outcome", async () => {
  const backend = await connect();
  const result = await backend.invoke(METHODS.catalog, {});
  const formats = JSON.parse(result.formatsJSON).formats;
  for (const format of ["xlsx", "pptx"]) {
    const input = {
      title: `Native ${format}`,
      format,
      contentJSON: formats.find((entry) => entry.format === format).exampleJSON,
      provenance,
    };
    const id = `tool-${format}`;
    const output = await backend.tool("create", input, id);
    expect(output.isError, JSON.stringify(output)).not.toBe(true);
    const native = output.structuredContent.value;
    expect(native.format).toBe(format);
    const read = await backend.invoke(METHODS.read, { id: native.documentId });
    expect(read.revision.provenance).toEqual(provenance);
    expect(read.inspectionJSON).toContain(
      format === "xlsx" ? "formula" : "chart",
    );
    const receipt = await backend.invoke(rpc.toolReceipt, {
      id: `tool:${id}`,
      method: "create",
      inputDigest: digestJSON(input),
    });
    expect(receipt.found).toBe(true);
    expect(JSON.parse(receipt.output)).toEqual({
      content: output.content.map((part) => ({
        kind: "text",
        text: part.text,
      })),
      details: output.structuredContent,
    });
    expect(await backend.tool("create", input, id)).toEqual(output);
  }
}, 9e4);
it("verified chunked import preserves native bytes and rejects wrong hashes without consuming the upload", async () => {
  const backend = await connect();
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
}, 9e4);
