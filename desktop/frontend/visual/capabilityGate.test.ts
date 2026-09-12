import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { VISUAL_RUNTIME_FEATURES } from "./agentFixtureFacts";

const SRC = join(import.meta.dirname, "..", "src");

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

describe("the visual fixtures' Runtime", () => {
  it("advertises every capability the app gates a surface on", () => {
    const gated = new Set<string>();
    for (const file of sources(SRC)) {
      for (const match of readFileSync(file, "utf8").matchAll(
        /useRuntimeCapability\(\s*"([a-zA-Z]+)"\s*\)/g,
      )) {
        gated.add(match[1]!);
      }
    }

    expect(gated.size).toBeGreaterThanOrEqual(3);
    expect([...gated].filter((name) => !VISUAL_RUNTIME_FEATURES.includes(name as never))).toEqual(
      [],
    );
  });
});
