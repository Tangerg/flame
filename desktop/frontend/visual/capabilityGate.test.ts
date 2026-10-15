import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { VISUAL_RUNTIME_FEATURES } from "./agentFixtureFacts";

const SRC = join(import.meta.dirname, "..", "src");
const manifest = JSON.parse(
  readFileSync(join(import.meta.dirname, "../../../runtime/contract/manifest.json"), "utf8"),
) as { features: string[] };

function gatedFeatures(source: string): string[] {
  return Array.from(
    source.matchAll(/useRuntimeCapability\(\s*["']([a-zA-Z]+)["']\s*\)/g),
    (match) => match[1]!,
  );
}

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

describe("the visual fixtures' Runtime", () => {
  it("finds every literal capability query without relying on an application inventory count", () => {
    expect(
      gatedFeatures(`
      useRuntimeCapability("git");
      useRuntimeCapability('relocate');
      useRuntimeCapability( "plugins" );
    `),
    ).toEqual(["git", "relocate", "plugins"]);
  });

  it("advertises only current published Runtime features", () => {
    expect(VISUAL_RUNTIME_FEATURES.filter((name) => !manifest.features.includes(name))).toEqual([]);
  });

  it("advertises every capability the app gates a surface on", () => {
    const gated = new Set<string>();
    for (const file of sources(SRC)) {
      for (const feature of gatedFeatures(readFileSync(file, "utf8"))) gated.add(feature);
    }

    expect([...gated].filter((name) => !VISUAL_RUNTIME_FEATURES.includes(name as never))).toEqual(
      [],
    );
  });
});
