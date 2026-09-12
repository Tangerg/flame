import { describe, expect, it } from "vitest";

import { validateWire } from "@flame/runtime-contract/validate";
import { WIRE_ENUMS } from "@flame/runtime-contract/wire";
import { WIRE_SAMPLES } from "@flame/runtime-contract/samples";
import requestMeta from "@flame/runtime-contract/samples/request.meta.json";

const canonicalSamplePrefix = "../../../../runtime/contract/typescript/samples/";
const files = import.meta.glob<{ default: unknown }>(
  "../../../../runtime/contract/typescript/samples/*.json",
  { eager: true },
);

function sample(file: string): unknown {
  const loaded = files[`${canonicalSamplePrefix}${file}`];
  if (!loaded) throw new Error(`no such canonical sample: ${file}`);
  return loaded.default;
}

describe("the canonical wire samples", () => {
  it("covers every file in the samples directory", () => {
    const bound = new Set(WIRE_SAMPLES.map((entry) => entry.file));
    const present = Object.keys(files).map((path) => path.replace(canonicalSamplePrefix, ""));
    expect(present.filter((file) => !bound.has(file))).toEqual([]);
    expect(WIRE_SAMPLES.length).toBe(present.length);
  });

  it.each(WIRE_SAMPLES)("$file satisfies $shape", ({ file, shape }) => {
    expect(validateWire(shape, sample(file))).toEqual([]);
  });

  it("excludes only published stream events", () => {
    const published = new Set<string>(WIRE_ENUMS.StreamEventType);
    for (const event of requestMeta.clientCapabilities.excludedEphemeralEvents ?? []) {
      expect(published.has(event)).toBe(true);
    }
  });
});

describe("generated MCP remote-tool constraints", () => {
  const candidate = (disabledTools: string[]) => ({
    name: "files",
    enabled: true,
    connection: { type: "stdio", command: "mcp-files" },
    handshakeTimeout: { type: "unbounded" },
    disabledTools,
  });

  it("rejects an invalid remote identity at the item path", () => {
    expect(validateWire("MCPServerCandidate", candidate(["tool/name"]))).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: "MCPServerCandidate.disabledTools[0]" }),
      ]),
    );
  });

  it("rejects more than one complete remote catalog of policy rules", () => {
    const tools = Array.from({ length: 2049 }, (_, index) => `tool_${index}`);
    expect(validateWire("MCPServerCandidate", candidate(tools))).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: "MCPServerCandidate.disabledTools" }),
      ]),
    );
  });
});
