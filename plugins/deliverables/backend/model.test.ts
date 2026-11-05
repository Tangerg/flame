import { expect, it, vi } from "vitest";
import { canonicalJSON, parseJSON } from "@flame/backend-sdk/mcp";
import { MODEL_LIMITS, LIMITS } from "../contracts/limits";
import { definitions } from "../contracts/capability";
import { catalog } from "./formats";
import {
  commandProjection,
  project,
  queryProjection,
  toolResult,
} from "./model";

const revision = {
  id: "a".repeat(64),
  documentId: "b".repeat(64),
  createdAt: "2026-11-05T00:00:00.000Z",
  operation: "create" as const,
  format: "docx" as const,
  sha256: "c".repeat(64),
  size: 512,
  provenance: {
    sources: [],
    assumptions: [],
    units: "USD",
    reproduction: "Original authored provenance.",
  },
  native: {
    id: "a".repeat(64),
    filename: "source.docx",
    mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    size: 512,
    sha256: "c".repeat(64),
  },
};

function inspected() {
  const inspection = {
    format: "docx",
    items: Array.from({ length: 100 }, (_, index) => ({
      id: `word/document.xml:text:${index}`,
      kind: "text",
      text: `Exact ${index}: ` + "x".repeat(4000),
    })),
    facts: { exact: "9007199254740993" },
    checks: [],
    truncated: false,
  };
  return {
    document: {
      id: revision.documentId,
      title: "Retained source",
      head: revision.id,
      format: "docx",
      createdAt: revision.createdAt,
    },
    revision,
    inspectionJSON: canonicalJSON(inspection),
    assessments: [],
  };
}

it("pages original native item identities and complete text within the byte budget", () => {
  const source = JSON.parse(inspected().inspectionJSON);
  const original = canonicalJSON(source);
  const read: unknown[] = [];
  let offset = 0;
  for (;;) {
    const page = project(source, {
      path: ["items"],
      offset,
      limit: MODEL_LIMITS.maximumEntries,
    });
    expect(Buffer.byteLength(canonicalJSON(page))).toBeLessThanOrEqual(
      MODEL_LIMITS.projectionBytes,
    );
    for (const entry of page.entries as Array<{
      key: string;
      value: unknown;
    }>) {
      expect(entry.value).toEqual(source.items[Number(entry.key)]);
      read.push(entry.value);
    }
    if (page.nextOffset === undefined) break;
    expect(page.nextOffset).toBeGreaterThan(offset);
    offset = page.nextOffset as number;
  }
  expect(read).toEqual(source.items);
  expect(canonicalJSON(source)).toBe(original);
});

it("marks oversized entries for an explicit deeper read and preserves Unicode across text pages", () => {
  const text = "🙂界é".repeat(3000);
  const source = { values: [{ id: "original", text }] };
  const page = project(source, { path: ["values"] });
  expect(page.entries).toEqual([
    {
      key: "0",
      omitted: true,
      type: "object",
      keys: 2,
      bytes: Buffer.byteLength(canonicalJSON(source.values[0])),
    },
  ]);
  let joined = "";
  let offset = 0;
  for (;;) {
    const part = project(source, {
      path: ["values", "0", "text"],
      offset,
      limit: 2000,
    });
    expect(part.unit).toBe("unicode-code-points");
    joined += part.text;
    if (part.nextOffset === undefined) break;
    offset = part.nextOffset as number;
  }
  expect(joined).toBe(text);
  expect(
    canonicalJSON(
      project(parseJSON('{"exact":9007199254740993}'), { path: ["exact"] }),
    ),
  ).toContain('"value":9007199254740993');
  expect(() => project(source, { path: ["values", "01"] })).toThrow(
    "array index",
  );
  expect(() => project(source, { path: ["__proto__"] })).toThrow("not present");
  expect(() => project(source, { offset: 1 })).toThrow("explicit path");
});

it("keeps full original inspection in the host result while model reads are pinned and bounded", () => {
  const retained = inspected();
  const query = vi.fn(() => retained);
  const summary = queryProjection({ query }, "read", {
    id: revision.documentId,
  });
  const output = toolResult({ value: summary.retained }, summary.model);
  expect(output.structuredContent).toEqual({ value: retained });
  expect(output.content[0]!.text).not.toContain("Exact 0:");
  expect(JSON.parse(output.content[0]!.text).value).toMatchObject({
    documentId: revision.documentId,
    revision: revision.id,
    sourceSha256: revision.sha256,
    inspection: { items: 100, retainedTruncated: false },
  });
  expect(() =>
    queryProjection({ query }, "read", {
      id: revision.documentId,
      path: ["inspection", "items"],
    }),
  ).toThrow("pin its exact revision");
  const selected = queryProjection({ query }, "read", {
    id: revision.documentId,
    revision: revision.id,
    sourceSha256: revision.sha256,
    path: ["inspection", "items"],
  });
  expect(query).toHaveBeenLastCalledWith("read", {
    id: revision.documentId,
    revision: revision.id,
  });
  expect(selected.retained).toEqual(retained);
  expect(
    Buffer.byteLength(
      toolResult({ value: selected.retained }, selected.model).content[0]!.text,
    ),
  ).toBeLessThanOrEqual(MODEL_LIMITS.textBytes);
  expect(() =>
    queryProjection({ query }, "read", {
      id: revision.documentId,
      revision: revision.id,
      offset: 20,
    }),
  ).toThrow("explicit path");
});

it("discovers one format schema on demand and leaves the original typed catalog intact", () => {
  const retained = {
    formatsJSON: canonicalJSON(catalog()),
    enginesJSON: canonicalJSON({
      platform: "fixture",
      configured: false,
      configuration: {},
      limitations: [],
    }),
    limitsJSON: canonicalJSON(LIMITS),
  };
  const query = vi.fn(() => retained);
  const overview = queryProjection({ query }, "catalog", {});
  expect(canonicalJSON(overview.model)).not.toContain("createSchemaJSON");
  expect(overview.retained).toEqual(retained);
  const selected = queryProjection({ query }, "catalog", {
    format: "docx",
    section: "edit",
  });
  expect(selected.model).toMatchObject({
    format: "docx",
    section: "edit",
    view: {
      type: "object",
      complete: true,
      value: parseJSON(
        catalog().formats.find((entry) => entry.format === "docx")!
          .editSchemaJSON,
      ),
    },
  });
  expect(() =>
    queryProjection({ query }, "catalog", { section: "create" }),
  ).toThrow("select one format");
  expect(() =>
    queryProjection({ query }, "catalog", { format: "pdf", section: "create" }),
  ).toThrow("does not define");
});

it("continues an inspection against the returned original revision and hash after the document head advances", () => {
  const original = inspected();
  const changed = {
    ...original,
    revision: { ...revision, id: "d".repeat(64), sha256: "e".repeat(64) },
    inspectionJSON: canonicalJSON({
      format: "docx",
      items: [],
      facts: {},
      checks: [],
      truncated: false,
    }),
  };
  let current = original;
  const query = vi.fn((_operation, input) =>
    input.revision === revision.id ? original : current,
  );
  const first = queryProjection({ query }, "read", {
    id: revision.documentId,
    revision: revision.id,
    sourceSha256: revision.sha256,
    path: ["inspection", "items"],
    limit: 1,
  });
  const identity = first.model as {
    revision: string;
    sourceSha256: string;
    view: { nextOffset: number };
  };
  current = changed;
  const next = queryProjection({ query }, "read", {
    id: revision.documentId,
    revision: identity.revision,
    sourceSha256: identity.sourceSha256,
    path: ["inspection", "items"],
    offset: identity.view.nextOffset,
    limit: 1,
  });
  expect(next.model).toMatchObject({
    revision: revision.id,
    sourceSha256: revision.sha256,
    view: {
      entries: [{ key: "1", value: { id: "word/document.xml:text:1" } }],
    },
  });
  expect(() =>
    queryProjection({ query }, "read", {
      id: revision.documentId,
      revision: revision.id,
      path: ["inspection", "items"],
      offset: 1,
    }),
  ).toThrow("sourceSha256");
  expect(() =>
    queryProjection({ query }, "read", {
      id: revision.documentId,
      revision: changed.revision.id,
      sourceSha256: revision.sha256,
      path: ["inspection", "items"],
      offset: 1,
    }),
  ).toThrow("expected source SHA-256");
});

it("keeps render limitations and source identity visible while returning original evidence only on selection", () => {
  const report = {
    sourceRevision: revision.id,
    sourceSha256: revision.sha256,
    pdfSha256: "d".repeat(64),
    platform: "fixture",
    engines: [{ name: "renderer", version: "original" }],
    pages: [{ number: 1, assetId: "e".repeat(64), sha256: "f".repeat(64) }],
    checks: [
      {
        code: "unverified_fonts",
        severity: "warning",
        message: "Original engine warning.",
      },
    ],
    extractedText: "Original text 🙂".repeat(3000),
    fontStatus: "unverified",
  };
  const retained = {
    id: "1".repeat(64),
    revision: revision.id,
    sourceSha256: revision.sha256,
    createdAt: revision.createdAt,
    kind: "render",
    reportJSON: canonicalJSON(report),
    assets: [],
  };
  const query = vi.fn(() => retained);
  const input = {
    id: revision.documentId,
    revision: revision.id,
    assessmentId: retained.id,
  };
  const summary = queryProjection({ query }, "assessment", input);
  expect(summary.model).toMatchObject({
    id: retained.id,
    revision: revision.id,
    sourceSha256: revision.sha256,
    report: {
      fontStatus: "unverified",
      checks: { warning: 1, error: 0 },
      pageCount: 1,
    },
  });
  expect(
    toolResult({ value: retained }, summary.model).structuredContent,
  ).toEqual({ value: retained });
  const selected = queryProjection({ query }, "assessment", {
    ...input,
    path: ["report", "extractedText"],
    offset: 2000,
    limit: 1000,
  });
  expect(selected.model).toMatchObject({
    assessmentId: retained.id,
    revision: revision.id,
    sourceSha256: revision.sha256,
    view: {
      text: Array.from(report.extractedText).slice(2000, 3000).join(""),
      nextOffset: 3000,
    },
  });
  expect(commandProjection("render", retained)).toEqual(
    commandProjection("render", structuredClone(retained)),
  );
});

it("uses the same pure bounded model projection for an original command and its receipt", () => {
  const original = {
    value: {
      ...revision,
      provenance: {
        ...revision.provenance,
        reproduction: "exact original text ".repeat(500),
      },
    },
  };
  const first = toolResult(
    original,
    commandProjection("create", original.value),
  );
  const recovered = toolResult(
    structuredClone(original),
    commandProjection("create", original.value),
  );
  expect(recovered).toEqual(first);
  expect(first.structuredContent).toEqual(original);
  expect(first.content[0]!.text).not.toContain("exact original text");
  expect(JSON.parse(first.content[0]!.text).value).toMatchObject({
    id: revision.id,
    sha256: revision.sha256,
    nativeAssetId: revision.native.id,
  });
  const large = toolResult({ value: inspected() });
  expect(Buffer.byteLength(large.content[0]!.text)).toBeLessThanOrEqual(
    MODEL_LIMITS.textBytes,
  );
  expect(large.structuredContent).toEqual({ value: inspected() });
  expect(JSON.parse(large.content[0]!.text)).toMatchObject({
    projected: true,
    originalRetained: true,
  });
});

it("keeps raw file transfer outside the model tool surface and retains semantic operations", () => {
  for (const name of ["upload", "append", "discard", "download"] as const)
    expect(
      definitions.find((definition) => definition.operation === name)?.audience,
    ).toBe("host");
  for (const name of [
    "create",
    "edit",
    "uploads",
    "import",
    "read",
    "render",
    "review",
    "export",
  ] as const)
    expect(
      definitions.find((definition) => definition.operation === name)?.audience,
    ).toBe("model");
});
