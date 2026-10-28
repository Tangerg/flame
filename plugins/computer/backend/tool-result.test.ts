import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { canonicalJSON, parseJSON } from "@qimu/node";
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import { admitToolResult, projectToolOutput } from "./tool-result";

const cases = parseJSON(
  readFileSync(
    new URL("./testdata/tool-results.json", import.meta.url),
    "utf8",
  ),
) as Array<{
  name: string;
  result: CallToolResult;
  expected?: unknown;
  rejected?: boolean;
}>;

describe("MCP result receipt projection", () => {
  it.each(cases)("matches the pinned Scope mapper: $name", (test) => {
    const original = canonicalJSON(test.result);
    if (test.rejected) {
      expect(() => projectToolOutput("js", test.result)).toThrow();
    } else {
      expect(canonicalJSON(projectToolOutput("js", test.result))).toBe(
        canonicalJSON(test.expected),
      );
    }
    expect(canonicalJSON(test.result)).toBe(original);
  });

  it("refuses malformed or incomplete transport envelopes without manufacturing an outcome", () => {
    for (const result of [
      {},
      { content: null },
      {
        content: [{ type: "image", mimeType: "image/png", data: "not base64" }],
      },
      { content: [], resultType: "input_required" },
      { content: [{ type: "tool_use", name: "hidden-control" }] },
      { content: [{ type: "text", text: "\ud800" }] },
    ])
      expect(() => admitToolResult(result)).toThrow();
  });

  it("detaches projected content and metadata from the immutable original", () => {
    const result = {
      content: [
        {
          type: "text" as const,
          text: "original",
          _meta: { source: { value: 1 } },
        },
      ],
      structuredContent: { value: 1 },
    };
    const projected = projectToolOutput("js", result);
    (projected.details as { value: number }).value = 2;
    (
      projected.content![0]!.metadata!["mcp/content"] as {
        _meta: { source: { value: number } };
      }
    )._meta.source.value = 2;
    expect(result.structuredContent.value).toBe(1);
    expect(result.content[0]!._meta.source.value).toBe(1);
  });

  it("projects a full screenshot without recursive base64 validation", () => {
    const data = Buffer.alloc(4 * 1024 * 1024, 23).toString("base64");
    const output = projectToolOutput("js", {
      content: [{ type: "image", mimeType: "image/png", data }],
    });
    expect(output.content?.[0]?.media?.source).toEqual({
      kind: "bytes",
      bytes: data,
    });
  });
});
