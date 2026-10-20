import { readFileSync, readdirSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-client/validation";
import { WIRE_ENUMS } from "@flame/runtime-client/wire";
import { WIRE_SAMPLES } from "@flame/runtime-client/testing";
import requestMeta from "@flame/runtime-client/testing/samples/request.meta.json";
const sampleDirectory = new URL(
  ".",
  import.meta.resolve("@flame/runtime-client/testing/samples/request.meta.json"),
);
const files = Object.fromEntries(
  readdirSync(sampleDirectory)
    .filter((file) => file.endsWith(".json"))
    .map((file) => [
      file,
      JSON.parse(readFileSync(new URL(file, sampleDirectory), "utf8")) as unknown,
    ]),
);
function sample(file: string): unknown {
  return files[file];
}

describe("the canonical wire samples", () => {
  it("covers every file in the samples directory", () => {
    const bound = new Set(WIRE_SAMPLES.map((entry) => entry.file));
    const present = Object.keys(files);
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
    server: "files",
    disabledTools,
  });

  it("rejects an invalid remote identity at the item path", () => {
    expect(validateWire("MCPToolExposure", candidate(["tool/name"]))).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: "MCPToolExposure.disabledTools[0]" }),
      ]),
    );
  });

  it("rejects more than one complete remote catalog of policy rules", () => {
    const tools = Array.from({ length: 2049 }, (_, index) => `tool_${index}`);
    expect(validateWire("MCPToolExposure", candidate(tools))).toEqual(
      expect.arrayContaining([expect.objectContaining({ path: "MCPToolExposure.disabledTools" })]),
    );
  });
});
